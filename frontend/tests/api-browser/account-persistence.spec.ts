import { test, expect, type BrowserContext } from '@playwright/test';

const ids = {
  user: 'a1111111-1111-4111-8111-111111111111',
  company: 'b2222222-2222-4222-8222-222222222222',
  item: 'c3333333-3333-4333-8333-333333333333',
};
function accountServer() {
  const state = {
    signedIn: true,
    profile: {
      id: ids.user,
      name: 'Серверный пользователь',
      email: 'account@example.test',
      role: 'developer',
      companies: [],
      avatarUrl: '',
      city: '',
      bio: 'Сведения с сервера',
      headline: '',
      skills: ['TypeScript'],
      github: '',
    },
    portfolio: [] as {
      id: string;
      title: string;
      description: string | null;
      url: string | null;
    }[],
    favorite: true,
    contact: {
      companyId: ids.company,
      status: 'planned',
      note: 'Заметка с сервера',
      date: '2026-11-19',
      contactedAt: '2026-11-19T12:00:00.000Z',
    },
    failProfile: false,
    failPortfolio: false,
    changedEmail: '',
    passwordChanged: false,
    unexpected: [] as string[],
    profileWrites: [] as Record<string, unknown>[],
  };
  const company = {
    id: ids.company,
    name: 'Компания только из API',
    initials: 'КА',
    category: 'Услуги',
    city: 'Алматы',
    address: 'Адрес с сервера',
    websiteStatus: 'found',
    checkedAt: '2026-11-19',
  };
  async function attach(context: BrowserContext) {
    await context.route('**/api/**', async (route) => {
      const request = route.request(),
        url = new URL(request.url()),
        path = url.pathname.slice(4),
        method = request.method();
      const body = request.postData() ? request.postDataJSON() : undefined;
      const send = (data: unknown, status = 200) =>
        route.fulfill({
          status,
          contentType: 'application/json',
          body: status === 204 ? '' : JSON.stringify(data),
        });
      const envelope = (items: unknown[]) => ({
        items,
        total: items.length,
        page: 1,
        pageSize: 100,
      });
      if (path === '/auth/refresh') return send({ error: { message: 'Войдите в аккаунт.' } }, 401);
      if (path === '/auth/logout') {
        state.signedIn = false;
        return send(null, 204);
      }
      if (path === '/auth/change-email') {
        expect(Object.keys(body)).toEqual(['email']);
        state.changedEmail = body.email;
        return send({ status: 'confirmation_required', email: body.email }, 202);
      }
      if (path === '/auth/reset-password') {
        expect(Object.keys(body).sort()).toEqual(['confirm', 'password']);
        expect(body.confirm).toBe(body.password);
        state.passwordChanged = true;
        return send({ status: 'updated' });
      }
      if (path === '/me')
        return state.signedIn
          ? send(state.profile)
          : send({ error: { message: 'Войдите в аккаунт.' } }, 401);
      if (path === '/me/profile' && method === 'PATCH') {
        expect(Object.keys(body).sort()).toEqual([
          'avatar_url',
          'bio',
          'github_url',
          'name',
          'skills',
        ]);
        state.profileWrites.push(body);
        if (state.failProfile) return send({ error: { message: 'Сервер отклонил профиль.' } }, 422);
        Object.assign(state.profile, {
          name: body.name,
          bio: body.bio || '',
          github: body.github_url || '',
          skills: (body.skills || '')
            .split(',')
            .map((s: string) => s.trim())
            .filter(Boolean),
          avatarUrl: body.avatar_url || '',
        });
        return send(state.profile);
      }
      if (path === '/me/portfolio' && method === 'GET')
        return state.failPortfolio
          ? send({ error: { message: 'Портфолио временно недоступно.' } }, 503)
          : send(state.portfolio);
      if (path === '/me/portfolio' && method === 'POST') {
        expect(Object.keys(body).sort()).toEqual(['description', 'project_url', 'title']);
        const item = {
          id: ids.item,
          title: body.title,
          description: body.description,
          url: body.project_url,
        };
        state.portfolio.push(item);
        return send(item, 201);
      }
      if (path === `/me/portfolio/${ids.item}` && method === 'PATCH') {
        expect(Object.keys(body).sort()).toEqual(['description', 'project_url', 'title']);
        Object.assign(state.portfolio[0], {
          title: body.title,
          description: body.description,
          url: body.project_url,
        });
        return send(state.portfolio[0]);
      }
      if (path === `/me/portfolio/${ids.item}` && method === 'DELETE') {
        state.portfolio = [];
        return send(null, 204);
      }
      if (path === '/me/favorites') return send(envelope(state.favorite ? [company] : []));
      if (path === `/me/favorites/${ids.company}` && method === 'DELETE') {
        state.favorite = false;
        return send(null, 204);
      }
      if (path === '/me/contacts') return send(envelope([state.contact]));
      if (path === `/me/contacts/${ids.company}` && method === 'PUT') {
        expect(Object.keys(body).sort()).toEqual(['contacted_at', 'notes', 'status']);
        Object.assign(state.contact, {
          note: body.notes || '',
          date: body.contacted_at?.slice(0, 10) || '',
          contactedAt: body.contacted_at,
          status: body.status,
        });
        return send(state.contact);
      }
      if (path === `/companies/${ids.company}`) return send(company);
      if (path === '/companies') return send(envelope([company]));
      if (path === '/categories' || path === '/orders' || path === '/me/applications')
        return send([]);
      state.unexpected.push(`${method} ${path}`);
      return send({ error: { message: 'Unexpected mocked route' } }, 404);
    });
  }
  return { state, attach };
}

