import { test, expect, type Page } from '@playwright/test';

async function setup(page: Page, role = 'moderator') {
  const writes: { path: string; body: Record<string, unknown>; key?: string }[] = [];
  const moderationReads: string[] = [];
  let stale = true,
    reviewed = false,
    published = false;
  let revision = 7,
    candidateName = 'Длинное название компании для проверки адаптации';
  await page.route('**/api/**', async (route) => {
    const request = route.request(),
      url = new URL(request.url()),
      path = url.pathname;
    if (path === '/api/me')
      return route.fulfill({
        json: {
          id: 'user',
          role,
          name: 'Тестовый Модератор',
          email: 'test@example.invalid',
          companies: [],
          skills: [],
        },
      });
    if (
      path === '/api/orders' ||
      path === '/api/me/portfolio' ||
      path === '/api/me/applications' ||
      path === '/api/me/contacts'
    )
      return route.fulfill({ json: [] });
    if (path === '/api/categories') return route.fulfill({ json: [] });
    if (path.startsWith('/api/v1/')) moderationReads.push(path);
    if (request.method() !== 'GET') {
      writes.push({
        path,
        body: request.postDataJSON() || {},
        key: request.headers()['idempotency-key'],
      });
      if (path.endsWith('/imports/candidates/candidate') && request.method() === 'PATCH') {
        revision++;
        candidateName = String(request.postDataJSON().changes.name || candidateName);
      }
      if (path.endsWith('/review')) {
        if (stale) {
          stale = false;
          return route.fulfill({
            status: 409,
            json: { error: { message: 'Карточка уже изменена. Обновите данные.' } },
          });
        }
        reviewed = true;
      }
      if (path.endsWith('/visibility')) published = true;
      return route.fulfill({ json: { status: 'approved' } });
    }
    if (path.endsWith('/moderation/summary'))
      return route.fulfill({
        json: {
          pending_candidates: reviewed ? 0 : 1,
          pending_corrections: 0,
          hidden_companies: published ? 0 : 1,
        },
      });
    if (path.endsWith('/imports/candidates'))
      return route.fulfill({
        json: {
          items: reviewed
            ? []
            : [
                {
                  id: 'candidate',
                  revision,
                  proposed_company: {
                    name: candidateName,
                    city: 'Астана',
                    address: 'Адрес проверки',
                    category_name: 'Кафе',
                    website_status: 'needs_review',
                  },
                  payload: {
                    source_url: 'https://www.openstreetmap.org/node/123',
                    attribution: '© OpenStreetMap contributors',
                  },
                },
              ],
          total: reviewed ? 0 : 1,
        },
      });
    if (path === '/api/v1/companies')
      return route.fulfill({
        json: {
          items: published ? [] : [{ id: 'hidden-company', name: 'Проверенная компания' }],
          total: published ? 0 : 1,
        },
      });
    if (path.endsWith('/collection/replenishment'))
      return route.fulfill({
        json: { enabled: true, minimum: 20, target: 50, counts: { available: 27, pending: 65 } },
      });
    if (path.endsWith('/collection/jobs'))
      return route.fulfill({
        json: {
          items: [
            {
              id: 'job',
              kind: 'collect',
              status: 'failed',
              attempt: 1,
              processed: 0,
              total: 0,
              error_code: 'SOURCE_UNAVAILABLE',
            },
          ],
          total: 1,
        },
      });
    return route.fulfill({ json: { items: [], total: 0 } });
  });
  return { writes, moderationReads };
}

