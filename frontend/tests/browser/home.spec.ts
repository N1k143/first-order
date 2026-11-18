import { test, expect } from '@playwright/test';
test('desktop hero follows Figma geometry and uses the local Golos font', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('/');
  await page.evaluate(() => document.fonts.ready);
  const title = page.getByRole('heading', { level: 1 });
  await expect(title).toHaveText('Ваш первый заказ ближе, чем кажется.');
  const box = await title.boundingBox();
  expect(box?.x).toBe(80);
  expect(box?.y).toBe(204);
  expect(box?.height).toBe(124);
  expect(await title.evaluate((el) => getComputedStyle(el).fontFamily)).toContain('Golos Text');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(1440);
  await page.screenshot({ path: testInfo.outputPath('home-desktop.png'), fullPage: true });
});
test('mobile page fits viewport and navigation opens with keyboard', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await page.evaluate(() => document.fonts.ready);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  const menu = page.locator('summary');
  await menu.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('navigation', { name: 'Мобильная навигация' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('home-mobile.png'), fullPage: true });
});
test('search submits user input as an encoded query parameter', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('textbox', { name: 'Компания, сфера или задача' }).fill('кофейни & меню');
  await page.getByRole('button', { name: 'Найти компании' }).click();
  await expect(page).toHaveURL(/\/companies\?/);
  expect(new URL(page.url()).searchParams.get('q')).toBe('кофейни & меню');
});
