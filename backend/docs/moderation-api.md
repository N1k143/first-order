# API кабинета модератора

Базовый путь на рабочем сайте: `/api/v1`. Все поля — `snake_case`. Фронтенд может обращаться к этому пути через существующий Next.js proxy. Здесь описан серверный контракт; интерфейс кабинета реализуется отдельно.

## Авторизация и общие ответы

Вход — существующий `POST /api/auth/login`, тело `{ "email": "...", "password": "..." }`. Браузер использует HttpOnly cookies и `credentials: 'include'`; изменяющие запросы должны иметь разрешённый `Origin`. Текущий пользователь — `GET /api/v1/me`; роль `moderator` назначается оператором через `backend/cmd/admin`, не формой регистрации и не клиентским JSON. Внутренний importer-token не даёт права модератора.

Все маршруты этого кабинета требуют роль `moderator`, кроме упомянутых публичных маршрутов каталога. Гость получает `401`, обычный пользователь, представитель компании и сервис импорта — `403`.

Списки возвращают `{ "items": [], "total": 0, "page": 1, "limit": 20 }`. Параметры: `page >= 1`, `1 <= limit <= 100`. Пустая выборка — `200` с пустым `items`.

Ошибки: `{ "error": { "code": "...", "message": "...", "details": {} }, "request_id": "..." }`. Не подставлять демоданные при ошибке.

