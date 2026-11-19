import {test, expect, type BrowserContext, type Page} from '@playwright/test';
import {openSync, closeSync, writeFileSync, chmodSync} from 'node:fs';
import {randomUUID} from 'node:crypto';

type Profile = {user_id: string; companies: {company_id: string; member_role: string; name: string}[]};
type CanonicalOrder = {id: string; status: string; selected_application_id?: string};
type CanonicalApplication = {id: string; status: string; order_id: string};

function credentials(role: 'COMPANY'|'DEVELOPER', tag: string) {
  const email = process.env[`E2E_${role}_EMAIL`] || '';
  const password = process.env[`E2E_${role}_PASSWORD`] || '';
  if (email.split('@').length !== 2 || !email.split('@')[0].includes(tag) || password.length < 12) {
    throw new Error(`Missing or untagged E2E ${role} credentials.`);
  }
  return {email, password};
}

async function login(page: Page, account: {email: string; password: string}) {
  await page.goto('/login');
  await page.getByLabel('Email', {exact: true}).fill(account.email);
  await page.getByLabel('Пароль', {exact: true}).fill(account.password);
  await page.getByRole('button', {name: 'Войти', exact: true}).click();
  await expect(page).toHaveURL(/\/companies$/);
  await expect(page.getByRole('button', {name: 'Выйти', exact: true})).toBeVisible();
}

async function read<T>(context: BrowserContext, path: string): Promise<T> {
  const response = await context.request.get('/api/v1' + path, {maxRedirects: 0});
  expect(response.status(), 'Canonical API read status').toBe(200);
  return await response.json() as T;
}

