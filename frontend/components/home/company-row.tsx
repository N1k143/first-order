import type { Company } from "@/types/company";

type CompanyRowProps = { company: Company };

export function CompanyRow({ company: c }: CompanyRowProps) {
  return (
    <details className="organization">
      <summary>
        <span className="organization-name">
          <span className="monogram">{c.initials}</span>
          <span>
            <strong>{c.name}</strong>
            <small>{c.category}</small>
          </span>
        </span>
        <span className="city">{c.city}</span>
        <span className="status">Не указан</span>
        <span className="profile-link">Подробнее ↗</span>
      </summary>
      <div className="organization-details">
        <p>Возможный проект: {c.idea}</p>
        <small>
          {c.city} · Демонстрационный профиль. Контактные данные не добавлены.
        </small>
      </div>
    </details>
  );
}
