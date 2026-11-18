# FirstOrder frontend

Next.js 16 · React 19 · TypeScript · CSS Modules. Дизайн: Golos Text и токены из Figma FirstOrder.

## Запуск

```sh
npm ci
cp .env.example .env.local
npm run dev
```

Открыть http://localhost:3000. Шрифт поставляется локально, Google Fonts не запрашивается во время сборки или просмотра.

```sh
npm run typecheck
npm run lint
npm test
npm run build
npm start
```

## Структура

- `src/app`: маршруты App Router, общий layout, метаданные и 404.
- `src/components/layout`: общие шапка и подвал.
- `src/features`: интерфейсы отдельных предметных областей; сейчас `home` и `orders`.
- `src/lib/api`: транспорт запросов, ошибки, отмена, тайм-ауты.
- `src/types`: типы предметной области.
- `tests`: проверки границ API-клиента.
- `tools/figma-export`: локальный экспортёр макетов, не часть приложения.
- `docs/implementation-status.md`: реализованный объём и оставшиеся блокеры.

## Связь с backend

`BACKEND_URL` задаётся только серверу Next.js. `/api/:path*` проксируется на Go backend; существующий `/health` доступен через `/api/health`. `checkBackendHealth()` использует именно этот маршрут. Бэкенд не изменён. Других согласованных маршрутов пока нет.

Сейчас главная содержит один статический пример заказа из Figma. Полная функциональность ожидает остальные макеты и API. Подробности в документе состояния.
