from __future__ import annotations

import copy
import zipfile
import xml.etree.ElementTree as ET

import create_week8_presentation as ui


SOURCE = "/data/services/ingeneriya/Первый_заказ_Неделя_9_Топология_и_архитектура.pptx"
OUTPUT = "/data/services/ingeneriya/Первый_заказ_Полная_архитектура_и_API.pptx"
SLIDE_COUNT = 20

CT = "http://schemas.openxmlformats.org/package/2006/content-types"


def extend_deck(archive: dict[str, bytes]) -> None:
    template_slide = archive["ppt/slides/slide7.xml"]
    template_rels = archive["ppt/slides/_rels/slide7.xml.rels"]

    pres = ET.fromstring(archive["ppt/presentation.xml"])
    sld_list = pres.find(ui.q(ui.P, "sldIdLst"))
    max_id = max(int(x.attrib["id"]) for x in list(sld_list))

    rels = ET.fromstring(archive["ppt/_rels/presentation.xml.rels"])
    existing_rids = []
    for rel in list(rels):
        rid = rel.attrib.get("Id", "")
        if rid.startswith("rId") and rid[3:].isdigit():
            existing_rids.append(int(rid[3:]))
    next_rid = max(existing_rids) + 1

    content_types = ET.fromstring(archive["[Content_Types].xml"])
    existing_parts = {x.attrib.get("PartName") for x in list(content_types)}

    for number in range(8, SLIDE_COUNT + 1):
        archive[f"ppt/slides/slide{number}.xml"] = template_slide
        archive[f"ppt/slides/_rels/slide{number}.xml.rels"] = template_rels

        rid = f"rId{next_rid}"
        next_rid += 1
        ET.SubElement(
            rels,
            f"{{{ui.PR}}}Relationship",
            {
                "Id": rid,
                "Type": "http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide",
                "Target": f"slides/slide{number}.xml",
            },
        )
        max_id += 1
        ET.SubElement(sld_list, ui.q(ui.P, "sldId"), {"id": str(max_id), ui.q(ui.R, "id"): rid})

        part_name = f"/ppt/slides/slide{number}.xml"
        if part_name not in existing_parts:
            ET.SubElement(
                content_types,
                f"{{{CT}}}Override",
                {
                    "PartName": part_name,
                    "ContentType": "application/vnd.openxmlformats-officedocument.presentationml.slide+xml",
                },
            )

    archive["ppt/presentation.xml"] = ET.tostring(pres, encoding="utf-8", xml_declaration=True)
    archive["ppt/_rels/presentation.xml.rels"] = ET.tostring(rels, encoding="utf-8", xml_declaration=True)
    archive["[Content_Types].xml"] = ET.tostring(content_types, encoding="utf-8", xml_declaration=True)

    app = ET.fromstring(archive["docProps/app.xml"])
    ns = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
    slides = app.find(ui.q(ns, "Slides"))
    if slides is not None:
        slides.text = str(SLIDE_COUNT)
    archive["docProps/app.xml"] = ET.tostring(app, encoding="utf-8", xml_declaration=True)


