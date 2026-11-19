import { test, expect, type Page } from '@playwright/test';
import { orders } from '../../src/data/demo';

const current = {
  ...orders[0],
  id: 'live-order',
  title: 'Актуальный заказ из API',
  publishedAt: '2026-11-19T10:00:00Z',
};
async function mockAPI(page: Page, list: unknown, status = 200) {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/orders') return route.fulfill({ status, json: list });
    if (url.pathname === '/api/orders/live-order') return route.fulfill({ json: current });
    return route.fulfill({ status: 401, json: { error: { message: 'Войдите в аккаунт' } } });
  });
}

test('API home uses server orders and links to the same record', async ({ page }) => {
  await mockAPI(page, [current]);
  await page.goto('/');
  const section = page.getByTestId('latest-order');
  await expect(section).toContainText(current.title);
  await expect(page.getByText('Сайт для локальной кофейни', { exact: true })).toHaveCount(0);
  await section.getByRole('link', { name: 'Посмотреть заказ' }).click();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(current.title);
});

for (const [name, list, status] of [
  ['empty', [], 200],
  ['failed', { error: { message: 'Сервис недоступен' } }, 503],
  ['hidden', [{ ...current, visibility: false }], 200],
  ['draft', [{ ...current, status: 'draft' }], 200],
] as const) {
  test(`API ${name} list never displays a demo or unavailable order`, async ({ page }) => {
    await mockAPI(page, list, status);
    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    await expect(page.getByTestId('latest-order')).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Посмотреть заказ', exact: true })).toHaveCount(0);
    await expect(page.getByText('Открытых заказов пока нет.')).toHaveCount(0);
  });
}

test('API public and account states fit narrow, tablet and desktop viewports', async ({
  page,
}, testInfo) => {
  test.setTimeout(90000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await mockAPI(page, [current]);
  for (const width of [320, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    for (const path of [
      '/',
      '/orders',
      '/orders/live-order',
      '/orders/live-order/apply',
      '/workspace/orders',
      '/workspace/orders/new',
      '/applications',
      '/login',
      '/register/company',
    ]) {
      await page.goto(path);
      await expect(page.locator('h1')).toBeVisible();
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth),
        `${width} ${path}`,
      ).toBeLessThanOrEqual(width);
    }
  }
  await page.setViewportSize({ width: 768, height: 900 });
  await page.goto('/');
  await expect(page.getByTestId('latest-order')).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('home-api-tablet.png'), fullPage: true });
  expect(errors).toEqual([]);
});

test('private owner records do not mask the newest public order on later pages', async ({
  page,
}) => {
  await mockAPI(page, []);
  await page.route('**/api/orders?**', (route) => {
    const query = new URL(route.request().url()).searchParams;
    const rows =
      query.get('page') === '1'
        ? Array.from({ length: 100 }, (_, index) => ({
            ...current,
            id: `hidden-${index}`,
            visibility: false,
          }))
        : [current];
    return route.fulfill({ json: rows });
  });
  await page.goto('/');
  await expect(page.getByTestId('latest-order')).toContainText(current.title);
});

test('public catalog does not display private drafts from the owner API response', async ({
  page,
}) => {
  await mockAPI(page, [
    current,
    { ...current, id: 'draft', title: 'Личный черновик', status: 'draft' },
  ]);
  await page.goto('/orders');
  await expect(page.getByRole('heading', { name: current.title })).toBeVisible();
  await expect(page.getByText('Личный черновик')).toHaveCount(0);
});
