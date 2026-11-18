import { test, expect } from '@playwright/test';

async function useCompany(page: import('@playwright/test').Page) {
  await page.goto('/workspace/orders/new');
  await page.getByRole('button', { name: 'Открыть демо компании' }).click();
  await expect(page.getByLabel('Название заказа')).toBeVisible();
}

test('company creates a draft, previews and publishes; public detail uses entered text', async ({
  page,
}, testInfo) => {
  await useCompany(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: testInfo.outputPath('order-editor-desktop.png'), fullPage: true });
  await page.getByLabel('Название заказа').fill('Каталог мебели для мастерской');
  await page
    .getByLabel('Описание задачи')
    .fill('Создать каталог деревянной мебели с описанием материалов и формой заявки.');
  await page.getByLabel('Бюджет от').fill('90000');
  await page.getByLabel('Бюджет до').fill('80000');
  await page.getByLabel('Желаемый срок').fill('2099-12-01');
  await page.getByRole('button', { name: 'Предпросмотр', exact: true }).click();
  await expect(page.locator('main [role=alert]')).toContainText('верхняя граница');
  await page.getByLabel('Бюджет до').fill('140000');
  await page.getByRole('button', { name: 'Предпросмотр', exact: true }).click();
  await expect(page).toHaveURL(/\/preview$/);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Каталог мебели для мастерской' })).toBeVisible();
  await page.getByRole('button', { name: 'Опубликовать заказ' }).click();
  await page.getByRole('link', { name: 'Открыть публичную страницу' }).click();
  await expect(
    page.getByText('Создать каталог деревянной мебели с описанием материалов и формой заявки.'),
  ).toBeVisible();
  await expect(page.getByText('Меню с категориями и ценами')).toHaveCount(0);
});

