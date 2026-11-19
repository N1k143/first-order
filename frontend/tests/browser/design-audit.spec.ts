import { test, expect } from '@playwright/test';

const routes = [
  '/',
  '/companies',
  '/companies/arman-coffee',
  '/companies/arman-coffee/contacts',
  '/companies/arman-coffee/correction',
  '/orders',
  '/orders/coffee-website',
  '/orders/interior-website/apply',
  '/login',
  '/login/company',
  '/register',
  '/register/developer',
  '/register/company',
  '/forgot-password',
  '/reset-password',
  '/verify-email',
  '/dashboard',
  '/applications',
  '/saved',
  '/contacts',
  '/profile',
  '/profile/edit',
  '/profile/portfolio',
  '/settings',
  '/workspace',
  '/workspace/orders',
  '/workspace/orders/new',
  '/workspace/orders/coffee-website',
  '/workspace/applications',
  '/workspace/profile',
  '/workspace/members',
  '/moderation',
  '/moderation/history',
  '/about',
  '/support',
  '/privacy',
  '/terms',
];

for (const width of [320, 768, 1024, 1440]) {
  test(`pages fit ${width}px without runtime errors`, async ({ page }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width, height: 900 });
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    const overflow: string[] = [];
    for (const route of routes) {
      await page.goto(route);
      await expect(page.locator('h1')).toBeVisible();
      const result = await page.evaluate(() => {
        const width = document.documentElement.clientWidth;
        if (document.documentElement.scrollWidth <= width + 1) return [];
        return [...document.querySelectorAll('main *, header *')]
          .filter((el) => {
            const box = el.getBoundingClientRect();
            return box.width && (box.right > width + 1 || box.left < -1);
          })
          .slice(0, 6)
          .map((el) => `${el.tagName}.${el.className}`);
      });
      if (result.length) overflow.push(`${route}: ${result.join(', ')}`);
    }
    expect(overflow).toEqual([]);
    expect(errors).toEqual([]);
  });
}

test('home selects newest available order and hides the section when none exist', async ({
  page,
}) => {
  await page.goto('/settings');
  await page.getByRole('button', { name: 'Выйти из аккаунта', exact: true }).click();
  await page.evaluate(() => {
    const data = JSON.parse(localStorage.getItem('firstorder:demo:v1')!);
    const template = data.orders[0];
    data.orders = [
      { ...template, id: 'old', title: 'Старый заказ', publishedAt: '2026-01-01' },
      { ...template, id: 'new', title: 'Новый заказ', publishedAt: '2026-11-19T10:00:00Z' },
      { ...template, id: 'hidden', visibility: false, publishedAt: '2099-01-01' },
      { ...template, id: 'draft', status: 'draft', publishedAt: '2099-01-02' },
    ];
    localStorage.setItem('firstorder:demo:v1', JSON.stringify(data));
  });
  await page.goto('/');
  const latest = page.getByTestId('latest-order');
  await expect(latest.getByRole('heading', { name: 'Новый заказ' })).toBeVisible();
  await latest.getByRole('link', { name: 'Посмотреть заказ' }).click();
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Новый заказ');
  await page.evaluate(() => {
    const data = JSON.parse(localStorage.getItem('firstorder:demo:v1')!);
    data.orders = [];
    localStorage.setItem('firstorder:demo:v1', JSON.stringify(data));
  });
  await page.goto('/');
  await expect(page.getByTestId('latest-order')).toHaveCount(0);
  await expect(page.getByText('Открытых заказов пока нет.')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Посмотреть заказ', exact: true })).toHaveCount(0);
});

test('tablet navigation and filters stay usable; mobile search and previous page are available', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 768, height: 1000 });
  await page.goto('/companies');
  await page.locator('summary').click();
  await expect(page.getByRole('navigation', { name: 'Мобильная навигация' })).toBeVisible();
  await page.locator('summary').click();
  await page.getByRole('button', { name: /Фильтры ·/ }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.getByRole('button', { name: 'Показать компании' }).click();
  await page.screenshot({ path: testInfo.outputPath('catalog-tablet.png'), fullPage: true });
  await page.setViewportSize({ width: 320, height: 900 });
  await expect(page.getByRole('button', { name: 'Найти', exact: true })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Страницы каталога' })
    .getByRole('button', { name: '2', exact: true })
    .click();
  await expect(page).toHaveURL(/page=2/);
  await page
    .getByRole('navigation', { name: 'Страницы каталога' })
    .getByRole('button', { name: '1', exact: true })
    .click();
  await expect(page).toHaveURL(/page=1/);
});
