'use client';

import Link from 'next/link';

export default function PageError({ retry }: { error: unknown; retry: () => void }) {
  return (
    <section className="container page">
      <div className="card stack" role="alert">
        <h1>Не удалось открыть раздел</h1>
        <p>Попробуйте загрузить его ещё раз или вернитесь на главную.</p>
        <div className="button-row">
          <button className="button" onClick={() => retry()}>
            Попробовать снова
          </button>
          <Link className="button secondary" href="/">
            На главную
          </Link>
        </div>
      </div>
    </section>
  );
}