test('moderator cabinet uses the server, preserves a stale decision and publishes separately', async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.goto('/');
  await expect(
    page.getByRole('link', { name: 'Мой кабинет', exact: true }).first(),
  ).toHaveAttribute('href', '/moderation');
  await page.getByRole('link', { name: 'Мой кабинет', exact: true }).first().click();
  await expect(
    page.getByRole('heading', { name: 'Длинное название компании для проверки адаптации' }),
  ).toBeVisible();
  await page.getByLabel('Основание решения').fill('Контакты и источник проверены');
  await page.getByRole('button', { name: 'Одобрить карточку', exact: true }).click();
  await expect(page.locator('main').getByRole('alert')).toContainText('Карточка уже изменена');
  await expect(page.getByLabel('Основание решения')).toHaveValue('Контакты и источник проверены');
  expect(writes).toHaveLength(1);
  expect(writes[0].body).toEqual({
    decision: 'approve',
    reason: 'Контакты и источник проверены',
    expected_revision: 7,
  });
  await page.getByRole('button', { name: 'Обновить данные', exact: true }).click();
  await expect(page.getByLabel('Основание решения')).toBeVisible();
  await page.getByLabel('Основание решения').fill('Повторная проверка выполнена');
  await page.getByRole('button', { name: 'Одобрить карточку', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Список пуст' })).toBeVisible();
  expect(writes.filter((write) => write.path.endsWith('/visibility'))).toHaveLength(0);
  await page.getByRole('button', { name: 'Скрытые компании', exact: true }).click();
  await page.getByLabel('Основание решения').fill('Разрешена публикация карточки');
  await page.getByRole('button', { name: 'Опубликовать', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Список пуст' })).toBeVisible();
  expect(writes.at(-1)?.body.visibility).toBe('published');
});

test('ordinary accounts cannot load moderator records', async ({ page }) => {
  const { moderationReads, writes } = await setup(page, 'developer');
  await page.goto('/moderation');
  await expect(page.getByText('Нужны права модератора', { exact: true })).toBeVisible();
  expect(moderationReads).toEqual([]);
  expect(writes).toEqual([]);
});

test('collection sends a bounded request with an idempotency key and exposes retry', async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.goto('/moderation/import');
  await expect(page.getByText('Доступно 27, ожидают проверки 65.', { exact: false })).toBeVisible();
  await page.getByLabel('Количество компаний').fill('10');
  await page.getByRole('button', { name: 'Запустить задание', exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].body).toEqual({ kind: 'collect', limit: 10 });
  expect(writes[0].key).toMatch(/^[a-f0-9-]{36}$/);
  await page.getByRole('button', { name: 'Повторить задание', exact: true }).click();
  await expect.poll(() => writes.length).toBe(2);
  expect(writes[1].path).toBe('/api/v1/collection/jobs/job/retry');
});

test('moderation fits mobile, tablet and desktop with usable menus and controls', async ({
  page,
}, testInfo) => {
  test.setTimeout(90000);
  await setup(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 844 });
    for (const path of ['/moderation', '/moderation/import', '/moderation/history']) {
      await page.goto(path);
      await expect(
        page.getByRole('button', { name: 'Обновить данные', exact: true }),
      ).toBeVisible();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
        `${width} ${path}`,
      ).toBeLessThanOrEqual(width);
    }
    if (width <= 1100) {
      await page.locator('summary').click();
      const menu = page.getByRole('navigation', { name: 'Мобильная навигация', exact: true });
      await expect(menu.getByRole('link', { name: 'Модерация', exact: true })).toBeVisible();
      await menu.getByRole('link', { name: 'Модерация', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Модерация', exact: true })).toBeVisible();
      await expect(menu).not.toBeVisible();
    }
    if (width === 390)
      await page.screenshot({ path: testInfo.outputPath('moderation-mobile.png'), fullPage: true });
  }
  expect(errors).toEqual([]);
});

test('candidate edits use the current revision and remain separate from approval', async ({
  page,
}) => {
  const { writes } = await setup(page);
  await page.goto('/moderation');
  await page.getByText('Исправить карточку', { exact: true }).click();
  await page.getByLabel('Название', { exact: true }).fill('Исправленное название');
  await page.getByLabel('Основание исправления').fill('Название сверено с источником');
  await page.getByRole('button', { name: 'Сохранить исправления', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'Исправленное название', exact: true }),
  ).toBeVisible();
  expect(writes[0].body).toEqual({
    expected_revision: 7,
    reason: 'Название сверено с источником',
    changes: { name: 'Исправленное название' },
  });
  await page.getByLabel('Основание решения').fill('Карточка проверена повторно');
  await page.getByRole('button', { name: 'Одобрить карточку', exact: true }).click();
  await expect(page.locator('main').getByRole('alert')).toContainText('Карточка уже изменена');
  expect(writes[1].body.expected_revision).toBe(8);
});

test('an uncertain collection response keeps the same key on retry', async ({ page }) => {
  await setup(page);
  const keys: string[] = [];
  await page.route('**/api/v1/collection/jobs', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    keys.push(route.request().headers()['idempotency-key']);
    await route.fulfill({
      status: keys.length === 1 ? 503 : 200,
      json: keys.length === 1 ? { error: { message: 'Ответ временно недоступен' } } : { id: 'job' },
    });
  });
  await page.goto('/moderation/import');
  await page.getByRole('button', { name: 'Запустить задание', exact: true }).click();
  await expect(page.locator('main').getByRole('alert')).toContainText('Ответ временно недоступен');
  await page.getByRole('button', { name: 'Запустить задание', exact: true }).click();
  await expect.poll(() => keys.length).toBe(2);
  expect(keys[1]).toBe(keys[0]);
});

test('moderator login opens the real cabinet', async ({ page }) => {
  await setup(page);
  await page.route('**/api/auth/login', (route) =>
    route.fulfill({ json: { status: 'authenticated' } }),
  );
  await page.goto('/login');
  await page.getByLabel('Email', { exact: true }).fill('moderator@example.invalid');
  await page.getByLabel('Пароль', { exact: true }).fill('local-browser-fixture-only');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL(/\/moderation$/);
  await expect(page.getByRole('button', { name: 'Одобрить карточку', exact: true })).toBeVisible();
});
