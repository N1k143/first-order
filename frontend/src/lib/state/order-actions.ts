import type { AppState, Order } from '@/types/domain';
export const orderLabels: Record<Order['status'], string> = {
  draft: 'Черновик',
  open: 'Приём откликов',
  in_progress: 'В работе',
  completed: 'Завершён',
};
export const applicationLabels = {
  pending: 'На рассмотрении',
  selected: 'Вы исполнитель',
  rejected: 'Выбран другой исполнитель',
  withdrawn: 'Отозван',
  completed: 'Завершён',
};
export function validateOrder(
  order: Pick<Order, 'title' | 'description' | 'budgetMin' | 'budgetMax' | 'deadline'>,
  today = new Date().toISOString().slice(0, 10),
) {
  if (order.title.trim().length < 5) return 'Название должно содержать не менее 5 символов.';
  if (order.description.trim().length < 20) return 'Опишите задачу подробнее: минимум 20 символов.';
  if (
    !Number.isFinite(order.budgetMin) ||
    !Number.isFinite(order.budgetMax) ||
    order.budgetMin <= 0 ||
    order.budgetMax < order.budgetMin
  )
    return 'Укажите корректный бюджет: верхняя граница не может быть меньше нижней.';
  if (
    !/^\d{4}-\d{2}-\d{2}$/.test(order.deadline) ||
    !Number.isFinite(Date.parse(order.deadline)) ||
    order.deadline < today
  )
    return 'Выберите сегодняшнюю или будущую дату.';
  return '';
}
export function selectContractor(s: AppState, orderId: string, applicationId: string): AppState {
  const order = s.orders.find((o) => o.id === orderId),
    app = s.applications.find((a) => a.id === applicationId && a.orderId === orderId);
  if (
    !order ||
    order.companyId !== s.company.id ||
    order.status !== 'open' ||
    !app ||
    app.status !== 'pending'
  )
    throw new Error('Отклик больше недоступен для выбора.');
  return {
    ...s,
    orders: s.orders.map((o) =>
      o.id === orderId ? { ...o, status: 'in_progress', contractorId: app.profileId } : o,
    ),
    applications: s.applications.map((a) =>
      a.orderId === orderId && a.status === 'pending'
        ? { ...a, status: a.id === applicationId ? 'selected' : 'rejected' }
        : a,
    ),
  };
}
export function withdrawApplication(s: AppState, id: string): AppState {
  const app = s.applications.find((a) => a.id === id);
  if (!app || app.profileId !== s.profile.id || app.status !== 'pending')
    throw new Error('Отозвать можно только свой отклик на рассмотрении.');
  return {
    ...s,
    applications: s.applications.map((a) => (a.id === id ? { ...a, status: 'withdrawn' } : a)),
    orders: s.orders.map((o) =>
      o.id === app.orderId ? { ...o, applicationsCount: Math.max(0, o.applicationsCount - 1) } : o,
    ),
  };
}