test('production UI: draft → publish → apply → select → accepted', async ({browser, baseURL}) => {
  test.skip(process.env.E2E_REGISTER_UI === '1', 'Registration-only stage; confirm accounts before running the lifecycle.');
  const tag = process.env.E2E_RUN_TAG || '';
  if (!/^[a-z0-9][a-z0-9-]{7,63}$/.test(tag) || !tag.includes('e2e')) throw new Error('Explicit E2E_RUN_TAG is required.');
  const companyAccount = credentials('COMPANY', tag);
  const developerAccount = credentials('DEVELOPER', tag);
  const manifestPath = process.env.E2E_BROWSER_MANIFEST_FILE || `/tmp/firstorder-browser-${tag}-${randomUUID()}.json`;
  closeSync(openSync(manifestPath, 'wx', 0o600));
  const manifest: {run_tag: string; target?: string; status: string; step: string; started_at: string; user_ids: string[]; company_ids: string[]; order_ids: string[]; application_ids: string[]; email_delivery_tested: boolean} = {
    run_tag: tag, target: baseURL, status: 'running', step: 'login', started_at: new Date().toISOString(),
    user_ids: [], company_ids: [], order_ids: [], application_ids: [], email_delivery_tested: false,
  };
  function save(step?: string) {
    if (step) manifest.step = step;
    chmodSync(manifestPath, 0o600);
    writeFileSync(manifestPath, JSON.stringify(manifest, null, 2), {mode: 0o600});
  }
  // Separate contexts simulate two people and keep HttpOnly cookies separate.
  const company = await browser.newContext({baseURL});
  const developer = await browser.newContext({baseURL});
  save();
  try {
    const companyPage = await company.newPage(), developerPage = await developer.newPage();
    await login(companyPage, companyAccount);
    const companyProfile = await read<Profile>(company, '/me');
    const owned = companyProfile.companies.filter(c => c.member_role === 'owner' && c.name.includes(tag));
    expect(owned.length, 'Exactly one tagged company is required').toBe(1);
    manifest.user_ids.push(companyProfile.user_id);
    manifest.company_ids.push(owned[0].company_id);
    save('create_draft');
    const title = `${tag} browser test ${randomUUID().slice(0, 8)}`;
    const deadline = new Date(Date.now() + 30 * 86400000).toISOString().slice(0, 10);
    await companyPage.goto('/workspace/orders/new');
    // The wrapping label includes option text in its accessible name.
    await companyPage.locator('select[name="company"]').selectOption(owned[0].company_id);
    await companyPage.getByLabel('Название заказа', {exact: true}).fill(title);
    await companyPage.getByLabel('Описание задачи', {exact: true}).fill(`${tag}: изолированная проверка приложения, не коммерческий заказ.`);
    await companyPage.getByLabel('Бюджет от, ₸', {exact: true}).fill('1000');
    await companyPage.getByLabel('Бюджет до, ₸', {exact: true}).fill('2000');
    await companyPage.getByLabel('Срок выполнения', {exact: true}).fill(deadline);
    const [createdResponse] = await Promise.all([
      companyPage.waitForResponse(r => new URL(r.url()).pathname === '/api/orders' && r.request().method() === 'POST'),
      companyPage.getByRole('button', {name: 'Создать черновик', exact: true}).click(),
    ]);
    expect(createdResponse.status()).toBe(201);
    const created = await createdResponse.json() as {id: string};
    manifest.order_ids.push(created.id);
    save('draft_navigation');
    await expect(companyPage).toHaveURL(/\/workspace\/orders\/[a-f0-9-]{36}$/);
    const orderID = new URL(companyPage.url()).pathname.split('/').at(-1)!;
    expect(orderID).toBe(created.id);
    save('publish_order');
    expect((await read<CanonicalOrder>(company, `/orders/${orderID}`)).status).toBe('draft');
    await companyPage.getByRole('button', {name: 'Опубликовать заказ', exact: true}).click();
    await expect(companyPage.getByTestId('order-status')).toHaveText('Приём откликов');
    expect((await read<CanonicalOrder>(company, `/orders/${orderID}`)).status).toBe('open');

    save('developer_login');
    await login(developerPage, developerAccount);
    const developerProfile = await read<Profile>(developer, '/me');
    manifest.user_ids.push(developerProfile.user_id);
    expect(developerProfile.user_id).not.toBe(companyProfile.user_id);
    save('submit_application');
    await developerPage.goto(`/orders/${orderID}/apply`);
    await developerPage.getByLabel('Сопроводительное письмо', {exact: true}).fill(`${tag}: тестовое предложение для изолированной проверки выбора исполнителя.`);
    await developerPage.getByLabel('Стоимость, ₸', {exact: true}).fill('1500');
    await developerPage.getByLabel('Готово к дате', {exact: true}).fill(deadline);
    const [submittedResponse] = await Promise.all([
      developerPage.waitForResponse(r => new URL(r.url()).pathname === `/api/orders/${orderID}/applications` && r.request().method() === 'POST'),
      developerPage.getByRole('button', {name: 'Отправить отклик', exact: true}).click(),
    ]);
    expect(submittedResponse.status()).toBe(201);
    const submitted = await submittedResponse.json() as {id: string};
    manifest.application_ids.push(submitted.id);
    save('application_submitted');
    await expect(developerPage.getByRole('heading', {name: 'Отклик отправлен', exact: true})).toBeVisible();
    const applications = await read<{items: CanonicalApplication[]}>(company, `/orders/${orderID}/applications`);
    expect(applications.items).toHaveLength(1);
    const applicationID = applications.items[0].id;
    expect(applicationID).toBe(submitted.id);
    save('select_contractor');
    expect(applications.items[0].status).toBe('submitted');
    await companyPage.getByRole('button', {name: 'Обновить отклики', exact: true}).click();
    await companyPage.getByTestId('application-card').filter({hasText: tag}).getByRole('button', {name: 'Выбрать исполнителя', exact: true}).click();
    await companyPage.getByRole('dialog').getByRole('button', {name: 'Подтвердить выбор', exact: true}).click();
    await expect(companyPage.getByTestId('order-status')).toHaveText('Исполнитель выбран');
    const selected = await read<CanonicalOrder>(company, `/orders/${orderID}`);
    expect(selected.status).toBe('contractor_selected');
    expect(selected.selected_application_id).toBe(applicationID);

    save('developer_acceptance');
    await developerPage.goto('/applications');
    await expect(developerPage.getByTestId('my-application').filter({hasText: title}).getByText('Исполнитель выбран', {exact: true})).toBeVisible();
    expect((await read<CanonicalApplication>(developer, `/applications/${applicationID}`)).status).toBe('accepted');
    manifest.status = 'passed';
    save('complete');
  } catch {
    manifest.status = 'failed';
    save();
    // Never forward Playwright's raw fill/call logs, which can contain secrets.
    throw new Error(`Production E2E failed at ${manifest.step}. See the private ID manifest.`);
  } finally {
    await Promise.all([company.close(), developer.close()]);
  }
});