def save(archive: dict[str, bytes], number: int, root: ET.Element) -> None:
    archive[f"ppt/slides/slide{number}.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)


def text_slide(
    archive: dict[str, bytes],
    number: int,
    section: str,
    heading: str,
    intro: str,
    lines: list[str],
    font_size: int = 1260,
    columns: tuple[list[str], list[str]] | None = None,
) -> None:
    root = ET.fromstring(archive[f"ppt/slides/slide{number}.xml"])
    st = ui.base_slide(root, number, section)
    sid = ui.title(st, 10, heading, intro)

    if columns:
        left, right = columns
        for x, content in ((650000, left), (6250000, right)):
            paragraphs = []
            for line in content:
                is_head = line.startswith("## ")
                paragraphs.append({
                    "text": line[3:] if is_head else line,
                    "size": 1450 if is_head else font_size,
                    "bold": is_head,
                    "color": ui.GREEN if is_head else ui.INK,
                    "bullet": not is_head,
                })
            ui.shape(st, sid, x, 1780000, 5350000, 4400000, paragraphs, fill=ui.WHITE, line=ui.LINE, radius=True, margin=190000, valign="t")
            sid += 1
    else:
        paragraphs = []
        for line in lines:
            is_head = line.startswith("## ")
            paragraphs.append({
                "text": line[3:] if is_head else line,
                "size": 1500 if is_head else font_size,
                "bold": is_head,
                "color": ui.GREEN if is_head else ui.INK,
                "bullet": not is_head,
            })
        ui.shape(st, sid, 650000, 1780000, 10850000, 4400000, paragraphs, fill=ui.WHITE, line=ui.LINE, radius=True, margin=210000, valign="t")
    save(archive, number, root)


def title_slide(archive: dict[str, bytes]) -> None:
    root = ET.fromstring(archive["ppt/slides/slide1.xml"])
    st = ui.base_slide(root, 1, "Архитектура", dark=True)
    ui.shape(st, 10, 760000, 620000, 3200000, 260000, [{"text": "FIRSTORDER · ПОЛНАЯ СПЕЦИФИКАЦИЯ", "size": 1120, "bold": True, "color": ui.ORANGE}])
    ui.shape(st, 11, 760000, 1450000, 10100000, 1700000, [
        {"text": "Архитектура и API", "size": 4100, "bold": True, "color": ui.WHITE},
        {"text": "компоненты · 5 модулей · 50 эндпоинтов · PostgreSQL", "size": 1800, "color": ui.MINT},
    ])
    ui.shape(st, 12, 760000, 4750000, 9300000, 850000, [
        {"text": "Текстовая версия для защиты и дальнейшей реализации backend", "size": 1550, "bold": True, "color": ui.WHITE},
        {"text": "Актуальное состояние: health-check + готовая модель данных; основной API — целевой контракт", "size": 1220, "color": ui.MINT},
    ])
    ui.shape(st, 13, 10150000, 5350000, 1200000, 650000, [{"text": "20", "size": 3500, "bold": True, "color": ui.ORANGE, "align": "r"}])
    save(archive, 1, root)


def build(archive: dict[str, bytes]) -> None:
    title_slide(archive)

    text_slide(archive, 2, "Проект", "Назначение и границы", "Платформа связывает начинающих разработчиков с малым бизнесом.", [], columns=(
        [
            "## Основные сценарии",
            "Поиск компаний без сайта по городу и категории",
            "Избранное и личные отметки о контакте",
            "Публикация заказа представителем компании",
            "Отклик разработчика и выбор одного исполнителя",
            "Импорт, исправление и модерация карточек",
        ],
        [
            "## За пределами MVP",
            "Встроенный чат и уведомления",
            "Платежи и безопасная сделка",
            "Отзывы, рейтинг и рекомендации",
            "Международная география и несколько валют",
            "Сложная аналитика и ML-рекомендации",
        ],
    ))

    text_slide(archive, 3, "Доступ", "Роли пользователей", "Права определяются JWT, ролью профиля и членством в компании.", [], columns=(
        [
            "## Гость",
            "Просматривает опубликованные компании",
            "Просматривает открытые видимые заказы",
            "Не создаёт и не изменяет данные",
            "## Разработчик",
            "Управляет профилем и портфолио",
            "Ведёт избранное и отметки контакта",
            "Отправляет, изменяет и отзывает свои отклики",
        ],
        [
            "## Представитель компании",
            "Управляет данными своей компании",
            "Создаёт, публикует и закрывает заказы",
            "Просматривает отклики и выбирает исполнителя",
            "## Модератор",
            "Скрывает и публикует карточки",
            "Обрабатывает запросы исправлений",
            "Просматривает журнал модерации и импорт",
        ],
    ), font_size=1160)

    text_slide(archive, 4, "Топология", "Компоненты и поток запроса", "Пользователь не обращается к PostgreSQL напрямую.", [
        "Пользователь → HTTPS → Cloudflare → Next.js → REST/JSON + Bearer JWT → Go API",
        "Go API → проверка JWT Supabase → RBAC → service layer → repository → PostgreSQL",
        "Cloudflare: DNS, TLS и защита публичного домена",
        "Next.js: страницы, формы, состояния загрузки, ошибок и пустых данных",
        "Go API: валидация, права, бизнес-правила, транзакции и единый формат ошибок",
        "Supabase Auth: регистрация, вход, обновление сессии и выдача JWT",
        "PostgreSQL: 5 схем, 13 таблиц, FK, CHECK, индексы, транзакции и RLS",
        "Внешний справочник → importer → source_records → catalog.companies",
        "e1.iq-home.kz: backend; e2.iq-home.kz: Supabase Studio и PostgreSQL",
    ], font_size=1240)

    text_slide(archive, 5, "Backend", "Пять модулей Go", "MVP разворачивается одним процессом, но границы совпадают со схемами БД.", [], columns=(
        [
            "## profiles",
            "Профили, роли, навыки и GitHub",
            "Портфолио разработчиков",
            "Представители компаний",
            "## catalog",
            "Категории и карточки компаний",
            "Избранное и отметки контакта",
            "Исправления и журнал модерации",
            "## importer",
            "Запуски импорта и исходные записи",
        ],
        [
            "## orders",
            "Создание и публикация заказов",
            "Бюджет, срок, видимость и статусы",
            "Хранение выбранного отклика",
            "## applications",
            "Отклики, предложение цены и срока",
            "Изменение и отзыв отклика",
            "Атомарный выбор одного исполнителя",
            "## Развитие",
            "Любой модуль можно вынести в отдельный сервис",
        ],
    ), font_size=1120)

    text_slide(archive, 6, "Данные", "Пять схем PostgreSQL", "Каждый модуль владеет своей предметной областью.", [], columns=(
        [
            "## profiles",
            "profiles — пользовательский профиль и роль",
            "portfolio_items — отдельные работы портфолио",
            "company_members — пользователь ↔ компания",
            "## catalog",
            "categories — справочник сфер",
            "companies — карточки и контакты",
            "favorites — личное избранное",
            "company_contacts — статусы контакта",
            "correction_requests — запросы исправления",
            "moderation_actions — аудит решений",
        ],
        [
            "## importer",
            "import_runs — состояние запуска",
            "source_records — raw_data и внешний ID",
            "## orders",
            "orders — заказ, бюджет, срок и статус",
            "## applications",
            "applications — отклик разработчика",
            "## Ключевые связи",
            "auth.users → profiles.profiles",
            "companies → orders → applications",
            "profiles → applications и portfolio_items",
        ],
    ), font_size=1060)

    text_slide(archive, 7, "API", "Общие правила HTTP API", "Базовый путь: https://e1.iq-home.kz/api/v1", [], columns=(
        [
            "## Запросы",
            "Authorization: Bearer <access_token>",
            "JSON, UTF-8, UUID",
            "Время ISO 8601 UTC; дата YYYY-MM-DD",
            "Деньги JSON number; валюта MVP — KZT",
            "Пагинация page=1&limit=20; максимум 100",
            "GET не изменяет состояние",
            "PUT используется для идемпотентного upsert",
        ],
        [
            "## Ответы и ошибки",
            "200 OK; 201 Created; 202 Accepted; 204 No Content",
            "400 некорректный запрос; 401 нет JWT",
            "403 недостаточно прав; 404 объект скрыт/не найден",
            "409 конфликт состояния или дубликат",
            "422 нарушение бизнес-правил",
            "500 внутренняя ошибка; 503 зависимость недоступна",
            "Ошибка: code, message, details, request_id",
        ],
    ), font_size=1120)

    text_slide(archive, 8, "Авторизация", "JWT и проверка прав", "Supabase подтверждает личность; Go API принимает решение о доступе.", [
        "Frontend выполняет вход через Supabase Auth и получает access_token",
        "Каждый защищённый запрос содержит Authorization: Bearer <JWT>",
        "Middleware проверяет подпись, issuer, audience и срок действия JWT",
        "JWT sub сопоставляется с auth.users.id и profiles.profiles.user_id",
        "Service проверяет роль profile.role и company_members для конкретной компании",
        "Права проверяются повторно непосредственно перед UPDATE/DELETE",
        "user_id, created_by, requested_by и developer_id берутся из JWT, а не из JSON",
        "service_role и пароль PostgreSQL существуют только на backend",
        "CORS разрешает frontend-домен; на вход и mutations действует rate limit",
    ], font_size=1220)

    text_slide(archive, 9, "Эндпоинты 1", "Система, профиль и портфолио", "9 маршрутов: состояние приложения и данные текущего пользователя.", [
        "GET /health — процесс работает; не обращается к БД",
        "GET /ready — проверяет PostgreSQL и готовность принимать трафик",
        "GET /api/v1/me — профиль, роль и компании текущего пользователя",
        "PATCH /api/v1/me/profile — имя, bio, skills и github_url",
        "GET /api/v1/profiles/{user_id} — публичный профиль и портфолио",
        "GET /api/v1/me/portfolio — список своих работ",
        "POST /api/v1/me/portfolio — создать работу",
        "PATCH /api/v1/me/portfolio/{item_id} — изменить свою работу",
        "DELETE /api/v1/me/portfolio/{item_id} — удалить свою работу",
        "Запрещено изменять user_id, role, created_at и чужие portfolio_items",
    ], font_size=1190)

    text_slide(archive, 10, "Эндпоинты 2", "Представители, категории и компании", "12 маршрутов для управления каталогом и владельцами компаний.", [], columns=(
        [
            "## Представители",
            "GET /companies/{id}/members — список участников",
            "POST /companies/{id}/members — добавить представителя",
            "DELETE /companies/{id}/members/{user_id} — удалить",
            "Нельзя удалить последнего owner",
            "## Категории",
            "GET /categories — публичный справочник",
            "POST /categories — создать, модератор",
            "PATCH /categories/{id} — переименовать",
            "DELETE /categories/{id} — удалить; FK станет NULL",
        ],
        [
            "## Компании",
            "GET /companies — поиск, фильтры и пагинация",
            "GET /companies/{id} — детальная карточка",
            "POST /companies — ручное создание модератором/importer",
            "PATCH /companies/{id} — изменить карточку",
            "PATCH /companies/{id}/visibility — publish/hide",
            "Фильтры: q, category_id, city, website_status",
            "Публично возвращаются только published",
            "Изменение visibility создаёт moderation_action",
        ],
    ), font_size=1030)

    text_slide(archive, 11, "Эндпоинты 3", "Избранное, контакты и исправления", "11 маршрутов персональной работы с каталогом и модерации.", [], columns=(
        [
            "## Избранное",
            "GET /me/favorites — список компаний",
            "PUT /me/favorites/{company_id} — добавить/upsert",
            "DELETE /me/favorites/{company_id} — удалить",
            "## Контакты",
            "GET /me/company-contacts — все отметки",
            "GET /me/company-contacts/{company_id} — одна",
            "PUT /me/company-contacts/{company_id} — создать/изменить",
            "DELETE /me/company-contacts/{company_id} — удалить",
        ],
        [
            "## Исправления",
            "POST /companies/{id}/correction-requests — сообщить ошибку",
            "GET /me/correction-requests — свои запросы",
            "GET /moderation/correction-requests — очередь модератора",
            "PATCH /moderation/correction-requests/{id} — решение",
            "Статусы: pending, approved, rejected",
            "Повторное решение: 409 CORRECTION_ALREADY_RESOLVED",
            "Решение сохраняет resolved_by, note и resolved_at",
        ],
    ), font_size=1040)

    text_slide(archive, 12, "Эндпоинты 4", "Заказы", "7 маршрутов жизненного цикла заказа.", [
        "GET /api/v1/orders — публичные open + visibility=true; фильтры и пагинация",
        "POST /api/v1/orders — представитель создаёт draft своей компании",
        "GET /api/v1/orders/{order_id} — детали с учётом видимости и роли",
        "PATCH /api/v1/orders/{order_id} — изменить draft/open своей компании",
        "POST /api/v1/orders/{order_id}/publish — draft → open",
        "POST /api/v1/orders/{order_id}/close — draft/open/selected → closed",
        "POST /api/v1/orders/{order_id}/complete — contractor_selected → completed",
        "Поля: company_id, title, description, budget_min/max, deadline",
        "created_by берётся из JWT; membership проверяется перед изменением",
        "Статусы: draft, open, contractor_selected, completed, closed, hidden",
        "DB CHECK: budget ≥ 0 и budget_max ≥ budget_min",
    ], font_size=1130)

    text_slide(archive, 13, "Эндпоинты 5", "Отклики и выбор исполнителя", "7 маршрутов; выбор выполняется одной транзакцией.", [
        "POST /orders/{order_id}/applications — отправить отклик на open-заказ",
        "GET /orders/{order_id}/applications — отклики владельцу заказа/модератору",
        "GET /me/applications — свои отклики",
        "GET /applications/{application_id} — детали по правилам владения",
        "PATCH /applications/{application_id} — изменить свой submitted-отклик",
        "POST /applications/{application_id}/withdraw — submitted → withdrawn",
        "POST /orders/{order_id}/select-application — выбрать исполнителя",
        "Поля: message, proposed_price, proposed_deadline",
        "UNIQUE(order_id, developer_id) запрещает повторный отклик",
        "Выбор: lock order → проверить права/status → accepted одному → rejected остальным",
        "Заказ получает selected_application_id и contractor_selected",
    ], font_size=1090)

    text_slide(archive, 14, "Эндпоинты 6", "Импорт и журнал модерации", "4 маршрута для внутренних и административных процессов.", [
        "POST /api/v1/imports — модератор/internal запускает асинхронный импорт; ответ 202",
        "GET /api/v1/imports — история запусков с пагинацией",
        "GET /api/v1/imports/{run_id} — status, counters, started/finished и error_message",
        "GET /api/v1/moderation/actions — журнал по объекту, действию и дате",
        "Статусы импорта: pending, running, completed, failed",
        "Алгоритм: create run → load → normalize → upsert source_record → upsert company",
        "UNIQUE(source, external_id) не допускает дубль исходной записи",
        "Ошибка внешнего источника фиксируется в import_runs и не удаляет старый каталог",
        "Журнал заполняется backend автоматически; публичного POST для него нет",
        "Действия: publish, hide, correct, approve, reject",
    ], font_size=1190)

    text_slide(archive, 15, "Бизнес-правила", "Статусы и допустимые переходы", "Недопустимый переход возвращает 409 или 422 и не меняет данные.", [], columns=(
        [
            "## Заказ",
            "draft → open: публикация",
            "open → contractor_selected: выбор отклика",
            "contractor_selected → completed: завершение",
            "draft/open/selected → closed: закрытие",
            "draft/open → hidden: модерация",
            "Новые отклики принимаются только в open",
            "## Отклик",
            "submitted → accepted",
            "submitted → rejected",
            "submitted → withdrawn",
        ],
        [
            "## Компания и контакт",
            "website_status: not_found/found/needs_review",
            "visibility: published/hidden",
            "contact: planned/contacted/replied/declined",
            "## Исправление",
            "pending → approved/rejected",
            "## Импорт",
            "pending → running → completed/failed",
            "## Членство",
            "owner/representative; последний owner сохраняется",
        ],
    ), font_size=1070)

    text_slide(archive, 16, "Целостность", "Ограничения базы и транзакции", "API проверяет правила заранее, PostgreSQL остаётся последней линией защиты.", [
        "PK UUID идентифицирует компании, заказы, отклики, импорт и портфолио",
        "FK запрещают сиротские профили, заказы, отклики и членство",
        "UNIQUE(order_id, developer_id) — один отклик разработчика на заказ",
        "Составной FK гарантирует: selected_application относится к этому же order",
        "CHECK ограничивает статусы, цены, бюджеты, видимость и действия модератора",
        "UNIQUE(source, external_id) обеспечивает идемпотентность импорта",
        "Транзакции: выбор исполнителя; одобрение исправления; импорт компании",
        "Транзакции: удаление member с проверкой owner; модерация + audit log",
        "Параллельный выбор блокирует заказ FOR UPDATE и сохраняет одного исполнителя",
        "updated_at обновляется триггерами для изменяемых сущностей",
    ], font_size=1160)

    text_slide(archive, 17, "Безопасность", "Защита API и данных", "Доверие строится от проверенного JWT, но каждое действие проверяется в контексте объекта.", [
        "Только HTTPS; TLS завершается на Cloudflare/reverse proxy",
        "JWT проверяется на каждом защищённом маршруте: signature, issuer, audience, exp",
        "RBAC + ownership: роль сама по себе не даёт доступ к чужой компании",
        "Параметризованный SQL; пользовательские строки не конкатенируются в запрос",
        "Секреты и service_role хранятся только в server environment",
        "RLS — дополнительная защита; основная бизнес-проверка остаётся в Go service",
        "CORS только для frontend; rate limit для auth и изменяющих маршрутов",
        "Клиент не получает SQL errors, stack trace и внутренние адреса",
        "Логи не содержат JWT, пароли, DB secrets и чувствительный текст сообщений",
        "Модерационные действия фиксируются автоматически и не создаются клиентом",
    ], font_size=1160)

    text_slide(archive, 18, "Код", "Структура backend и ответственность слоёв", "Модули одинаково организованы и тестируются независимо.", [], columns=(
        [
            "## Каталоги",
            "cmd/api/main.go — запуск и маршруты",
            "internal/platform/config — окружение",
            "internal/platform/database — pgx pool",
            "internal/platform/auth — JWT middleware",
            "internal/platform/httpx — JSON и ошибки",
            "internal/platform/logging — request_id и логи",
            "internal/{module}/handler.go — HTTP",
            "internal/{module}/service.go — правила",
            "internal/{module}/repository.go — SQL",
        ],
        [
            "## Поток вызова",
            "router → middleware → handler",
            "handler: parse, validate DTO, status code",
            "service: RBAC, ownership, state transitions",
            "repository: параметризованный SQL",
            "database: transaction, commit/rollback",
            "## Запреты",
            "Handler не содержит SQL",
            "Repository не решает права пользователя",
            "Frontend не обращается к таблицам напрямую",
            "Модуль не изменяет чужие таблицы без контракта",
        ],
    ), font_size=1040)

    text_slide(archive, 19, "Эксплуатация", "Логи, health-check и нефункциональные требования", "Минимальная наблюдаемость должна появиться вместе с первым реальным API.", [], columns=(
        [
            "## Наблюдаемость",
            "request_id для каждого запроса",
            "JSON log: time, method, path, status, duration",
            "user_id и безопасный error_code",
            "GET /health каждые 30 секунд",
            "GET /ready проверяет PostgreSQL",
            "Алерт при росте 5xx и failed imports",
            "Backup перед миграцией и тест восстановления",
        ],
        [
            "## Критерии",
            "95% обычных GET ≤ 500 мс на стенде",
            "Health доступен ≥ 99% демонстрационного периода",
            "401 без JWT; 403 при недостаточной роли",
            "Не больше 100 объектов на страницу",
            "PUT избранного и контакта идемпотентен",
            "Ошибки имеют единый JSON-формат",
            "Миграции разворачивают БД с нуля",
        ],
    ), font_size=1080)

    text_slide(archive, 20, "План", "Текущее состояние и последовательность реализации", "Архитектура описана полностью, но реализация API только начинается.", [], columns=(
        [
            "## Уже есть",
            "GET /health на e1.iq-home.kz",
            "Next.js landing и локальный каталог",
            "Supabase/PostgreSQL на e2.iq-home.kz",
            "5 схем и 13 прикладных таблиц",
            "FK, CHECK, индексы и выбор исполнителя",
            "Часть RLS-политик",
            "## Ограничение",
            "Frontend пока использует demo companies",
        ],
        [
            "## Следующие шаги",
            "1. Config, pgx, JWT middleware, errors, logging",
            "2. Profiles и Catalog read API",
            "3. Подключение frontend к каталогу",
            "4. Orders и Applications + transaction",
            "5. Модерация и importer",
            "6. Полные RLS/DB permissions",
            "7. Интеграционные и E2E тесты",
            "Готовность: оба сценария проходят UI → API → DB",
        ],
    ), font_size=1060)


def main() -> None:
    with zipfile.ZipFile(SOURCE, "r") as zin:
        archive = {name: zin.read(name) for name in zin.namelist()}
    extend_deck(archive)
    build(archive)
    with zipfile.ZipFile(OUTPUT, "w", zipfile.ZIP_DEFLATED) as zout:
        for name, data in archive.items():
            zout.writestr(name, data)
    print(OUTPUT)


if __name__ == "__main__":
    main()
