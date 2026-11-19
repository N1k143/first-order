'use client';
import Link from 'next/link';
import { OrderCard } from '@/features/orders/order-card';
import { useAppState } from '@/lib/state/store';
import { useResource } from '@/lib/api/resources';
import { ResourceState } from '@/components/ui/primitives';
import styles from './home-page.module.css';
export function HomePage() {
  const state = useAppState();
  const orders = useResource(
    '/orders?status=open',
    state.orders.filter((o) => o.status === 'open' && o.visibility !== false),
  );
  const featuredOrder = orders.data?.[0];
  return (
    <>
      <section className={styles.hero}>
        <div className={`container ${styles.heroGrid}`}>
          <div className={styles.heroContent}>
            <p className={styles.eyebrow}>ПЕРВЫЙ КЛИЕНТ. НАСТОЯЩИЙ ОПЫТ.</p>
            <h1 className={styles.title}>
              Ваш первый заказ
              <br /> ближе, чем кажется.
            </h1>
            <p className="muted">
              Находите компании, которым нужен сайт, предлагайте помощь
              <br />и откликайтесь на задачи бизнеса.
            </p>
            <form action="/companies" className={styles.search}>
              <label className="sr-only" htmlFor="hero-search">
                Компания, сфера или задача
              </label>
              <input
                className="field"
                id="hero-search"
                name="q"
                placeholder="Компания, сфера или задача"
              />
              <button className="button" type="submit">
                Найти компании
              </button>
            </form>
            <p className="small muted">Например: кофейни, образование, сайты без онлайн-записи</p>
          </div>
          <aside className={styles.steps} aria-label="Как начать">
            <h2>
              Бизнесу нужен сайт.
              <br />
              Вам — первый проект.
            </h2>
            <hr />
            <p className={styles.step}>01 Найдите компанию</p>
            <p className={styles.step}>02 Предложите решение</p>
            <p className={styles.step}>03 Начните работать</p>
            <p className="small">
              Без сложного старта.
              <br />С понятной задачей и реальным клиентом.
            </p>
          </aside>
        </div>
      </section>
      <section className={`container ${styles.content}`} id="how-it-works">
        <div className="spread">
          <h2>Два способа найти работу</h2>
          <p className={`small muted ${styles.sectionHint}`}>Выберите подходящий вам</p>
        </div>
        <div className={styles.choices}>
          <article className="card stack">
            <h3>Компании без найденного сайта</h3>
            <p className="small muted">
              Изучите бизнес, проверьте контакты и предложите подходящее решение.
            </p>
            <Link href="/companies" className={`button ${styles.choiceButton}`}>
              Открыть каталог
            </Link>
          </article>
          <article className="card stack">
            <h3>Заказы от бизнеса</h3>
            <p className="small muted">
              Откликайтесь на опубликованные задачи с бюджетом и сроком.
            </p>
            <Link href="/orders" className={`button secondary ${styles.choiceButton}`}>
              Посмотреть заказы
            </Link>
          </article>
        </div>
        <div className={styles.orders}>
          <div className="stack">
            <h2>Свежие заказы</h2>
            {featuredOrder ? (
              <OrderCard order={featuredOrder} />
            ) : orders.loading || orders.error ? (
              <ResourceState error={orders.error} reload={orders.reload} />
            ) : (
              <p className="muted">Открытых заказов пока нет.</p>
            )}
          </div>
          <aside className={`card stack ${styles.business}`}>
            <h3>Вы представляете бизнес?</h3>
            <p className="small muted">
              Опишите задачу и сравните предложения разработчиков. Вы сами выбираете исполнителя.
            </p>
            <Link href="/workspace/orders/new" className={`button ${styles.businessButton}`}>
              Разместить заказ
            </Link>
          </aside>
        </div>
      </section>
    </>
  );
}
