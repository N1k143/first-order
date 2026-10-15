"use client";

import { useState } from "react";
import type { Company } from "@/types/company";
import { CompanyRow } from "./company-row";

type CatalogSectionProps = { companies: Company[] };

export function CatalogSection({ companies }: CatalogSectionProps) {
  const [query, setQuery] = useState("");
  const [city, setCity] = useState("");
  const results = companies.filter(
    (c) =>
      (!city || city === c.city) &&
      (c.name + c.category).toLowerCase().includes(query.trim().toLowerCase()),
  );
  return (
    <section className="catalog wrap section" id="catalog">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Каталог организаций</p>
          <h2>Начните с подходящей компании.</h2>
        </div>
        <p>
          Выберите знакомую сферу и подумайте,
          <br />
          какую задачу бизнеса может решить сайт.
        </p>
      </div>
      <div className="catalog-panel">
        <div className="catalog-toolbar">
          <label className="search">
            <span aria-hidden="true">⌕</span>
            <input
              aria-label="Название или сфера деятельности"
              placeholder="Название или сфера деятельности"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <select
            aria-label="Город"
            value={city}
            onChange={(e) => setCity(e.target.value)}
          >
            <option value="">Все города</option>
            <option>Алматы</option>
            <option>Астана</option>
          </select>
          <span className="catalog-count" aria-live="polite">
            Найдено: {results.length}
          </span>
        </div>
        <div className="table-heading">
          <span>Организация</span>
          <span>Город</span>
          <span>Веб-сайт</span>
          <span>Профиль</span>
        </div>
        {results.map((company) => (
          <CompanyRow key={company.name} company={company} />
        ))}
        {!results.length && (
          <p className="empty">
            Компаний по этому запросу нет. Попробуйте другую сферу или город.
          </p>
        )}
        <div className="catalog-note">
          Демонстрация каталога. Организации приведены для примера.
        </div>
      </div>
    </section>
  );
}