test('profile/avatar and portfolio persist through reload and another browser context', async ({
  page,
  context,
  browser,
  baseURL,
}) => {
  const server = accountServer();
  await server.attach(context);
  await page.goto('/profile/edit');
  await expect(page.getByLabel('Имя и фамилия', { exact: true })).toHaveValue(
    'Серверный пользователь',
  );
  await page.getByLabel('Имя и фамилия', { exact: true }).fill('Обновлённый профиль');
  await page.locator('textarea[name="bio"]').fill('Профиль хранится на сервере');
  await page.getByLabel('Навыки через запятую', { exact: true }).fill('Go, React');
  const png = await page.evaluate(() => {
    const canvas = document.createElement('canvas');
    canvas.width = 2;
    canvas.height = 2;
    canvas.getContext('2d')!.fillRect(0, 0, 2, 2);
    return canvas.toDataURL('image/png').split(',')[1];
  });
  await page.getByLabel('Загрузить фото', { exact: true }).setInputFiles({
    name: 'avatar.png',
    mimeType: 'image/png',
    buffer: Buffer.from(png, 'base64'),
  });
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await expect(page.getByText('Профиль сохранён.', { exact: true })).toBeVisible();
  expect(server.state.profile.avatarUrl).toMatch(/^data:image\/png;base64,/);
  expect(server.state.profileWrites).toHaveLength(1);
  await page.reload();
  await expect(page.getByLabel('Имя и фамилия', { exact: true })).toHaveValue(
    'Обновлённый профиль',
  );
  await expect(page.locator('input[name="avatar_url"]')).toHaveValue(
    server.state.profile.avatarUrl,
  );
  await page.goto('/profile/portfolio');
  const create = page.getByTestId('portfolio-create');
  await create.getByLabel('Название проекта', { exact: true }).fill('Новая работа');
  await create.locator('textarea[name="description"]').fill('Результат проекта');
  await create.getByLabel('Ссылка на проект', { exact: true }).fill('https://example.test/project');
  await create.getByRole('button', { name: 'Добавить в портфолио', exact: true }).click();
  const item = page.getByTestId('portfolio-item');
  await expect(item).toHaveCount(1);
  await item.getByLabel('Название работы', { exact: true }).fill('Работа после редактирования');
  await item.locator('textarea[name="description"]').fill('');
  await item.getByLabel('Адрес проекта', { exact: true }).fill('');
  await item.getByRole('button', { name: 'Сохранить работу', exact: true }).click();
  await expect(item.getByText('Работа обновлена.', { exact: true })).toBeVisible();
  expect(server.state.portfolio[0]).toMatchObject({
    title: 'Работа после редактирования',
    description: null,
    url: null,
  });
  const second = await browser.newContext({ baseURL });
  await server.attach(second);
  try {
    const other = await second.newPage();
    await other.goto('/profile');
    await expect(
      other.getByRole('heading', { name: 'Обновлённый профиль', exact: true }),
    ).toBeVisible();
    await expect(
      other.getByRole('heading', { name: 'Работа после редактирования', exact: true }),
    ).toBeVisible();
    expect(await other.evaluate(() => localStorage.getItem('firstorder:demo:v1'))).toBeNull();
    await other.goto('/profile/portfolio');
    await other
      .getByTestId('portfolio-item')
      .getByRole('button', { name: 'Удалить работу', exact: true })
      .click();
    await other
      .getByRole('dialog')
      .getByRole('button', { name: 'Удалить работу', exact: true })
      .click();
    await expect(other.getByTestId('portfolio-item')).toHaveCount(0);
    await page.reload();
    await expect(page.getByTestId('portfolio-item')).toHaveCount(0);
  } finally {
    await second.close();
  }
  expect(server.state.unexpected).toEqual([]);
});

