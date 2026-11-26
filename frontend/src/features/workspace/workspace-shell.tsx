'use client';

import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import { AccountShell } from '@/components/layout/account-shell';
import { AccountAccess } from '@/features/account/account-access';
import { EmptyState, Notice, Select } from '@/components/ui/primitives';
import { selectCompany, useAppState, useSelectedCompanyId } from '@/lib/state/store';

export function CompanyWorkspaceShell({
  title,
  description,
  fixedCompanyId,
  children,
}: {
  title: string;
  description?: string;
  fixedCompanyId?: string;
  children: ReactNode | ((companyId: string) => ReactNode);
}) {
  const state = useAppState(),
    selectedId = useSelectedCompanyId(),
    router = useRouter();
  const companies = state.profile.companies || [];
  const fixedMember = Boolean(
    fixedCompanyId && companies.some((company) => company.companyId === fixedCompanyId),
  );
  const companyId = fixedMember ? fixedCompanyId! : selectedId;
  const [busy, setBusy] = useState(false),
    [error, setError] = useState('');
  const synchronized = useRef<string | null>(null);
  useEffect(() => {
    if (!fixedMember || synchronized.current === fixedCompanyId) return;
    synchronized.current = fixedCompanyId!;
    if (fixedCompanyId !== selectedId)
      void selectCompany(fixedCompanyId!).catch((error) =>
        setError(error instanceof Error ? error.message : 'Не удалось выбрать компанию заказа.'),
      );
  }, [fixedMember, fixedCompanyId, selectedId]);
  async function change(id: string) {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await selectCompany(id);
      if (fixedCompanyId && fixedCompanyId !== id) router.push('/workspace/orders');
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось сменить компанию.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <AccountShell title={title} description={description} role="company">
      <AccountAccess company>
        {companyId ? (
          <>
            <div className="card stack">
              <Select
                label="Рабочая компания"
                name="workspace_company"
                value={companyId}
                disabled={busy}
                onChange={(event) => void change(event.target.value)}
              >
                {companies.map((company) => (
                  <option key={company.companyId} value={company.companyId}>
                    {company.name || company.companyId}
                  </option>
                ))}
              </Select>
              <p className="small muted">
                Заказы, профиль и участники относятся к выбранной компании.
              </p>
              {error && <Notice error>{error}</Notice>}
            </div>
            {busy ? (
              <div role="status">Загружаем компанию…</div>
            ) : (
              <div className="stack-lg" key={companyId}>
                {typeof children === 'function' ? children(companyId) : children}
              </div>
            )}
          </>
        ) : (
          <EmptyState
            title="Нет доступных компаний"
            description="Попросите владельца компании добавить ваш зарегистрированный аккаунт в участники."
          />
        )}
      </AccountAccess>
    </AccountShell>
  );
}
