import Link from 'next/link';
import type { Order } from '@/types/domain';
import {orderStatus} from '@/lib/order-status';
import styles from './order-card.module.css';
export function OrderCard({ order }: { order: Order }) {
  const money = new Intl.NumberFormat('ru-RU');
  return <article className="card stack"><div className={styles.heading}><h3><Link href={`/orders/${order.id}`}>{order.title}</Link></h3><span className="badge">{orderStatus[order.status]}</span></div><p className={styles.budget}>{money.format(order.budgetMin)} – {money.format(order.budgetMax)} ₸</p><p className="small muted">{order.companyName} · {order.city} · До {new Date(`${order.deadline}T12:00:00Z`).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', timeZone: 'UTC' })}</p><p className="muted">{order.description}</p><div className={styles.bottom}><Link className={`button ${styles.action}`} href={`/orders/${order.id}`}>Посмотреть заказ</Link><span className="small muted">{order.applicationsCount} откликов · сегодня</span></div></article>;
}
