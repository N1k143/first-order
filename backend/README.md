# «Первый заказ»: backend

Go API для текущего frontend: каталог и категории, профили и портфолио, участники компаний, избранное и контакты, исправления и модерация, заказы и отклики, CSV-импорт, регистрация и сессии Supabase Auth. Один процесс и одна PostgreSQL, домены разделены пакетами обработчиков и пятью схемами БД.

Реализованы 49 бизнес-маршрутов, 10 Auth-операций и 2 проверки состояния. Модель содержит 13 прикладных таблиц; `auth.users` принадлежит Supabase. Алиасы для frontend не считаются отдельными операциями.

## Быстрый запуск

Нужны Go 1.25+, PostgreSQL с настроенным Supabase Auth и Node.js для frontend. API не создаёт собственные пароли или таблицы Auth. Команды ниже выполняются из `backend/`.

1. Скопируйте `.env.example` в `.env`, заполните `DATABASE_URL`, `SUPABASE_URL`, `SUPABASE_ANON_KEY`. Для локального HTTP задайте `COOKIE_SECURE=false`; на HTTPS оставьте `true`. `.env` автоматически не читается:

   ```sh
   cp .env.example .env
   # После заполнения файла:
   set -a
   . ./.env
   set +a
   ```

2. Примените миграции под отдельной административной учётной записью. Перед изменением существующей базы сохраните резервную копию. `001` создаёт отсутствующие объекты, `002` закрывает прямой доступ browser/PostgREST к прикладным таблицам и добавляет ограничения выбора исполнителя. Это намеренное изменение доступа для перехода на Go API. `003` добавляет nullable `profiles.profiles.avatar_url`, если столбца ещё нет; примените её до запуска версии API с аватарами.

   ```sh
   DATABASE_URL="$MIGRATION_DATABASE_URL" go run ./cmd/migrate
   ```

   `MIGRATION_DATABASE_URL` задаётся оператором вне репозитория. Мигратор использует advisory lock и журнал `public.firstorder_schema_migrations`; повторный запуск пропускает применённые файлы. Требуется существующая `auth.users`. Для self-hosted Supabase миграции выполняются под `supabase_admin`: обычный `postgres` может не иметь права создавать роль с `BYPASSRLS`.

3. Миграция создаёт `firstorder_backend NOLOGIN BYPASSRLS`. В административной сессии `psql` включите вход и интерактивно назначьте пароль:

   ```sql
   ALTER ROLE firstorder_backend LOGIN;
   \password firstorder_backend
   ```

   В `DATABASE_URL` API используйте **саму роль `firstorder_backend`**. Обычное членство другой роли в ней не наследует `BYPASSRLS`. Runtime получает CRUD только прикладных таблиц и чтение `id,email` из `auth.users`. Административный DSN не передавайте API или frontend.

4. Запустите API:

   ```sh
   go run ./cmd/api
   curl http://127.0.0.1:18080/health
   curl http://127.0.0.1:18080/ready
   curl 'http://127.0.0.1:18080/api/v1/companies?page=1&limit=20'
   ```

   Старый entrypoint `go run ./services/catalog/cmd/api` запускает тот же полный API и теперь тоже требует `DATABASE_URL`.

5. В `frontend/.env.local` задайте:

   ```dotenv
   BACKEND_URL=http://127.0.0.1:18080
   NEXT_PUBLIC_DATA_MODE=api
   ```

   Перезапустите frontend; для production пересоберите его, поскольку `NEXT_PUBLIC_*` фиксируется при сборке. Next проксирует `/api/v1/*` с сохранением версии, `/api/*` — в совместимые маршруты API. Браузер использует HttpOnly cookies того же origin.

## Auth и начальные роли

