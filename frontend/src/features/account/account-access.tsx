'use client';
import Link from 'next/link';
import { useState, type ReactNode } from 'react';
import {
  useAppState,
  useStoreReady,
  useSessionError,
  refreshAppState,
  updateDemo,
  DEMO,
} from '@/lib/state/store';
import { EmptyState, Notice, ResourceState } from '@/components/ui/primitives';
import { checkBackendHealth } from '@/lib/api/client';
export function BackendStatus() {
  const [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false);
  return (
    <div className="stack">
      <p className="small muted">
        Проверка доступности сервера приложения. Вход, каталог, заказы и отклики подключены к API.
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
                ? 'Сервер доступен.'
                : 'Сервер ответил, но формат проверки не соответствует ожидаемому.',
            );
          } catch {
            setMessage('Не удалось подключиться к серверу. Попробуйте ещё раз позже.');
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
  demoOnly = false,
}: {
  children: ReactNode;
  company?: boolean;
  demoOnly?: boolean;
}) {
  const state = useAppState();
  const ready = useStoreReady();
  const sessionError = useSessionError();
  if (!ready)
    return (
      <div className="card" role="status">
        Загрузка профиля…
      </div>
    );
  if (sessionError)
    return (
      <ResourceState
        error={sessionError}
        reload={() => {
          void refreshAppState();
        }}
      />
    );
  if (!DEMO && demoOnly)
    return (
      <EmptyState
        title="Раздел недоступен"
        description="Этот инструмент доступен только в демонстрационном режиме."
        href="/dashboard"
        label="В кабинет"
      />
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
  if (company && !(DEMO ? state.profile.role === 'company' : state.profile.companies?.length))
    return (
      <div className="card stack">
        <h2>Кабинет компании</h2>
        <p className="muted">
          {DEMO
            ? 'В деморежиме можно переключить роль и проверить работу компании.'
            : 'Для управления компанией нужны права участника. Попросите владельца добавить вас в компанию или зарегистрируйте новую.'}
        </p>
        <div className="button-row">
          {DEMO && (
            <button
              className="button"
              onClick={() =>
                updateDemo((s) => ({ ...s, profile: { ...s.profile, role: 'company' } }))
              }
            >
              Открыть демо компании
            </button>
          )}
          <Link className="button secondary" href="/register/company">
            Создать аккаунт компании
          </Link>
        </div>
      </div>
    );
  return <>{children}</>;
}
