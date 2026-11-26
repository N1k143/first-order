'use client';
import { useSearchParams, useRouter, usePathname } from 'next/navigation';
import { useRef } from 'react';
import { companies } from '@/data/demo';
import { useCategories } from '@/lib/api/categories';
import { useResource } from '@/lib/api/resources';
import { useAppState } from '@/lib/state/store';
import { CompanyCard, statusLabels } from './company-card';
import { filterCompanies } from './filter';
import { EmptyState, PageHeading, ResourceState } from '@/components/ui/primitives';
import styles from './catalog.module.css';
export function CatalogPage() {
  const params = useSearchParams(),
    router = useRouter(),
    pathname = usePathname(),
    dialog = useRef<HTMLDialogElement>(null);
  const state = useAppState();
  const directory = useCategories();
  const page = Math.max(1, Math.floor(Number(params.get('page'))) || 1),
    size = 3;
  const filtered = filterCompanies(
    companies
      .map((c) => (c.id === state.company.id ? state.company : c))
      .filter((c) => !state.hiddenCompanies.includes(c.id)),
    new URLSearchParams(params.toString()),
  );
  const requestParams = new URLSearchParams(params.toString());
  requestParams.set('pageSize', String(size));
  if (!requestParams.has('city')) requestParams.set('city', 'Астана');
  const data = useResource('/companies?' + requestParams.toString(), {
    items: filtered.slice((page - 1) * size, page * size),
    total: filtered.length,
    page,
    pageSize: size,
  });
  function change(values: Record<string, string>) {
    const p = new URLSearchParams(params.toString());
    p.delete('page');
    for (const [k, v] of Object.entries(values)) p.set(k, v);
    router.replace(pathname + '?' + p.toString(), { scroll: false });
  }
  const filters = (
    <div className={`card ${styles.filters}`}>
      <h3>Фильтры</h3>
      <label className="form-field mobile-only">
        <span>Город</span>
        <select
          className="field"
          aria-label="Город в фильтрах"
          value={params.get('city') ?? 'Астана'}
          onChange={(event) => change({ city: event.target.value })}
        >
          <option value="">Все города</option>
          <option>Астана</option>
          <option>Алматы</option>
        </select>
      </label>
      <p className="small">Сфера бизнеса</p>
      {!directory.data && <ResourceState error={directory.error} reload={directory.reload} />}
      {[{ id: '', name: 'Все сферы' }, ...(directory.data || [])].map((c) => (
        <button
          key={c.id}
          aria-pressed={(params.get('category_id') || '') === c.id}
          onClick={() => change({ category_id: c.id, category: '' })}
        >
          {c.name}
        </button>
      ))}
      <hr />
      <p className="small">Статус сайта</p>
      <button aria-pressed={!params.get('status')} onClick={() => change({ status: '' })}>
        Все статусы
      </button>
      {Object.entries(statusLabels).map(([v, label]) => (
        <button
          key={v}
          className={params.get('status') === v ? styles.statusSelected : ''}
          aria-pressed={params.get('status') === v}
          onClick={() => change({ status: v })}
        >
          {label}
        </button>
      ))}
      <hr />
      <button onClick={() => router.replace('/companies?city=&status=', { scroll: false })}>
        Сбросить фильтры
      </button>
    </div>
  );
  return (
    <section className="container page">
      <PageHeading
        title="Компании для вашего первого проекта"
        description="Найдите бизнес, которому можете предложить разработку сайта."
        mobileTitle="Компании"
        mobileDescription="Найдите бизнес для первого проекта."
      />
      <form
        className={styles.search}
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          change({ q: String(f.get('q') || ''), city: String(f.get('city') || '') });
        }}
      >
        <input
          className="field"
          key={params.get('q')}
          name="q"
          aria-label="Название компании или сфера"
          placeholder="Название компании или сфера"
          defaultValue={params.get('q') || ''}
        />
        <select
          className="field"
          name="city"
          aria-label="Город"
          key={params.get('city')}
          defaultValue={params.get('city') ?? 'Астана'}
        >
          <option value="">Все города</option>
          <option>Астана</option>
          <option>Алматы</option>
        </select>
        <button className="button" type="submit">
          Найти
        </button>
        <button
          type="button"
          className={`button secondary mobile-only ${styles.filterToggle}`}
          onClick={() => dialog.current?.showModal()}
        >
          Фильтры · {params.get('city') === '' ? 'Все города' : params.get('city') || 'Астана'}
        </button>
      </form>
      <div className={styles.layout}>
        <aside className={styles.sidebar}>{filters}</aside>
        <div className={styles.results}>
          {data.data ? (
            <>
              <div className="spread">
                <p>Найдено {data.data.total} компаний</p>
                <select
                  className={styles.sort}
                  aria-label="Сортировка"
                  value={params.get('sort') || 'updated'}
                  onChange={(e) => change({ sort: e.target.value })}
                >
                  <option value="updated">Сначала недавно проверенные</option>
                  <option value="name">По названию</option>
                </select>
              </div>
              {data.data.items.length ? (
                data.data.items.map((c) => <CompanyCard key={c.id} company={c} />)
              ) : (
                <EmptyState
                  title="Компании не найдены"
                  description="Попробуйте другую сферу или сбросьте фильтры."
                  href="/companies?city=&status="
                  label="Сбросить фильтры"
                />
              )}
              <nav className={styles.pagination} aria-label="Страницы каталога">
                {Array.from({ length: Math.ceil(data.data.total / size) }, (_, i) => (
                  <button
                    key={i}
                    className={`button ${page === i + 1 ? '' : 'secondary'}`}
                    aria-current={page === i + 1 ? 'page' : undefined}
                    onClick={() => change({ page: String(i + 1) })}
                  >
                    {i + 1}
                  </button>
                ))}
                {page * size < data.data.total && (
                  <button
                    className="button secondary"
                    onClick={() => change({ page: String(page + 1) })}
                  >
                    Далее →
                  </button>
                )}
              </nav>
              <p className={`caption muted ${styles.pageNote}`}>
                Статус «сайт не найден» отражает результат проверки, а не подтверждённую потребность
                компании.
              </p>
            </>
          ) : (
            <ResourceState error={data.error} reload={data.reload} />
          )}
        </div>
      </div>
      <dialog ref={dialog} className={styles.filtersDialog}>
        <div className="stack-lg">
          <div className="spread">
            <h2>Фильтры</h2>
            <button
              className="text-button"
              onClick={() => dialog.current?.close()}
              aria-label="Закрыть фильтры"
            >
              Закрыть
            </button>
          </div>
          <label className="form-field">
            Город
            <select
              className="field"
              value={params.get('city') ?? 'Астана'}
              onChange={(e) => change({ city: e.target.value })}
            >
              <option value="">Все города</option>
              <option>Астана</option>
              <option>Алматы</option>
            </select>
          </label>
          {filters}
          <button className="button" onClick={() => dialog.current?.close()}>
            Показать компании
          </button>
        </div>
      </dialog>
    </section>
  );
}
