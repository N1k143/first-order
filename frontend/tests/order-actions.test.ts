import assert from 'node:assert/strict';
import test from 'node:test';
import { initialState } from '../src/data/demo';
import {
  validateOrder,
  selectContractor,
  withdrawApplication,
} from '../src/lib/state/order-actions';

test('order validation rejects inverted budgets and dates in the past', () => {
  const order = { ...initialState.orders[0], deadline: '2026-12-01' };
  assert.equal(validateOrder(order, '2026-11-18'), '');
  assert.match(validateOrder({ ...order, budgetMax: 1 }, '2026-11-18'), /бюджет/);
  assert.match(validateOrder({ ...order, deadline: '2026-01-01' }, '2026-11-18'), /дату/);
  assert.match(validateOrder({ ...order, description: ' ' }, '2026-11-18'), /подробнее/);
});

test('choosing a contractor changes the order and rejects competing pending offers', () => {
  const result = selectContractor(structuredClone(initialState), 'coffee-website', 'anna-coffee');
  assert.equal(result.orders[0].status, 'in_progress');
  assert.equal(result.orders[0].selectedApplicationId, 'anna-coffee');
  assert.equal(result.applications.find((a) => a.id === 'my-coffee')?.status, 'rejected');
  assert.throws(() => selectContractor(result, 'coffee-website', 'my-coffee'));
  assert.equal(initialState.orders[0].status, 'open');
});

test('withdrawal is restricted to own pending application and decrements count once', () => {
  const result = withdrawApplication(structuredClone(initialState), 'my-coffee');
  assert.equal(result.orders[0].applicationsCount, initialState.orders[0].applicationsCount - 1);
  assert.throws(() => withdrawApplication(result, 'my-coffee'));
  assert.throws(() => withdrawApplication(initialState, 'anna-coffee'));
  assert.throws(() => withdrawApplication(initialState, 'my-school'));
});
