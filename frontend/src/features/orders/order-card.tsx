import Link from 'next/link';
import { budgetLabel, deadlineLabel } from '@/lib/format';
import type { Order } from '@/types/domain';
import styles from './order-card.module.css';
import { orderStatus } from '@/lib/order-status';
export function OrderCard({ order }: { order: Order }) {
  return (
    <article className="card stack">
      <div className={styles.heading}>
        <h3>
          <Link href={`/orders/${order.id}`}>{order.title}</Link>
        </h3>
        <span className="badge">{orderStatus[order.status]}</span>
      </div>
      <p className={styles.budget}>{budgetLabel(order.budgetMin, order.budgetMax)}</p>
      <p className="small muted">
        {order.companyName} · {order.city} · {deadlineLabel(order.deadline)}
      </p>
      <p className="muted">{order.description}</p>
      <div className={styles.bottom}>
        <Link className={`button ${styles.action}`} href={`/orders/${order.id}`}>
          Посмотреть заказ
        </Link>
        <span className="small muted">{order.applicationsCount} откликов</span>
      </div>
    </article>
  );
}
