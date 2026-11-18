import { test, expect } from '@playwright/test';

const screens = [
  '/register',
  '/register/developer',
  '/register/company',
  '/login',
  '/login/developer',
  '/login/company',
  '/forgot-password',
  '/reset-password',
  '/verify-email',
];

test('every auth page has contextual green panel on desktop and mobile', async ({
  page,
}, testInfo) => {
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const path of screens) {
      await page.goto(path);
      const panel = page.getByRole('complementary', { name: 'О возможностях FirstOrder' });
      await expect(panel).toBeVisible();
      expect(await panel.evaluate((element) => getComputedStyle(element).backgroundColor)).toBe(
        'rgb(36, 92, 69)',
      );
      expect(await page.evaluate(() => document.documentElement.scrollWidth), path).toBe(width);
    }
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('/login/company');
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: testInfo.outputPath('company-login.png'), fullPage: true });
});

test('role-specific login changes copy, demo role and recovery links', async ({ page }) => {
  await page.goto('/login/developer');
  await expect(page.getByRole('heading', { name: 'Следующий проект уже ближе.' })).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Роль для входа' })
    .getByRole('link', { name: 'Компания' })
    .click();
  await expect(page.getByRole('heading', { name: 'Ваша задача ждёт решения.' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Не помню пароль' })).toHaveAttribute(
    'href',
    '/forgot-password?role=company',
  );
  await page.getByLabel('Email', { exact: true }).fill('owner@example.com');
  await page.getByLabel('Пароль', { exact: true }).fill('demo-password');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page).toHaveURL(/\/workspace$/);
});

test('avatar upload, profile fields and removal survive page reload', async ({
  page,
}, testInfo) => {
  await page.goto('/profile/edit');
  await page.getByLabel('Имя и фамилия').fill('Мария Соколова');
  await page.getByLabel('О себе').fill('Разрабатываю интерфейсы для малого бизнеса.');
  // Use a browser-produced PNG so file decoding is also exercised.
  const pngBytes = await page.evaluate(() => {
    const canvas = document.createElement('canvas');
    canvas.width = 80;
    canvas.height = 80;
    const ctx = canvas.getContext('2d')!;
    ctx.fillStyle = '#245c45';
    ctx.fillRect(0, 0, 80, 80);
    return canvas.toDataURL('image/png').split(',')[1];
  });
  await page.getByLabel('Загрузить фото').setInputFiles({
    name: 'avatar.png',
    mimeType: 'image/png',
    buffer: Buffer.from(pngBytes, 'base64'),
  });
  await expect(page.getByRole('button', { name: 'Сохранить изменения' })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Удалить фото' })).toBeVisible();
  await page.getByRole('button', { name: 'Сохранить изменения' }).click();
  await page.reload();
  await expect(page.getByLabel('О себе')).toHaveValue(
    'Разрабатываю интерфейсы для малого бизнеса.',
  );
  await expect(page.getByLabel('Имя и фамилия')).toHaveValue('Мария Соколова');
  const avatar = page.locator('header a[aria-label="Мой профиль"]');
  await expect(avatar.locator('img')).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: testInfo.outputPath('profile-mobile.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  const bounds = await avatar.boundingBox();
  expect(bounds?.width).toBe(44);
  expect(bounds?.height).toBe(44);
  await page.getByRole('button', { name: 'Удалить фото' }).click();
  await page.getByRole('button', { name: 'Сохранить изменения' }).click();
  await expect(avatar).toHaveText('МС');
  await page.reload();
  await expect(avatar).toHaveText('МС');
});

test('broken avatar falls back to initials and invalid upload is rejected', async ({ page }) => {
  await page.route('https://example.com/missing.jpg', (route) =>
    route.fulfill({ status: 404, body: '' }),
  );
  await page.goto('/profile/edit');
  await page.getByLabel('Или ссылка на фото').fill('https://example.com/missing.jpg');
  await page.getByRole('button', { name: 'Сохранить изменения' }).click();
  await expect(page.locator('header a[aria-label="Мой профиль"]')).toHaveText('НА');
  await page
    .getByLabel('Загрузить фото')
    .setInputFiles({ name: 'bad.png', mimeType: 'image/png', buffer: Buffer.from('not an image') });
  await expect(page.locator('main [role=alert]')).toContainText('Не удалось открыть изображение');
});

test('company fields and existing portfolio items can be edited', async ({ page }) => {
  await page.goto('/workspace/profile');
  await page.getByRole('button', { name: 'Открыть демо компании' }).click();
  await page.getByLabel('Сфера деятельности').fill('Образование');
  await page.getByLabel('Адрес компании').fill('ул. Новая, 25');
  await page.getByLabel('Сайт компании').fill('https://example.com');
  await page.getByRole('button', { name: 'Сохранить изменения' }).click();
  await page.reload();
  await expect(page.getByLabel('Адрес компании')).toHaveValue('ул. Новая, 25');
  await expect(page.getByLabel('Сайт компании')).toHaveValue('https://example.com');
  await page.goto('/profile/portfolio');
  const project = page
    .locator('form')
    .filter({ has: page.getByRole('heading', { name: 'Сайт кофейни «Зерно»' }) });
  await project
    .getByLabel('Описание работы')
    .fill('Обновлённое описание: дизайн, разработка и запуск сайта.');
  await project.getByRole('button', { name: 'Сохранить работу' }).click();
  await page.reload();
  await expect(project.getByLabel('Описание работы')).toHaveValue(
    'Обновлённое описание: дизайн, разработка и запуск сайта.',
  );
});

test('profile editor matches editable columns and portfolio optional fields can be cleared', async ({
  page,
}) => {
  await page.goto('/profile/edit');
  await expect(page.getByLabel('Email', { exact: true })).toHaveCount(0);
  await expect(page.getByLabel('Город', { exact: true })).toHaveCount(0);
  await expect(
    page.locator('input[name=role],input[name=is_moderator],input[name=user_id]'),
  ).toHaveCount(0);
  await page.getByLabel('О себе').fill('');
  await page.getByLabel('Навыки через запятую').fill('');
  await page.getByLabel('Ссылка на GitHub').fill('');
  await page.getByRole('button', { name: 'Сохранить изменения' }).click();
  await page.reload();
  await expect(page.getByLabel('О себе')).toHaveValue('');
  await page.goto('/profile/portfolio');
  await page.getByLabel('Название проекта', { exact: true }).fill('Проект без описания');
  await page.getByRole('button', { name: 'Добавить в портфолио' }).click();
  await page.reload();
  const project = page
    .locator('form')
    .filter({ has: page.getByRole('heading', { name: 'Проект без описания' }) });
  await expect(project.getByLabel('Описание работы')).toHaveValue('');
  await expect(project.getByLabel('Адрес проекта')).toHaveValue('');
});
