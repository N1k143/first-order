import { categories } from '@/data/demo';
import type { AppState, Company, Order } from '@/types/domain';

type LegacyState = Omit<AppState, 'history' | 'orders' | 'company'> & {
  history: (AppState['history'][number] | string)[];
  orders: (Order & { contractorId?: string })[];
  company: Company & { description?: string };
};

/** Upgrade persisted demo records without resetting user edits. */
export function migrateDemoState(value: LegacyState): AppState {
  const company = { ...value.company };
  delete company.description;
  company.categoryId ??= categories.find((item) => item.name === company.category)?.id ?? null;
  return {
    ...value,
    company,
    orders: value.orders.map((old) => {
      const order = { ...old };
      delete order.contractorId;
      return {
        ...order,
        budgetMin: order.budgetMin ?? null,
        budgetMax: order.budgetMax ?? null,
        deadline: order.deadline || null,
        visibility: order.visibility ?? order.status !== 'draft',
        selectedApplicationId:
          order.selectedApplicationId ??
          value.applications.find(
            (app) => app.orderId === order.id && app.profileId === old.contractorId,
          )?.id ??
          null,
      };
    }),
    applications: value.applications.map((app) => ({
      ...app,
      price: app.price ?? null,
      deadline: app.deadline || null,
    })),
    contacts: value.contacts.map((contact) => ({
      ...contact,
      contactedAt: contact.contactedAt ?? null,
    })),
    history: value.history.map((item, index) =>
      typeof item === 'string'
        ? {
            id: `legacy-${index}`,
            moderator_id: '',
            company_id: null,
            order_id: null,
            action: 'legacy_record',
            reason: item,
            created_at: '',
          }
        : item,
    ),
  };
}
