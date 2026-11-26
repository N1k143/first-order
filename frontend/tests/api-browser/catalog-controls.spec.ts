import { test, expect } from '@playwright/test';

test('mobile catalog keeps filters, city selection and favorites available', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const requests: URL[] = [];
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/me' || url.pathname.startsWith('/api/auth/'))
      return route.fulfill({ status: 401, json: {} });
    if (url.pathname === '/api/companies') {
      requests.push(url);
      return route.fulfill({
        json: {
          items: [
            {
              id: 'company',
              name: 'Компания проверки',
              initials: 'КП',
              category: 'Кафе',
              city: url.searchParams.get('city') || 'Астана',
              address: 'Адрес',
              websiteStatus: 'needs_review',
              checkedAt: null,
            },
          ],
          total: 4,
          page: Number(url.searchParams.get('page') || 1),
          pageSize: 3,
        },
      });
    }
    return route.fulfill({ json: [] });
  });
  await page.goto('/companies');
  await expect(page.getByRole('button', { name: 'Сохранить', exact: true })).toBeVisible();
  expect(requests[0].searchParams.get('city')).toBe('Астана');
  expect(requests[0].searchParams.get('pageSize')).toBe('3');
  await page.getByRole('button', { name: 'Фильтры', exact: false }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('button', { name: 'Все статусы', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(dialog.getByRole('button', { name: 'Сайт не найден', exact: true })).toHaveAttribute(
    'aria-pressed',
    'false',
  );
  await dialog.getByRole('combobox', { name: 'Город', exact: true }).selectOption('Алматы');
  await expect.poll(() => requests.at(-1)?.searchParams.get('city')).toBe('Алматы');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Далее →', exact: true }).click();
  await expect.poll(() => requests.at(-1)?.searchParams.get('page')).toBe('2');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

test('combined filters survive quick changes, canonical links and reloads', async ({ page }) => {
  const requests: URL[] = [];
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/me' || url.pathname.startsWith('/api/auth/'))
      return route.fulfill({ status: 401, json: {} });
    if (url.pathname === '/api/categories')
      return route.fulfill({ json: [{ id: 'category-cafe', name: 'Кафе' }] });
    if (url.pathname === '/api/companies') {
      requests.push(url);
      return route.fulfill({ json: { items: [], total: 0, page: 1, pageSize: 3 } });
    }
    return route.fulfill({ json: [] });
  });
  await page.route('**/companies?**', async (route) => {
    if (new URL(route.request().url()).pathname === '/companies')
      await new Promise((resolve) => setTimeout(resolve, 120));
    await route.fallback();
  });
  await page.goto('/companies?q=кофеня&page=5&website_status=needs_review');
  await expect(page.getByRole('button', { name: 'Нужна проверка', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.getByLabel('Сортировка', { exact: true }).first()).toHaveValue('relevance');
  await page.getByRole('button', { name: 'Кафе', exact: true }).click({ noWaitAfter: true });
  await page.getByRole('button', { name: 'Сайт найден', exact: true }).click({ noWaitAfter: true });
  await page.getByLabel('Сортировка', { exact: true }).first().selectOption('name');
  await expect
    .poll(() => {
      const params = new URL(page.url()).searchParams;
      return [
        params.get('q'),
        params.get('category_id'),
        params.get('status'),
        params.get('sort'),
        params.get('page'),
      ];
    })
    .toEqual(['кофеня', 'category-cafe', 'found', 'name', null]);
  await expect.poll(() => requests.at(-1)?.searchParams.get('status')).toBe('found');
  expect(requests.at(-1)?.searchParams.get('category_id')).toBe('category-cafe');
  expect(requests.at(-1)?.searchParams.has('website_status')).toBe(false);
  await page.reload();
  await expect(page.getByRole('button', { name: 'Кафе', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.getByRole('button', { name: 'Сайт найден', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await page.getByRole('button', { name: 'Сбросить фильтры', exact: true }).click();
  await expect.poll(() => requests.at(-1)?.searchParams.get('city')).toBe('');
  await expect.poll(() => new URL(page.url()).searchParams.has('q')).toBe(false);
});