## Маршруты для новых экранов

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/moderation/summary` | Счётчики кабинета |
| GET | `/imports/candidates` | Очередь собранных карточек |
| GET | `/imports/candidates/{id}` | Исходные данные, черновик и эффективная карточка |
| PATCH | `/imports/candidates/{id}` | Ручные правки черновика |
| GET | `/imports/candidates/{id}/duplicates` | Возможные совпадения с компаниями каталога |
| POST | `/imports/candidates/{id}/review` | Одобрение, отклонение либо привязка к существующей компании |
| GET | `/moderation/candidate-actions` | История правок и решений по кандидатам |
| GET | `/moderation/correction-requests/{id}` | Обращение пользователя с текущей карточкой компании |

### Сводка

`GET /moderation/summary`:

```json
{
  "pending_candidates": 100,
  "approved_candidates": 0,
  "rejected_candidates": 0,
  "pending_candidates_with_website": 8,
  "hidden_companies": 0,
  "published_companies": 0,
  "pending_corrections": 0,
  "hidden_orders": 0
}
```

Это пример формы ответа, не гарантированные значения. `hidden_companies` включает любые скрытые компании, не только ожидающие первой публикации.

### Список и просмотр кандидата

Фильтры `GET /imports/candidates`:

- `status`: `pending` (по умолчанию), `approved`, `rejected`, `all`;
- `q`: поиск по эффективному названию, до 240 символов;
- `city`: точное эффективное название города;
- `category`: эффективное название категории; при выборе `category_id` используется имя из справочника;
- `category_id`: UUID выбранной модератором категории;
- `website_status`: `found`, `not_found`, `needs_review`;
- `source`: сейчас `osm_astana`;
- `company_id`: UUID связанной компании;
- `page`, `limit`.

Сортировка фиксирована: `collected_at DESC, id`. Фильтры применяются к ручным правкам, если они есть, а не только к исходным данным.

Один элемент списка и ответ `GET /imports/candidates/{id}` содержат:

```json
{
  "id": "candidate-uuid",
  "source": "osm_astana",
  "external_id": "node/123",
  "review_status": "pending",
  "revision": 1,
  "draft": {},
  "company_id": null,
  "collected_at": "2026-11-26T10:00:00Z",
  "reviewed_at": null,
  "reviewed_by": null,
  "reviewer_name": null,
  "review_note": null,
  "payload": {
    "name": "Исходное название",
    "city": "Астана",
    "category": "shop:bakery",
    "source_url": "https://www.openstreetmap.org/node/123",
    "attribution": "© OpenStreetMap contributors · ODbL 1.0 · https://www.openstreetmap.org/copyright",
    "website_check": { "status": "needs_review", "reason": "ownership_not_confirmed" }
  },
  "proposed_company": {
    "name": "Исходное название",
    "city": "Астана",
    "category_id": null,
    "category_name": "shop:bakery",
    "address": null,
    "phone": null,
    "email": null,
    "website_url": null,
    "website_status": "needs_review"
  }
}
```

В примере `payload` сокращён: фактический объект также содержит контакты, tags, исходный website URL, `website_candidates`, `website_check.attempts` с доказательствами и причины неопределённости. Эти поля не предназначены для вставки как HTML. URL разрешать только HTTP(S).

Форму заполнять из `proposed_company`. `payload` показывать отдельно как источник и результаты автоматической проверки. `draft` содержит только ручные переопределения; исходный `payload` модератор не меняет. Сборщик может обновлять `payload`, но не стирает `draft` и не пересматривает уже принятые решения.

### Редактирование

`PATCH /imports/candidates/{id}`:

```json
{
  "expected_revision": 1,
  "reason": "Проверены контакты и категория",
  "changes": {
    "name": "Уточнённое название",
    "category_name": "Пекарни",
    "phone": "+7 700 000 00 00",
    "website_status": "not_found",
    "website_url": null
  }
}
```

Доступные поля `changes`: `name`, `city`, `category_id`, `category_name`, `address`, `phone`, `email`, `website_url`, `website_status`. Пропущенные поля сохраняются. `null` очищает необязательные поля; `name`, `city`, `website_status` не могут быть `null`. Идентификаторы, исходные доказательства, права, решение и видимость через PATCH не меняются.

Можно выбрать существующий `category_id` из `GET /categories` либо передать новое `category_name`. Ненулевые значения обоих полей одновременно запрещены. Выбор одного очищает другое; для отсутствия категории передать оба `null`. Новая категория создаётся при одобрении, а не при сохранении черновика.

Для `found` нужен непустой HTTP(S) `website_url`; для `not_found` его нужно очистить. `needs_review` означает отсутствие достоверного решения, не отсутствие сайта. При одобрении ссылка переносится в официальное поле компании только для `found`; непроверенные ссылки остаются в доказательствах/черновике.

`expected_revision` обязателен для PATCH. Успех — `200` с полной карточкой и новой `revision`. Версия увеличивается и при обновлении сборщиком. При `409 STALE_CANDIDATE` ответ содержит `error.details.current_revision`; перечитать карточку и предложить человеку сверить изменения, не отправлять форму повторно автоматически. После решения правки запрещены (`409 ALREADY_REVIEWED`).

### Дубли и решение

`GET /imports/candidates/{id}/duplicates` возвращает стандартный список компаний, включая скрытые. Совпадения ищутся по исходному URL, имени/городу/адресу либо телефону. Это кандидаты на совпадение, не автоматическое объединение.

`POST /imports/candidates/{id}/review`:

```json
{ "decision": "approve", "expected_revision": 2, "reason": "Карточка проверена" }
```

- `approve`: создаёт одну **скрытую** компанию из эффективных полей, фиксирует решение и обе истории. Дубль даёт `409 POSSIBLE_DUPLICATE`, в `error.details.duplicates_path` — путь списка совпадений. Никакая компания при конфликте не создаётся.
- `reject`: отклоняет кандидата без создания компании; причина и автор сохраняются в истории.
- `link`: связывает источник с существующей компанией, не перезаписывает её поля, владельцев, заказы, статусы или видимость:

```json
{ "decision": "link", "expected_revision": 2, "company_id": "existing-company-uuid", "reason": "Это тот же филиал" }
```

Ответ: `{ "status": "approved", "company_id": "uuid", "revision": 3 }` либо `{ "status": "rejected", "company_id": null, "revision": 3 }`.

Для нового интерфейса всегда передавать `expected_revision`. Ради совместимости старые запросы `approve`/`reject` без версии пока принимаются, но не защищают от просмотра устаревшей формы. Для `link` версия обязательна. Повторное решение даёт 409; одновременные решения/создание дублей защищены транзакциями и блокировками.

### История кандидатов

`GET /moderation/candidate-actions`: фильтры `candidate_id`, `moderator_id`, `action` (`edit`, `approve`, `reject`, `link`), `date_from`, `date_to`, `page`, `limit`. Даты: `YYYY-MM-DD` либо ISO 8601; конечный день включён, дата без времени трактуется как UTC. Сортировка `created_at DESC,id`.

Элемент: `id`, `candidate_id`, `moderator_id`, `moderator_name`, `action`, `reason`, `before_state`, `after_state`, `created_at`. Снимки содержат источник, черновик, решение, связанную компанию и ревизию. Даже отклонение без компании записывается. Runtime-роль не имеет UPDATE/DELETE на таблицу истории. Старые решения, принятые до миграции 005, не получают выдуманных событий задним числом.

После миграции 006 история также содержит `actor_type`: `moderator` для пользовательских действий, `operator` для подготовки каталога серверной командой. В операторских событиях `moderator_id=null`, имя пользователя не подставлять. Это относится и к `/moderation/actions`.

## Существующие операции кабинета

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/companies?visibility=all` | Все компании; фильтры `q`, `city`, `category_id`, `website_status`, `visibility` |
| GET | `/companies/{id}` | Карточка, включая скрытую для модератора |
| PATCH | `/companies/{id}` | Правки существующей компании |
| PATCH | `/companies/{id}/visibility` | `{ "visibility": "published" или "hidden", "reason": "..." }` |
| GET | `/moderation/correction-requests` | Обращения; фильтр `status=pending/approved/rejected` |
| GET | `/moderation/correction-requests/{id}` | Обращение, `company_name`, `requested_by_name`, `current_company` |
| PATCH | `/moderation/correction-requests/{id}` | Решение и атомарное исправление карточки |
| GET | `/moderation/actions` | История компаний/заказов; фильтры объекта, автора, действия и дат |
| PATCH | `/orders/{id}/visibility` | `{ "visibility": false или true, "reason": "..." }` |
| GET | `/imports` и `/imports/{id}` | Журнал существующего CSV-импорта |

