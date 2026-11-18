'use client';
import Link from 'next/link';
import { useState, type ReactNode } from 'react';
import { useAppState, useStoreReady, updateDemo, DEMO } from '@/lib/state/store';
import { EmptyState, Notice } from '@/components/ui/primitives';
import { checkBackendHealth } from '@/lib/api/client';
export function BackendStatus() {
  const [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false);
  return (
    <div className="stack">
      <p className="small muted">
        Сервер пока предоставляет только проверку доступности. Заказы, профили и вход работают как
        демонстрация в этом браузере.
      </p>
      <button
        className="button secondary align-start"
        disabled={busy}
        onClick={async () => {
          setBusy(true);
          try {
            const result = await checkBackendHealth();
            setMessage(
              result === 'OK'
                ? 'Сервер доступен. API заказов и аккаунтов ещё не реализован.'
                : 'Сервер ответил, но формат проверки не соответствует ожидаемому.',
            );
          } catch {
            setMessage(
              'Не удалось подключиться к серверу. Проверьте BACKEND_URL и запуск бэкенда.',
            );
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? 'Проверяем…' : 'Проверить связь с сервером'}
      </button>
      {message && <Notice>{message}</Notice>}
    </div>
  );
}
export function AccountAccess({
  children,
  company = false,
}: {
  children: ReactNode;
  company?: boolean;
}) {
  const state = useAppState();
  const ready = useStoreReady();
  if (!ready)
    return (
      <div className="card" role="status">
        Загрузка профиля…
      </div>
    );
  if (!DEMO)
    return (
      <div className="card stack">
        <h2>Раздел пока недоступен</h2>
        <BackendStatus />
      </div>
    );
  if (!state.signedIn)
    return (
      <EmptyState
        title="Войдите, чтобы продолжить"
        description="После входа здесь появятся ваши данные."
        href="/login"
        label="Войти"
      />
    );
  if (company && state.profile.role !== 'company')
    return (
      <div className="card stack">
        <h2>Кабинет компании</h2>
        <p className="muted">
          Размещать заказы может представитель компании. В деморежиме можно переключить роль и
          проверить этот сценарий.
        </p>
        <div className="button-row">
          <button
            className="button"
            onClick={() =>
              updateDemo((s) => ({ ...s, profile: { ...s.profile, role: 'company' } }))
            }
          >
            Открыть демо компании
          </button>
          <Link className="button secondary" href="/register/company">
            Создать аккаунт компании
          </Link>
        </div>
      </div>
    );
  return <>{children}</>;
}