test('favorites, contacts, settings use server data and do not optimistically confirm email', async ({
  page,
  context,
}) => {
  const server = accountServer();
  await server.attach(context);
  await page.goto('/saved');
  await expect(page.getByTestId('saved-company')).toContainText('Компания только из API');
  await page.getByRole('button', { name: 'Убрать из избранного', exact: true }).click();
  await expect(page.getByTestId('saved-company')).toHaveCount(0);
  await page.reload();
  await expect(page.getByText('Сохраните интересные компании', { exact: true })).toBeVisible();
  expect(server.state.favorite).toBe(false);
  await page.goto('/contacts');
  await expect(page.getByTestId('contact-entry')).toContainText('Компания только из API');
  await page.locator('textarea[name="note"]').fill('Обновлённая заметка');
  await page.locator('select[name="status"]').selectOption('replied');
  await page.getByRole('button', { name: 'Сохранить заметку', exact: true }).click();
  await expect(page.getByText('Заметка сохранена.', { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.locator('textarea[name="note"]')).toHaveValue('Обновлённая заметка');
  await expect(page.locator('select[name="status"]')).toHaveValue('replied');
  await page.goto('/settings');
  await expect(page.getByText('Сбросить демоданные', { exact: true })).toHaveCount(0);
  await page.getByLabel('Новый email', { exact: true }).fill('new-account@example.test');
  await page.getByRole('button', { name: 'Отправить подтверждение', exact: true }).click();
  await expect(page.getByText(/До завершения подтверждения действует прежний email/)).toBeVisible();
  await expect(page.getByTestId('current-account-email')).toHaveText('account@example.test');
  expect(server.state.changedEmail).toBe('new-account@example.test');
  await page.getByLabel('Новый пароль', { exact: true }).fill('mock-new-password-123');
  await page.getByLabel('Повторите пароль', { exact: true }).fill('mock-new-password-123');
  await page.getByRole('button', { name: 'Изменить пароль', exact: true }).click();
  await expect(page.getByText('Пароль обновлён.', { exact: true })).toBeVisible();
  expect(server.state.passwordChanged).toBe(true);
  await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect(server.state.signedIn).toBe(false);
  expect(server.state.unexpected).toEqual([]);
});

test('server profile and portfolio failures remain visible and preserve stored values', async ({
  page,
  context,
}) => {
  const server = accountServer();
  await server.attach(context);
  server.state.failProfile = true;
  await page.goto('/profile/edit');
  await page.getByLabel('Имя и фамилия', { exact: true }).fill('Несохранённое имя');
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await expect(page.getByText('Сервер отклонил профиль.', { exact: true })).toBeVisible();
  expect(server.state.profile.name).toBe('Серверный пользователь');
  await page.reload();
  await expect(page.getByLabel('Имя и фамилия', { exact: true })).toHaveValue(
    'Серверный пользователь',
  );
  server.state.failPortfolio = true;
  await page.goto('/profile/portfolio');
  await expect(page.getByText('Портфолио временно недоступно.', { exact: true })).toBeVisible();
  await expect(page.getByText('Портфолио пока пусто', { exact: true })).toHaveCount(0);
  server.state.failPortfolio = false;
  await page.getByRole('button', { name: 'Попробовать снова', exact: true }).click();
  await expect(page.getByText('Портфолио пока пусто', { exact: true })).toBeVisible();
  expect(server.state.unexpected).toEqual([]);
});
