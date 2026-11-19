import { test, expect } from '@playwright/test';

for (const source of ['redirect', 'verification'] as const) {
  test(`email change waits for the second confirmation (${source})`, async ({ page }) => {
    const sessionExchanges: string[] = [];
    await page.route('**/api/**', async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/auth/session') sessionExchanges.push(path);
      if (path === '/api/auth/verify') {
        expect(route.request().postDataJSON()).toEqual({
          token_hash: 'one-time-test-token',
          type: 'email_change',
        });
        await route.fulfill({ status: 202, json: { status: 'second_confirmation_required' } });
        return;
      }
      await route.fulfill({ status: 401, json: {} });
    });
    const suffix =
      source === 'redirect'
        ? '#message=Confirmation%20link%20accepted&sb=1'
        : '?token_hash=one-time-test-token&type=email_change';
    await page.goto(`/auth/callback${suffix}`);
    await expect(page.getByRole('heading', { name: 'Подтвердите второй адрес' })).toBeVisible();
    await expect(page).toHaveURL(/\/auth\/callback$/);
    expect(sessionExchanges).toEqual([]);
    await expect(page.getByRole('link', { name: 'В настройки' })).toBeVisible();
  });
}
