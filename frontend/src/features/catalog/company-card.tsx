'use client';
import Link from 'next/link';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import type { Company } from '@/types/domain';
import { useAppState, updateDemo } from '@/lib/state/store';
import { mutate } from '@/lib/api/resources';
import styles from './company-card.module.css';
export const statusLabels = {
  not_found: 'Сайт не найден',
  found: 'Сайт найден',
  needs_review: 'Нужна проверка',
};
export function CompanyCard({ company }: { company: Company }) {
  const state = useAppState();
  const router = useRouter();
  const saved = state.favorites.includes(company.id);
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);
  async function save() {
    if (!state.signedIn) {
      router.push('/login');
      return;
    }
    setPending(true);
    setError('');
    try {
      await mutate(`/me/favorites/${company.id}`, saved ? 'DELETE' : 'POST', undefined, () =>
        updateDemo((s) => ({
          ...s,
          favorites: saved
            ? s.favorites.filter((id) => id !== company.id)
            : [...s.favorites, company.id],
        })),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось сохранить.');
    } finally {
      setPending(false);
    }
  }
  return (
    <article className={`card stack ${styles.card}`}>
      <span className="mobile-only">
        <span className="badge amber">{statusLabels[company.websiteStatus]}</span>
      </span>
      <div className={styles.top}>
        <span className={styles.avatar}>{company.initials}</span>
        <div className={styles.identity}>
          <h3>
            <Link href={`/companies/${company.id}`}>{company.name}</Link>
          </h3>
          <p className="small muted">
            {company.category} · {company.city}
          </p>
        </div>
        <button
          disabled={pending}
          className={`button quiet ${styles.save}`}
          onClick={save}
          aria-pressed={saved}
        >
          {saved ? 'Сохранено' : 'Сохранить'}
        </button>
      </div>
      <p className={`muted ${styles.description}`}>{company.description}</p>
      <div className={styles.bottom}>
        <span className={`badge ${company.websiteStatus === 'found' ? '' : 'amber'}`}>
          {statusLabels[company.websiteStatus]}
        </span>
        <span className="caption muted">
          Проверено{' '}
          {new Date(company.checkedAt + 'T12:00:00Z').toLocaleDateString('ru-RU', {
            day: 'numeric',
            month: 'long',
            timeZone: 'UTC',
          })}
        </span>
        <Link href={`/companies/${company.id}`}>О компании →</Link>
      </div>
      <Link className={`button secondary ${styles.mobileLink}`} href={`/companies/${company.id}`}>
        О компании
      </Link>
      {error && (
        <p className="error-text" role="alert">
          {error}
        </p>
      )}
    </article>
  );
}