Решение обращения:

```json
{
  "status": "approved",
  "resolution_note": "Контакт проверен",
  "changes": { "phone": "+7 700 000 00 00" }
}
```

Для отклонения `status=rejected`, `changes` не передаётся. Повторное решение даёт `409 CORRECTION_ALREADY_RESOLVED`. Списки обращений дополнены `company_name`, `requested_by_name`.

Публикация OSM-карточек требует отображения их источника и атрибуции на фронте; происхождение доступно в связанной записи `/imports/candidates?status=approved&company_id=...`. Создание компании после approve и привязка link не публикуют её автоматически.

Запуск сетевого сборщика, расписание и загрузка произвольного файла через API не входят в эту итерацию. `POST /imports` пока поддерживает только настроенный оператором `demo_csv`; не показывать на фронте несуществующий запуск OSM-импорта.

## Развёртывание и проверка

Требуется `005_moderation_workspace.sql` после 001–004. Миграция добавляет черновики, ревизии и защищённую таблицу событий; существующие 100 карточек и решения сохраняются. Предыдущий бэкенд продолжает работать с добавленными nullable/default-полями. Новую сборку запускать после миграции.

Проверки: все роли и importer-token; фильтры по эффективным данным; nullable-поля; неизменность источника; устаревшие версии после ручной правки и сбора; одновременное одобрение; гонка двух дублей; скрытая компания после approve; отдельная публикация; link без изменения компании; отклонение с аудитом; контекст обращения и атомарное исправление.
