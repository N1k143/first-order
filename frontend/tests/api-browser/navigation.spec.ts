import { test, expect } from '@playwright/test';

test('delayed paged categories survive repeated navigation and logout', async ({ page }) => {
  let signedIn = true;
  let categoryCalls = 0;
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/auth/logout') {
      signedIn = false;
      await route.fulfill({ status: 204 });
      return;
    }
    if (path === '/api/auth/refresh') {
      await route.fulfill({ status: 401, json: {} });
      return;
    }
    if (path === '/api/me') {
      await route.fulfill({
        status: signedIn ? 200 : 401,
        json: signedIn
          ? {
              id: 'developer',
              name: 'Проверка навигации',
              email: 'test@example.invalid',
              role: 'developer',
              companies: [],
              skills: [],
              bio: '',
              github: '',
              city: '',
              headline: '',
            }
          : {},
      });
      return;
    }
    if (path === '/api/categories') {
      categoryCalls++;
      await new Promise((resolve) => setTimeout(resolve, 200));
      const items = [{ id: 'category', name: 'Образование' }];
      await route.fulfill({
        json: categoryCalls % 2 ? { items, total: 1, limit: 100, page: 1 } : items,
      });
      return;
    }
    if (path === '/api/companies') {
      await route.fulfill({ json: { items: [], total: 0, page: 1, pageSize: 3 } });
      return;
    }
    await route.fulfill({ json: [] });
  });
  await page.goto('/');
  const menu = page.getByRole('navigation', { name: 'Основная навигация', exact: true });
  for (let i = 0; i < 6; i++) {
    await menu.getByRole('link', { name: 'Компании', exact: true }).click();
    await expect(
      page.getByRole('button', { name: 'Образование', exact: true }).first(),
    ).toBeVisible();
    if (i === 2) {
      await page.getByRole('button', { name: 'Выйти', exact: true }).click();
      await expect(page.getByRole('link', { name: 'Войти', exact: true }).first()).toBeVisible();
    }
    await menu.getByRole('link', { name: 'Заказы', exact: true }).click();
    await expect(page.getByText('Заказы не найдены', { exact: true })).toBeVisible();
  }
  expect(categoryCalls).toBeGreaterThanOrEqual(6);
  expect(errors).toEqual([]);
  await expect(page.locator('html#__next_error__')).toHaveCount(0);
});

test('invalid category data shows a retry state instead of crashing the page', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/api/me' || path.startsWith('/api/auth/')) {
      await route.fulfill({ status: 401, json: {} });
      return;
    }
    if (path === '/api/categories') {
      await route.fulfill({ json: { items: null } });
      return;
    }
    await route.fulfill({ json: path === '/api/companies' ? { items: [], total: 0 } : [] });
  });
  await page.goto('/companies');
  await expect(page.getByRole('alert').first()).toContainText('Не удалось прочитать справочник');
  await expect(
    page.getByRole('button', { name: 'Попробовать снова', exact: true }).first(),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