test('application can be submitted once and withdrawn from the dashboard', async ({ page }) => {
  await page.goto('/orders/interior-website/apply');
  await page
    .getByLabel('Сопроводительное письмо')
    .fill('Создам адаптивное портфолио студии и удобную форму обратной связи.');
  await page.getByLabel('Стоимость').fill('160000');
  await page.getByLabel('Готово к дате').fill('2099-12-01');
  await page.getByRole('button', { name: 'Отправить отклик' }).click();
  await expect(page.getByRole('button', { name: 'Отправить отклик' })).toBeDisabled();
  await page.goto('/applications');
  const application = page
    .locator('article')
    .filter({ has: page.getByRole('heading', { name: 'Сайт студии интерьеров' }) });
  await application.getByRole('button', { name: 'Отозвать отклик' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Подтвердить' }).click();
  await expect(application.getByText('Отозван', { exact: true })).toBeVisible();
  await page.reload();
  await expect(application.getByText('Отозван', { exact: true })).toBeVisible();
});

test('profile edits and portfolio persist; logout protects personal pages', async ({ page }) => {
  await page.goto('/profile/edit');
  await page.getByLabel('Имя и фамилия').fill('Тестовый Разработчик');
  await page.getByRole('button', { name: 'Сохранить изменения' }).click();
  await page.goto('/profile/portfolio');
  await page.getByLabel('Название проекта').fill('Мой учебный проект');
  await page.getByLabel('Описание и ваша роль').fill('Разработал интерфейс и адаптивную верстку.');
  await page.getByRole('button', { name: 'Добавить в портфолио' }).click();
  await page.goto('/profile');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Тестовый Разработчик');
  await expect(page.getByRole('heading', { name: 'Мой учебный проект' }).first()).toBeVisible();
  await page.goto('/settings');
  await page.getByRole('button', { name: 'Выйти из аккаунта' }).click();
  await page.goto('/profile/edit');
  await expect(page.getByRole('heading', { name: 'Войдите, чтобы продолжить' })).toBeVisible();
  await expect(page.getByLabel('Имя и фамилия')).toHaveCount(0);
});

test('company selects an executor and completes an order', async ({ page }) => {
  await useCompany(page);
  await page.goto('/workspace/orders/coffee-website/applications');
  const anna = page
    .locator('article')
    .filter({ has: page.getByRole('heading', { name: 'Анна Соколова' }) });
  await anna.getByRole('button', { name: 'Выбрать исполнителя' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Подтвердить' }).click();
  await expect(anna.getByText('Исполнитель выбран')).toBeVisible();
  await page.goto('/workspace/orders/coffee-website');
  await page.getByRole('button', { name: 'Завершить заказ' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Подтвердить' }).click();
  await expect(page.getByText('Завершён', { exact: true })).toBeVisible();
});

test('catalog search survives reload and health check uses the real proxy route', async ({
  page,
}) => {
  await page.goto('/companies');
  await page.getByRole('textbox', { name: 'Название компании или сфера' }).fill('Forma');
  await page.getByRole('button', { name: 'Найти', exact: true }).click();
  await expect(page).toHaveURL(/q=Forma/);
  await page.reload();
  await expect(page.locator('article')).toHaveCount(1);
  await expect(page.locator('article')).toContainText('Forma Studio');
  await page.route('**/api/health', (route) =>
    route.fulfill({ contentType: 'text/plain', body: 'OK' }),
  );
  await page.goto('/settings');
  await page.getByRole('button', { name: 'Проверить связь с сервером' }).click();
  await expect(page.getByRole('status')).toContainText('Сервер доступен');
});

test('new pages fit mobile screens and have no runtime errors', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.setViewportSize({ width: 390, height: 844 });
  for (const route of [
    '/dashboard',
    '/saved',
    '/contacts',
    '/profile',
    '/profile/edit',
    '/profile/portfolio',
    '/settings',
    '/login',
    '/orders',
    '/companies',
    '/about',
    '/support',
    '/privacy',
    '/terms',
  ]) {
    const response = await page.goto(route);
    expect(response?.status(), route).toBe(200);
    await expect(page.locator('h1')).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth), route).toBe(390);
  }
  await useCompany(page);
  await page.screenshot({ path: testInfo.outputPath('order-editor-mobile.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
  for (const route of [
    '/workspace',
    '/workspace/orders',
    '/workspace/profile',
    '/workspace/members',
    '/workspace/applications',
  ]) {
    await page.goto(route);
    expect(await page.evaluate(() => document.documentElement.scrollWidth), route).toBe(390);
  }
  expect(errors).toEqual([]);
});

test('registration checks password confirmation and creates a new demo company', async ({
  page,
}) => {
  await page.goto('/register/developer');
  await page.getByLabel('Имя и фамилия').fill('Новый Разработчик');
  await page.getByLabel('Email', { exact: true }).fill('developer@example.com');
  await page.getByLabel('Пароль', { exact: true }).fill('example-password');
  await page.getByLabel('Повторите пароль').fill('different-password');
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page.locator('main [role=alert]')).toContainText('Пароли не совпадают');
  await page.goto('/register/company');
  await page.getByLabel('Ваше имя').fill('Представитель компании');
  await page.getByLabel('Рабочий email').fill('company@example.com');
  await page.getByLabel('Пароль', { exact: true }).fill('example-password');
  await page.getByRole('button', { name: /Компании нет в каталоге/ }).click();
  await page.getByLabel('Компания', { exact: true }).fill('Новая мастерская');
  await page.getByLabel('Город компании').fill('Алматы');
  await page.getByRole('button', { name: 'Создать аккаунт компании' }).click();
  await expect(page).toHaveURL(/verify-email/);
  await page.getByRole('link', { name: 'Продолжить в деморежиме' }).click();
  await page.goto('/workspace/profile');
  await expect(page.getByLabel('Название компании')).toHaveValue('Новая мастерская');
  await expect(page.getByLabel('Город', { exact: true })).toHaveValue('Алматы');
});

test('company notes and favorites persist; correction reaches moderation', async ({ page }) => {
  await page.goto('/companies/dental-plus');
  await page.getByRole('button', { name: 'Сохранить компанию' }).click();
  await page.goto('/saved');
  await expect(page.getByRole('heading', { name: 'Dental Plus' })).toBeVisible();
  await page.goto('/companies/dental-plus/contacts');
  await page.getByLabel('Моя заметка').fill('Уточнить необходимость формы записи на приём.');
  await page.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await page.goto('/contacts');
  await expect(page.getByText('Уточнить необходимость формы записи на приём.')).toBeVisible();
  await page.goto('/companies/dental-plus/correction');
  await page
    .getByLabel('Что нужно исправить')
    .fill('Проверить актуальность адреса медицинского центра.');
  await page.getByRole('button', { name: 'Отправить на проверку' }).click();
  await expect(page.getByRole('status')).toContainText('очередь модерации');
  await page.goto('/moderation');
  await page.getByRole('button', { name: 'Открыть демо модератора' }).click();
  const correction = page
    .locator('form')
    .filter({ has: page.getByRole('heading', { name: 'Dental Plus' }) });
  await correction
    .getByLabel('Результат проверки')
    .fill('Адрес проверен, актуальные сведения подтверждены.');
  await correction.getByRole('button', { name: 'Подтвердить обращение' }).click();
  await page.goto('/moderation/history');
  await expect(page.getByText(/Dental Plus.*Обращение подтверждено/)).toBeVisible();
});
