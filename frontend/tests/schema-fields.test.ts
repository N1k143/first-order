import assert from 'node:assert/strict';
import test from 'node:test';
import {
  applicationFields,
  companyFields,
  contactFields,
  optionalDate,
  optionalNumber,
  orderFields,
} from '../src/lib/api/form-fields';
import { initialState } from '../src/data/demo';
import { migrateDemoState } from '../src/lib/state/migrate';
import { validateOrder } from '../src/lib/state/order-actions';
import { budgetLabel, dateLabel, money } from '../src/lib/format';

function form(values: Record<string, string>) {
  const result = new FormData();
  for (const [key, value] of Object.entries(values)) result.set(key, value);
  return result;
}

test('nullable numeric and date fields preserve zero and reject invalid input', () => {
  assert.equal(optionalNumber(''), null);
  assert.equal(optionalNumber('0'), 0);
  assert.equal(optionalNumber('12.5'), 12.5);
  for (const value of ['-1', 'NaN', 'Infinity']) assert.throws(() => optionalNumber(value));
  assert.equal(optionalDate(''), null);
  assert.equal(optionalDate('2028-02-29'), '2028-02-29');
  for (const value of ['2027-02-29', 'invalid', '2026-13-01'])
    assert.throws(() => optionalDate(value));
  assert.deepEqual(applicationFields(form({ letter: ' Текст ' })), {
    message: 'Текст',
    proposed_price: null,
    proposed_deadline: null,
  });
  assert.deepEqual(orderFields(form({ title: 'Заказ', description: 'Описание' })), {
    title: 'Заказ',
    description: 'Описание',
    budget_min: null,
    budget_max: null,
    deadline: null,
  });
});

test('company payload excludes nonexistent and privileged fields', () => {
  const payload = companyFields(
    form({
      name: ' Компания ',
      city: 'Астана',
      description: 'нет в БД',
      visibility: 'public',
      website_status: 'found',
    }),
  );
  assert.equal(payload.category_id, null);
  assert.equal(payload.name, 'Компания');
  assert.equal('description' in payload, false);
  assert.equal('visibility' in payload, false);
  assert.equal('website_status' in payload, false);
});

test('saving contact notes preserves an unchanged timestamp and supports clearing it', () => {
  const timestamp = '2026-11-18T07:32:18.000Z';
  assert.equal(
    contactFields(form({ contacted_at: '2026-11-18' }), timestamp).contacted_at,
    timestamp,
  );
  assert.equal(contactFields(form({ contacted_at: '' }), timestamp).contacted_at, null);
  assert.equal(contactFields(form({ note: ' ' })).notes, null);
});

test('orders allow optional and one-sided budgets without invalid dates in UI', () => {
  const order = { ...initialState.orders[0], budgetMin: null, budgetMax: null, deadline: null };
  assert.equal(validateOrder(order), '');
  assert.equal(validateOrder({ ...order, budgetMin: 0 }), '');
  assert.equal(validateOrder({ ...order, budgetMax: 500 }), '');
  assert.match(validateOrder({ ...order, deadline: '2027-02-29' }, '2026-01-01'), /дату/);
  assert.match(budgetLabel(null, null), /договорённости/);
  assert.match(money(0), /0/);
  assert.equal(dateLabel('invalid'), 'Не указано');
});

test('legacy demo migration preserves edits, avatars, zero values and selected applications', () => {
  const original = structuredClone(initialState);
  original.profile.avatarUrl = 'https://example.com/avatar.png';
  original.orders[0].budgetMin = 0;
  const legacy = {
    ...original,
    company: { ...original.company, description: 'old' },
    orders: original.orders.map((order, index) => ({
      ...order,
      contractorId: index === 0 ? 'anna' : undefined,
    })),
    history: ['Старая запись'],
  };
  legacy.orders[0].contractorId = original.applications.find(
    (app) => app.id === 'anna-coffee',
  )!.profileId;
  const result = migrateDemoState(legacy);
  assert.equal(result.orders[0].selectedApplicationId, 'anna-coffee');
  assert.equal(result.orders[0].budgetMin, 0);
  assert.equal(result.profile.avatarUrl, original.profile.avatarUrl);
  assert.equal(result.history[0].reason, 'Старая запись');
  assert.equal('description' in result.company, false);
  assert.equal('contractorId' in result.orders[0], false);
  assert.equal(legacy.company.description, 'old');
});
