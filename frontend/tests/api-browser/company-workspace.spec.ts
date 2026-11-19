import { test, expect, type Page } from '@playwright/test';
import type { Company } from '../../src/types/domain';
import type {
  CompanyOrderRow,
  CompanyApplicationRow,
  CompanyMemberRow,
} from '../../src/features/workspace/workspace-data';

const ownerID = '11111111-1111-4111-8111-111111111111';
const companyA = '22222222-2222-4222-8222-222222222222';
const companyB = '33333333-3333-4333-8333-333333333333';
const otherOwner = '44444444-4444-4444-8444-444444444444';
const newMember = '55555555-5555-4555-8555-555555555555';
const categoryID = '66666666-6666-4666-8666-666666666666';
const applicationID = '77777777-7777-4777-8777-777777777777';
function order(id: string, companyId = companyA, status = 'open'): CompanyOrderRow {
  return {
    id,
    company_id: companyId,
    created_by: ownerID,
    selected_application_id: null,
    company_name: companyId === companyA ? 'Компания Альфа' : 'Компания Бета',
    city: 'Астана',
    title: `Задача ${id}`,
    description: 'Реальное описание задачи компании из тестового API.',
    budget_min: null,
    budget_max: null,
    deadline: null,
    status,
    visibility: status !== 'draft',
    created_at: '2026-11-20T12:00:00Z',
    updated_at: '2026-11-20T12:00:00Z',
    applications_count: 0,
  };
}
function member(
  companyId: string,
  userId: string,
  role: 'owner' | 'representative',
  name: string,
): CompanyMemberRow {
  return {
    company_id: companyId,
    user_id: userId,
    member_role: role,
    name,
    created_at: '2026-11-20T12:00:00Z',
  };
}
async function mockWorkspace(page: Page) {
  const model = {
    companies: {
      [companyA]: {
        id: companyA,
        name: 'Компания Альфа',
        initials: 'КА',
        city: 'Астана',
        category: 'Разработка',
        categoryId: categoryID,
        address: 'Адрес Альфа',
        websiteStatus: 'needs_review',
        checkedAt: null,
        phone: '+70000000001',
        email: 'alpha@example.test',
        website: 'https://alpha.example.test',
        sourceUrl: null,
      },
      [companyB]: {
        id: companyB,
        name: 'Компания Бета',
        initials: 'КБ',
        city: 'Алматы',
        category: 'Разработка',
        categoryId: categoryID,
        address: 'Адрес Бета',
        websiteStatus: 'needs_review',
        checkedAt: null,
        phone: '+70000000002',
        email: 'beta@example.test',
        website: '',
        sourceUrl: null,
      },
    } as Record<string, Company>,
    orders: [order('order-alpha'), order('order-beta', companyB)],
    applications: [] as CompanyApplicationRow[],
    members: {
      [companyA]: [
        member(companyA, ownerID, 'owner', 'Владелец'),
        member(companyA, otherOwner, 'owner', 'Другой владелец'),
      ],
      [companyB]: [
        member(companyB, ownerID, 'representative', 'Представитель'),
        member(companyB, otherOwner, 'owner', 'Владелец Бета'),
      ],
    } as Record<string, CompanyMemberRow[]>,
    companyError: false,
    lastOwnerRace: false,
    requests: [] as { method: string; path: string; body?: Record<string, unknown> }[],
    unexpected: [] as string[],
  };
  await page.route('**/api/**', async (route) => {
    const request = route.request(),
      url = new URL(request.url()),
      path = url.pathname.replace(/^\/api/, ''),
      method = request.method();
    const body = request.postData()
      ? (request.postDataJSON() as Record<string, unknown>)
      : undefined;
    model.requests.push({ method, path: url.pathname + url.search, body });
    const json = (value: unknown, status = 200) => route.fulfill({ status, json: value });
    const fail = (message: string, status = 400) => json({ error: { message } }, status);
    const paged = <T>(rows: T[]) => {
      const pageNumber = Number(url.searchParams.get('page') || 1),
        limit = Number(url.searchParams.get('limit') || 100);
      return json({
        items: rows.slice((pageNumber - 1) * limit, pageNumber * limit),
        total: rows.length,
        page: pageNumber,
        limit,
      });
    };
    if (path === '/me')
      return json({
        id: ownerID,
        name: 'Владелец',
        email: 'owner@example.test',
        role: 'company',
        bio: '',
        skills: [],
        companies: Object.entries(model.members).flatMap(([companyId, members]) => {
          const mine = members.find((item) => item.user_id === ownerID);
          return mine
            ? [{ companyId, memberRole: mine.member_role, name: model.companies[companyId].name }]
            : [];
        }),
      });
    if (path === '/auth/refresh') return json({});
    if (['/me/favorites', '/me/contacts', '/me/applications', '/me/portfolio'].includes(path))
      return json([]);
    if (path === '/categories') return paged([{ id: categoryID, name: 'Разработка' }]);
    const companyMatch = path.match(/^\/companies\/([^/]+)$/);
    if (companyMatch) {
      const id = companyMatch[1],
        company = model.companies[id];
      if (!company) return fail('Компания не найдена', 404);
      if (method === 'PATCH') {
        if (model.companyError) return fail('Недостаточно прав для изменения компании', 403);
        Object.assign(company, {
          name: body?.name,
          city: body?.city,
          categoryId: body?.category_id,
          address: body?.address || '',
          phone: body?.phone || '',
          email: body?.email || '',
          website: body?.website_url || '',
          sourceUrl: body?.source_url,
        });
      }
      return json(company);
    }
    const memberMatch = path.match(/^\/v1\/companies\/([^/]+)\/members(?:\/([^/]+))?$/);
    if (memberMatch) {
      const id = memberMatch[1],
        members = model.members[id] || [],
        actor = members.find((item) => item.user_id === ownerID);
      if (method === 'GET') return paged(members);
      if (actor?.member_role !== 'owner')
        return fail('Только владелец может менять участников', 403);
      if (method === 'POST') {
        const userId = String(body?.user_id);
        if (members.some((item) => item.user_id === userId))
          return fail('Пользователь уже состоит в компании', 409);
        const added = member(
          id,
          userId,
          body?.member_role === 'owner' ? 'owner' : 'representative',
          'Новый участник',
        );
        members.push(added);
        return json(added, 201);
      }
      if (method === 'DELETE') {
        const target = members.find((item) => item.user_id === memberMatch[2]);
        if (model.lastOwnerRace) {
          model.members[id] = members.filter((item) => item.user_id === ownerID);
          return fail('Нельзя удалить последнего владельца компании', 409);
        }
        if (
          target?.member_role === 'owner' &&
          members.filter((item) => item.member_role === 'owner').length === 1
        )
          return fail('Нельзя удалить последнего владельца компании', 409);
        model.members[id] = members.filter((item) => item.user_id !== memberMatch[2]);
        return route.fulfill({ status: 204 });
      }
    }
    if (path === '/v1/orders')
      return paged(
        model.orders.filter((item) => item.company_id === url.searchParams.get('company_id')),
      );
    const applicationMatch = path.match(/^\/v1\/orders\/([^/]+)\/applications$/);
    if (applicationMatch)
      return paged(model.applications.filter((item) => item.order_id === applicationMatch[1]));
    const choose = path.match(/^\/orders\/([^/]+)\/select-application$/);
    if (choose && method === 'POST') {
      const selectedOrder = model.orders.find((item) => item.id === choose[1]);
      if (!selectedOrder) return fail('Заказ не найден', 404);
      Object.assign(selectedOrder, {
        status: 'contractor_selected',
        selected_application_id: String(body?.application_id),
      });
      model.applications
        .filter((item) => item.order_id === choose[1])
        .forEach((item) =>
          Object.assign(item, {
            status: item.id === body?.application_id ? 'accepted' : 'rejected',
          }),
        );
      return json({});
    }
    if (path === '/orders') return json([]);
    model.unexpected.push(`${method} ${path}`);
    return fail('Unexpected mock route', 404);
  });
  return model;
}