test('production UI: public signup reaches email confirmation screen', async ({browser, baseURL}) => {
  test.skip(process.env.E2E_REGISTER_UI !== '1', 'Enable only for explicitly approved signup email recipients.');
  if (process.env.E2E_EMAIL_POLICY !== 'confirmation' || process.env.E2E_ALLOW_EMAIL !== '1') {
    throw new Error('Confirmation policy and explicit email permission are required.');
  }
  const tag = process.env.E2E_RUN_TAG || '';
  if (!/^[a-z0-9][a-z0-9-]{7,63}$/.test(tag) || !tag.includes('e2e')) throw new Error('Explicit E2E_RUN_TAG is required.');
  const accounts = {COMPANY: credentials('COMPANY', tag), DEVELOPER: credentials('DEVELOPER', tag)};
  const path = process.env.E2E_BROWSER_MANIFEST_FILE || `/tmp/firstorder-browser-registration-${tag}-${randomUUID()}.json`;
  closeSync(openSync(path, 'wx', 0o600));
  const manifest = {run_tag: tag, target: baseURL, stage: 'public_signup', status: 'running', accepted_emails: [] as string[], email_delivery_tested: false, cleanup_lookup: 'Exact tagged email; profile/company IDs are created after confirmation.'};
  const save = () => writeFileSync(path, JSON.stringify(manifest, null, 2), {mode: 0o600});
  save();
  try {
    for (const role of ['COMPANY', 'DEVELOPER'] as const) {
      const context = await browser.newContext({baseURL});
      try {
        const page = await context.newPage(), company = role === 'COMPANY';
        await page.goto(company ? '/register/company' : '/register/developer');
        await page.getByLabel(company ? 'Ваше имя' : 'Имя и фамилия', {exact: true}).fill(`${tag} ${role.toLowerCase()}`);
        await page.getByLabel(company ? 'Рабочий email' : 'Email', {exact: true}).fill(accounts[role].email);
        await page.getByLabel('Пароль', {exact: true}).fill(accounts[role].password);
        if (company) {
          await page.getByLabel('Компания', {exact: true}).fill(`${tag} test company`);
          await page.getByRole('button', {name: 'Компании нет в каталоге? Добавить данные компании', exact: true}).click();
          await page.getByLabel('Город компании', {exact: true}).fill('E2E test city');
        } else await page.getByLabel('Повторите пароль', {exact: true}).fill(accounts[role].password);
        const [response] = await Promise.all([
          page.waitForResponse(r => new URL(r.url()).pathname === '/api/auth/register' && r.request().method() === 'POST'),
          page.getByRole('button', {name: company ? 'Создать аккаунт компании' : 'Создать аккаунт', exact: true}).click(),
        ]);
        expect(response.status()).toBe(201);
        manifest.accepted_emails.push(accounts[role].email);
        save();
        expect((await response.json() as {status: string}).status).toBe('verification_required');
        await expect(page).toHaveURL(/\/verify-email\?/);
        await expect(page.getByRole('heading', {name: 'Проверьте почту', exact: true})).toBeVisible();
      } finally { await context.close(); }
    }
    manifest.status = 'awaiting_email_confirmation';
    save();
  } catch {
    manifest.status = 'failed';
    save();
    throw new Error('Public signup stage failed. Inspect private manifest and server request IDs; do not log browser secrets.');
  }
});
