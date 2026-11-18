'use client';

import { useState } from 'react';
import { AccountShell } from '@/components/layout/account-shell';
import { EmptyState, Textarea, Notice } from '@/components/ui/primitives';
import { AccountAccess } from '@/features/account/account-access';
import { useAppState, updateDemo } from '@/lib/state/store';
import { companies } from '@/data/demo';

export function ModerationPage({ view = 'queue' }: { view?: 'queue' | 'import' | 'history' }) {
  const state = useAppState();
  const [message, setMessage] = useState('');
  const queue = state.corrections.filter((c) => c.status === 'pending');
  return (
    <AccountShell
      title={
        view === 'queue' ? 'Модерация' : view === 'import' ? 'Импорт данных' : 'Журнал модерации'
      }
      role="moderator"
    >
      <AccountAccess>
        {state.profile.role !== 'moderator' ? (
          <div className="card stack">
            <h2>Демонстрация модерации</h2>
            <p className="muted">
              Этот раздел предназначен для проверки данных. В рабочем сервисе доступ будет
              назначаться сервером.
            </p>
            <button
              className="button align-start"
              onClick={() =>
                updateDemo((s) => ({ ...s, profile: { ...s.profile, role: 'moderator' } }))
              }
            >
              Открыть демо модератора
            </button>
          </div>
        ) : (
          <>
            {view === 'queue' &&
              (queue.length ? (
                queue.map((c) => (
                  <form
                    className="card stack"
                    key={c.id}
                    onSubmit={(e) => {
                      e.preventDefault();
                      const reason = String(new FormData(e.currentTarget).get('reason')).trim();
                      const approve =
                        (e.nativeEvent as SubmitEvent).submitter?.getAttribute('value') ===
                        'approve';
                      if (reason.length < 5) {
                        setMessage('Укажите результат проверки: минимум 5 символов.');
                        return;
                      }
                      updateDemo((s) => ({
                        ...s,
                        corrections: s.corrections.map((item) =>
                          item.id === c.id
                            ? { ...item, status: approve ? 'approved' : 'rejected', reason }
                            : item,
                        ),
                        history: [
                          `${new Date().toLocaleString('ru-RU')} · ${companies.find((item) => item.id === c.companyId)?.name || c.companyId} · ${approve ? 'Обращение подтверждено' : 'Обращение отклонено'} · ${reason}`,
                          ...s.history,
                        ],
                      }));
                      setMessage(
                        'Результат проверки сохранён. Данные карточки автоматически не меняются.',
                      );
                    }}
                  >
                    <span className="badge amber align-start">На проверке</span>
                    <h3>
                      {companies.find((item) => item.id === c.companyId)?.name || c.companyId}
                    </h3>
                    <p className="preserve-lines">{c.message}</p>
                    <Textarea
                      label="Результат проверки"
                      name="reason"
                      required
                      minLength={5}
                      maxLength={2000}
                      placeholder="Укажите основание решения и источник проверки."
                    />
                    <div className="button-row">
                      <button className="button" value="approve">
                        Подтвердить обращение
                      </button>
                      <button className="button secondary" value="reject">
                        Отклонить
                      </button>
                    </div>
                  </form>
                ))
              ) : (
                <EmptyState
                  title="Всё проверено"
                  description="Новые сообщения об ошибках появятся здесь."
                />
              ))}
            {view === 'history' &&
              (state.history.length ? (
                state.history.map((item, i) => (
                  <article className="card" key={i}>
                    <p>{item}</p>
                  </article>
                ))
              ) : (
                <EmptyState
                  title="Журнал пуст"
                  description="Здесь сохраняются решения модератора."
                />
              ))}
            {view === 'import' && (
              <article className="card stack">
                <h3>Загрузка каталога</h3>
                <p className="muted">
                  Импорт будет доступен после подключения серверного обработчика. Сейчас нельзя
                  загрузить файл или запустить импорт.
                </p>
                <p className="small muted">
                  Будущий импорт должен проверять дубликаты, источники и корректность данных до
                  публикации карточек.
                </p>
              </article>
            )}
            {message && <Notice>{message}</Notice>}
          </>
        )}
      </AccountAccess>
    </AccountShell>
  );
}
