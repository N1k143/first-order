import { test, expect } from '@playwright/test';

test('order search aligns the input and Find button across mobile and desktop', async ({
  page,
}, testInfo) => {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/me' || url.pathname.startsWith('/api/auth/'))
      return route.fulfill({ status: 401, json: {} });
    return route.fulfill({ json: [] });
  });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  for (const width of [320, 390, 700, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto('/orders?city=Астана&min=10000');
    const form = page.locator('form.order-search');
    const input = form.getByRole('textbox', { name: 'Поиск заказов' });
    const button = form.getByRole('button', { name: 'Найти', exact: true });
    await expect(input).toBeVisible();
    await expect(button).toBeVisible();
    const a = (await input.boundingBox())!,
      b = (await button.boundingBox())!,
      f = (await form.boundingBox())!;
    expect(a.height).toBeGreaterThanOrEqual(44);
    expect(Math.abs(a.height - b.height)).toBeLessThanOrEqual(1);
    expect(b.x + b.width).toBeLessThanOrEqual(width);
    if (width <= 700) {
      expect(b.y).toBeGreaterThanOrEqual(a.y + a.height);
      expect(Math.abs(b.width - f.width)).toBeLessThanOrEqual(1);
    } else {
      expect(Math.abs(a.y - b.y)).toBeLessThanOrEqual(1);
      expect(Math.abs(a.width + 12 + b.width - f.width)).toBeLessThanOrEqual(1);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      width,
    );
    if (width === 390 || width === 768)
      await page.screenshot({
        path: testInfo.outputPath(`orders-search-${width}.png`),
        fullPage: true,
      });
  }
  await page.getByRole('textbox', { name: 'Поиск заказов' }).fill('сайт кафе');
  await page
    .locator('form.order-search')
    .getByRole('button', { name: 'Найти', exact: true })
    .click();
  await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe('сайт кафе');
  expect(new URL(page.url()).searchParams.get('city')).toBe('Астана');
  expect(new URL(page.url()).searchParams.get('min')).toBe('10000');
  expect(errors).toEqual([]);
});