В Supabase Auth настройте SMTP и разрешённый redirect `AUTH_REDIRECT_URL`, например `http://localhost:3000/auth/callback`. Frontend callback принимает стандартную ссылку с фрагментом access/refresh tokens либо `token_hash`/`type`, очищает адрес и обменивает данные на HttpOnly cookies. Реальная доставка писем зависит от SMTP; тесты используют контролируемый Auth-сервер. Контракт адаптера сверяется с [OpenAPI Supabase Auth](https://github.com/supabase/auth/blob/master/openapi.yaml).

Регистрация допускает `developer` или `company`. Первый вход создаёт профиль. Для нового представителя создаётся скрытая компания и membership `owner`; присоединение к уже существующей компании по одному названию запрещено. Проверенное владение существующей компанией и модератора выдаёт оператор после первого входа пользователя:

```sh
DATABASE_URL="$ADMIN_DATABASE_URL" go run ./cmd/admin moderator USER_UUID
DATABASE_URL="$ADMIN_DATABASE_URL" go run ./cmd/admin owner COMPANY_UUID USER_UUID
```

Права берутся из БД на каждом запросе. Клиент не может назначить себе модератора, владельца, автора записи или статус заказа. Последнего владельца компании удалить нельзя. Владелец управляет участниками; представитель управляет карточкой и заказами своей компании.

`JWT_SECRET` необязателен: без него Supabase проверяет JWT через `/auth/v1/user`, включая асимметричные ключи. Если задан legacy HS256 secret, API локально проверяет подпись, срок, issuer, audience и роль токена; завершение сессии не аннулирует уже выданный JWT до его `exp`. Refresh хранится только в HttpOnly cookie. `SUPABASE_ANON_KEY` нужен серверному Auth-адаптеру; service-role key не требуется.

## Контракт API

Рабочий каталог, источники, фильтры с количеством компаний и операторская подготовка карточек: [docs/catalog-api.md](docs/catalog-api.md).

Контракт кабинета модератора для фронтенда: [docs/moderation-api.md](docs/moderation-api.md). Реализованы сводка, очередь кандидатов с фильтрами, просмотр, черновые правки с ревизиями, совпадения, approve/reject/link, история кандидатов и контекст обращения. Для новых операций нужна миграция `005_moderation_workspace.sql`; исходные данные сборщика и ручной черновик хранятся отдельно.

Основной API — `/api/v1`, поля в `snake_case`. JSON-списки: `{items,total,page,limit}`; `page` начинается с 1, `limit` по умолчанию 20, максимум 100. Ошибка: `{error:{code,message,details},request_id}`. `PATCH` различает отсутствующее поле и явный `null` для nullable-полей; служебные поля отвергаются.

| Область | Операции | Где смотреть обработчики |
| --- | ---: | --- |
| Профиль, портфолио, участники компаний | 10 | [profiles.go](internal/api/profiles.go) |
| Каталог, категории, избранное, контакты, исправления, модерация | 23 | [catalog.go](internal/api/catalog.go), [catalog_metadata.go](internal/api/catalog_metadata.go) |
| Заказы, отклики, выбор исполнителя, видимость | 15 | [orders.go](internal/api/orders.go) |
| Импорт | 3 | [importer.go](internal/api/importer.go) |
| Кандидаты, сводка и журнал кабинета модератора | 7 | [discovery.go](internal/api/discovery.go), [moderation_candidates.go](internal/api/moderation_candidates.go), [moderation.go](internal/api/moderation.go) |
| Auth | 10 | [auth.go](internal/api/auth.go) |
| `/health`, `/ready` | 2 | [server.go](internal/api/server.go) |

Auth POST-маршруты: `/auth/register`, `/auth/login`, `/auth/logout`, `/auth/refresh`, `/auth/session`, `/auth/forgot-password`, `/auth/reset-password`, `/auth/change-email`, `/auth/resend-verification`, `/auth/verify`. Доступны также с префиксом `/api/v1`.

[compat.go](internal/api/compat.go) адаптирует существующий frontend: camelCase DTO, `pageSize`, названия категорий, локализованные статусы контактов, POST избранного, PATCH контакта, `/corrections`. Этот слой использует те же обработчики и проверки доступа. Для каталога frontend по умолчанию получает 3 компании на страницу.

Заказ создаётся как `draft`; действия `publish`, `select-application`, `complete`, `close` управляют переходами. Скрытие модератором меняет `visibility` отдельно от жизненного цикла. Выбор исполнителя в одной транзакции принимает один отклик, отклоняет остальные поданные и обновляет заказ. Операции с откликами блокируют заказ в одинаковом порядке; частичный уникальный индекс запрещает два accepted-отклика. Повторный отклик после отзыва не создаётся: действует уникальная пара заказ/разработчик.

## Импорт

Управляемый сбор Астаны: [контракт API](docs/collection-api.md) — запуск, прогресс, отмена, история, повтор и восстановление после перезапуска. Требуется миграция `007_collection_jobs.sql`. Новые компании попадают только в очередь модерации.

Сбор реальных компаний Астаны и проверка сайтов: [инструкция сборщика](internal/discovery/README.md). `cmd/collect-companies` сохраняет до 100 карточек разных категорий в закрытую очередь проверки; автоматической публикации нет. Требуется миграция `004_company_discovery.sql`. Дополнительные сайты ищутся по доменам корпоративной почты и связанным сущностям Wikidata. Используются бесплатные источники без ключей; неподтверждённые карточки помечаются `needs_review`, источник и результат каждой попытки сохраняются.

`POST /api/v1/imports` с `{"source":"demo_csv"}` доступен модератору или внутреннему `IMPORTER_TOKEN`; возвращает `202` и `run_id`. Модератор читает `/imports` и `/imports/{run_id}`. Сервисный токен не даёт пользовательскую сессию или права модерации.

Файл задаётся только оператором через `IMPORT_CSV_PATH`; пример — [companies.csv](testdata/companies.csv). Обязательные колонки `external_id,name`, остальные: `category_id,city,address,phone,email,website_url,website_status,source_url`. Максимум 20 МБ и 10 000 строк. `category_id` должен существовать. Ключ `(source,external_id)` обеспечивает повторный импорт без дублей; обновление сохраняет решение модератора о видимости.

Очередь хранится в PostgreSQL и обрабатывается фоновым worker. `SKIP LOCKED` позволяет несколько процессов; при остановке транзакция откатывается в `pending`. Каждый CSV атомарен: ошибка откатывает все строки и фиксирует `failed`. В этой версии промежуточный `running` и прогресс не видны до commit; UI наблюдает `pending → completed/failed`. Это ограничение относится к CSV-импорту; управляемый сбор внешних данных использует отдельную очередь. Загрузка произвольного файла через API не реализована.

## Эксплуатация

- По умолчанию API слушает localhost; reverse proxy завершает HTTPS. Для контейнера задан `0.0.0.0:18080`.
- `ALLOWED_ORIGINS` содержит точные origin. Unsafe-запрос с cookie требует разрешённый `Origin`. Ограничение JSON — 1 МБ; запросы имеют timeout.
- `RATE_LIMIT_PER_MINUTE` ограничивает изменяющие запросы по IP непосредственного клиента. Заголовки forwarded не считаются доверенными. За reverse proxy настройте его собственное ограничение и подходящий общий лимит API.
- `/health` проверяет процесс; `/ready` проверяет БД и при заданном `READY_TOKEN` требует `Authorization: Bearer …`.
- JSON-логи включают request ID, user ID для аутентифицированных бизнес-запросов, путь, статус, безопасный код ошибки и длительность. Пароли, JWT, тело запросов, DSN и сырые SQL-ошибки в HTTP-ответы не попадают.
- `make build` собирает `bin/api`; `Dockerfile` собирает API и мигратор. Образ не содержит конфигурации или паролей. Миграции выполняются отдельно, до запуска runtime.

## Проверки

```sh
go test ./...
go vet ./...
TEST_DATABASE_URL='postgres://…/firstorder_test?sslmode=disable' go test -race -count=1 ./internal/api
```

Интеграционные тесты требуют отдельную мигрированную PostgreSQL; имя БД должно содержать `test`. Для изолированного теста без Supabase **до миграций** создайте только тестовую заготовку `CREATE SCHEMA auth; CREATE TABLE auth.users(id uuid PRIMARY KEY,email text);`. В рабочей базе заготовка недопустима: там нужна настоящая Supabase Auth.

Тесты создают и удаляют собственные UUID-фикстуры, без TRUNCATE. Проверяются ACL, чужое портфолио, последний owner, конкурентный отзыв membership, nullable PATCH, полный цикл заказов, одновременный выбор/отзыв отклика, роль runtime, rollback исправлений, повторный CSV и rollback неуспешного импорта, JWT и cookies. Без `TEST_DATABASE_URL` интеграционные сценарии пропускаются.

Проверка реализации выполнена на отдельной PostgreSQL 17: миграции, интеграционные тесты с race detector, vet и сборка Go; для frontend — TypeScript, ESLint, 7 тестов транспорта и production build. Затем миграции применены к рабочей Supabase, API и frontend запущены. Финальная проверка подтвердила публичную регистрацию двух аккаунтов через браузер, получение писем через Gmail SMTP и подтверждение обоих адресов настоящими ссылками. После этого успешно прошёл сценарий входа, создания и публикации заказа, отклика и выбора исполнителя с отображением принятого отклика. Подтверждение email оставалось обязательным; административное подтверждение не использовалось. Все записи финального тестового прогона и тестовые пароли удалены, остаток записей проверен и равен нулю. Точный статус, состав очистки и схема развёртывания — в [deploy/README.md](../deploy/README.md).

### Фото профиля и настройки аккаунта

`PATCH /api/v1/me/profile` принимает `avatar_url`: `null` удаляет фото, отсутствие поля сохраняет прежнее. Разрешены http(s)-ссылки до 2048 байт и base64 data URL с PNG/JPEG/WebP до 256 КиБ после декодирования, максимум 2048 × 2048 пикселей. API проверяет фактический формат и декодирование; внешние изображения не скачивает. `GET /me` и `GET /profiles/{user_id}` возвращают фото, а совместимый `/api`-контракт использует `avatarUrl`.

`POST` или `PATCH /auth/change-email` принимает только `{email}` и использует текущую пользовательскую сессию для Supabase `PUT /user`. Ответ `202 {status:"confirmation_required",email}` означает ожидание подтверждения, а не замену текущего адреса. Если Auth уже подтверждает этот адрес, возвращается `200 {status:"updated",email}`. При безопасной смене адреса могут потребоваться ссылки из обоих ящиков: первый шаг `/auth/verify` возвращает `202 {status:"second_confirmation_required"}` без новой сессии. Callback задаётся серверным `AUTH_REDIRECT_URL`; клиент не может заменить его. `POST /auth/reset-password` с `{password,confirm}` работает и из настроек с обычной действующей сессией; требования повторной аутентификации определяет Supabase.
