import Link from 'next/link';
import type {
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from 'react';
export function PageHeading({
  title,
  description,
  mobileTitle,
  mobileDescription,
}: {
  title: string;
  description?: ReactNode;
  mobileTitle?: string;
  mobileDescription?: string;
}) {
  return (
    <div className="page-heading">
      <h1>
        {mobileTitle ? (
          <>
            <span className="desktop-only">{title}</span>
            <span className="mobile-only">{mobileTitle}</span>
          </>
        ) : (
          title
        )}
      </h1>
      {description && (
        <p className="muted">
          {mobileDescription ? (
            <>
              <span className="desktop-only">{description}</span>
              <span className="mobile-only">{mobileDescription}</span>
            </>
          ) : (
            description
          )}
        </p>
      )}
    </div>
  );
}
export function Field({
  label,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  return (
    <label className="form-field">
      <span>{label}</span>
      <input {...props} className={`field ${props.className || ''}`} />
    </label>
  );
}
export function Select({
  label,
  children,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & { label: string }) {
  return (
    <label className="form-field">
      <span>{label}</span>
      <select {...props} className="field">
        {children}
      </select>
    </label>
  );
}
export function Textarea({
  label,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string }) {
  return (
    <label className="form-field">
      <span>{label}</span>
      <textarea {...props} className="field" />
    </label>
  );
}
export function EmptyState({
  title,
  description,
  href,
  label,
}: {
  title: string;
  description: string;
  href?: string;
  label?: string;
}) {
  return (
    <div className="card stack">
      <h3>{title}</h3>
      <p className="muted">{description}</p>
      {href && (
        <div>
          <Link className="button secondary" href={href}>
            {label || 'Продолжить'}
          </Link>
        </div>
      )}
    </div>
  );
}
export function ResourceState({ error, reload }: { error?: string; reload: () => void }) {
  return error ? (
    <div className="card stack" role="alert">
      <h3>Не удалось загрузить данные</h3>
      <p className="muted">{error}</p>
      <div>
        <button className="button secondary" onClick={reload}>
          Попробовать снова
        </button>
      </div>
    </div>
  ) : (
    <div className="card skeleton" role="status">
      Загрузка…
    </div>
  );
}
export function Notice({ children, error = false }: { children: ReactNode; error?: boolean }) {
  return (
    <p className={error ? 'notice error' : 'notice'} role={error ? 'alert' : 'status'}>
      {children}
    </p>
  );
}
export function InfoRow({
  label,
  value,
  status,
}: {
  label: ReactNode;
  value: ReactNode;
  status?: ReactNode;
}) {
  return (
    <div className="info-row">
      <span>{label}</span>
      <span className="small muted">{value}</span>
      {status}
    </div>
  );
}
export const money = (value: number) => new Intl.NumberFormat('ru-RU').format(value) + ' ₸';
export const dateLabel = (value: string) =>
  new Date(value + 'T12:00:00Z').toLocaleDateString('ru-RU', {
    day: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  });