test('company overview reads every page and switches companies without leaking prior rows', async ({
  page,
}) => {
  const model = await mockWorkspace(page);
  model.orders = [
    ...Array.from({ length: 101 }, (_, index) =>
      order(`alpha-${index}`, companyA, index === 100 ? 'draft' : 'open'),
    ),
    order('only-beta', companyB, 'completed'),
  ];
  await page.goto('/workspace');
  const stats = page.getByTestId('company-statistics');
  await expect(
    stats.locator('.stat').filter({ hasText: 'Открытых заказов' }).locator('strong'),
  ).toHaveText('100');
  await expect(
    stats.locator('.stat').filter({ hasText: 'Черновиков' }).locator('strong'),
  ).toHaveText('1');
  expect(
    model.requests.some(
      (request) => request.path.includes('/v1/orders?') && request.path.includes('page=2'),
    ),
  ).toBe(true);
  await page.locator('select[name="workspace_company"]').selectOption(companyB);
  await expect(page.getByTestId('company-order')).toHaveCount(1);
  await expect(page.getByTestId('company-order')).toContainText('Задача only-beta');
  await expect(page.getByText('Задача alpha-0', { exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.locator('select[name="workspace_company"]')).toHaveValue(companyB);
  await expect(page.getByTestId('company-order')).toContainText('Задача only-beta');
  expect(model.unexpected).toEqual([]);
});

test('order search and status filters apply only to the selected company', async ({ page }) => {
  const model = await mockWorkspace(page);
  model.orders = [
    order('open-alpha'),
    order('draft-alpha', companyA, 'draft'),
    order('foreign-beta', companyB, 'draft'),
  ];
  await page.goto('/workspace/orders');
  await expect(page.getByTestId('company-order')).toHaveCount(2);
  await page.getByLabel('Статус заказа').selectOption('draft');
  await expect(page.getByTestId('company-order')).toHaveCount(1);
  await expect(page.getByTestId('company-order')).toContainText('draft-alpha');
  await page.getByLabel('Поиск моих заказов').fill('нет такой задачи');
  await expect(page.getByRole('heading', { name: 'Заказы не найдены' })).toBeVisible();
  await expect(page.getByText('foreign-beta', { exact: false })).toHaveCount(0);
});

test('company profile PATCH targets selected membership and preserves edits on server failure', async ({
  page,
}) => {
  const model = await mockWorkspace(page);
  await page.goto('/workspace/profile');
  await page.locator('select[name="workspace_company"]').selectOption(companyB);
  await expect(page.getByLabel('Название компании', { exact: true })).toHaveValue('Компания Бета');
  await page.getByLabel('Название компании', { exact: true }).fill('Бета обновлена');
  await page.getByLabel('Телефон', { exact: true }).fill('+70000000999');
  await page.getByLabel('Сфера деятельности').selectOption('');
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await expect(page.getByRole('status')).toContainText('Данные компании сохранены.');
  const saved = model.requests.find((request) => request.method === 'PATCH');
  expect(saved?.path).toBe(`/api/companies/${companyB}`);
  expect(saved?.body).toMatchObject({
    name: 'Бета обновлена',
    category_id: null,
    phone: '+70000000999',
    website_url: null,
  });
  expect(Object.keys(saved?.body || {}).sort()).toEqual([
    'address',
    'category_id',
    'city',
    'email',
    'name',
    'phone',
    'source_url',
    'website_url',
  ]);
  expect(model.companies[companyA].name).toBe('Компания Альфа');
  await page.reload();
  await expect(page.getByLabel('Название компании', { exact: true })).toHaveValue('Бета обновлена');
  model.companyError = true;
  await page.getByLabel('Название компании', { exact: true }).fill('Несохранённое имя');
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await expect(page.locator('main').getByRole('alert')).toContainText('Недостаточно прав');
  await expect(page.getByLabel('Название компании', { exact: true })).toHaveValue(
    'Несохранённое имя',
  );
  expect(model.companies[companyB].name).toBe('Бета обновлена');
});

test('owners add registered IDs and remove members; representatives only read', async ({
  page,
}) => {
  const model = await mockWorkspace(page);
  await page.goto('/workspace/members');
  await expect(page.getByTestId('company-member')).toHaveCount(2);
  await page.getByLabel('ID пользователя', { exact: true }).fill(newMember);
  await page.getByRole('button', { name: 'Добавить участника', exact: true }).click();
  await expect(page.getByTestId('company-member')).toHaveCount(3);
  expect(model.requests.find((request) => request.method === 'POST')?.body).toEqual({
    user_id: newMember,
    member_role: 'representative',
  });
  const added = page.getByTestId('company-member').filter({ hasText: newMember });
  await added.getByRole('button', { name: 'Удалить участника' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Удалить участника' }).click();
  await expect(page.getByTestId('company-member')).toHaveCount(2);
  await page.locator('select[name="workspace_company"]').selectOption(companyB);
  await expect(
    page.getByText('Добавлять и удалять участников может только владелец компании.'),
  ).toBeVisible();
  await expect(page.getByLabel('ID пользователя', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Удалить участника' })).toHaveCount(0);
  expect(model.members[companyA].some((item) => item.user_id === newMember)).toBe(false);
});

test('last-owner server conflict is shown and stale membership controls are removed', async ({
  page,
}) => {
  const model = await mockWorkspace(page);
  await page.goto('/workspace/members');
  const target = page.getByTestId('company-member').filter({ hasText: ownerID });
  await target.getByRole('button', { name: 'Удалить участника' }).click();
  model.lastOwnerRace = true;
  await page.getByRole('dialog').getByRole('button', { name: 'Удалить участника' }).click();
  await expect(page.locator('main').getByRole('alert')).toContainText(
    'Нельзя удалить последнего владельца компании',
  );
  expect(model.members[companyA].map((item) => item.user_id)).toEqual([ownerID]);
  await expect(page.getByRole('button', { name: 'Удалить участника' })).toHaveCount(0);
  await expect(page.getByText('Последний владелец', { exact: true })).toBeVisible();
});

test('company-wide applications select on the correct order and refresh accepted status', async ({
  page,
}) => {
  const model = await mockWorkspace(page);
  model.orders[0].applications_count = 1;
  model.applications = [
    {
      id: applicationID,
      order_id: model.orders[0].id,
      developer_id: newMember,
      developer_name: 'Разработчик',
      order_title: model.orders[0].title,
      company_name: 'Компания Альфа',
      message: 'Предложение разработчика для выбранной компании',
      proposed_price: null,
      proposed_deadline: null,
      status: 'submitted',
      created_at: '2026-11-20T12:00:00Z',
      updated_at: '2026-11-20T12:00:00Z',
    },
  ];
  await page.goto('/workspace/applications');
  await page
    .getByTestId('application-card')
    .getByRole('button', { name: 'Выбрать исполнителя' })
    .click();
  await page.getByRole('dialog').getByRole('button', { name: 'Подтвердить выбор' }).click();
  await expect(page.getByTestId('application-card')).toContainText('Исполнитель выбран');
  expect(
    model.requests.find((request) => request.path.endsWith('/select-application'))?.body,
  ).toEqual({ application_id: applicationID });
  expect(model.orders[0].selected_application_id).toBe(applicationID);
  await page.locator('select[name="workspace_company"]').selectOption(companyB);
  await expect(page.getByRole('heading', { name: 'Откликов пока нет' })).toBeVisible();
  await expect(page.getByText('Предложение разработчика для выбранной компании')).toHaveCount(0);
  expect(model.unexpected).toEqual([]);
});
