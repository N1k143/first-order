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
  await dialog.getByLabel('Город в фильтрах').selectOption('Алматы');
  await expect.poll(() => requests.at(-1)?.searchParams.get('city')).toBe('Алматы');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Далее →', exact: true }).click();
  await expect.poll(() => requests.at(-1)?.searchParams.get('page')).toBe('2');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});
