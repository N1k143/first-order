from __future__ import annotations

import zipfile
import xml.etree.ElementTree as ET

import create_week8_presentation as ui


SOURCE = "/data/services/ingeneriya/Первый_заказ_Неделя_8_Модель_данных.pptx"
OUTPUT = "/data/services/ingeneriya/Первый_заказ_Неделя_9_Топология_и_архитектура.pptx"


def save_slide(archive: dict[str, bytes], number: int, root: ET.Element) -> None:
    archive[f"ppt/slides/slide{number}.xml"] = ET.tostring(
        root, encoding="utf-8", xml_declaration=True
    )


def make_slides(archive: dict[str, bytes]) -> None:
    # 1. Title
    root = ET.fromstring(archive["ppt/slides/slide1.xml"])
    st = ui.base_slide(root, 1, "Топология", dark=True)
    ui.shape(st, 10, 760000, 650000, 2600000, 260000, [
        {"text": "НЕДЕЛЯ 9 · FIRSTORDER", "size": 1150, "bold": True, "color": ui.ORANGE}
    ])
    ui.shape(st, 11, 760000, 1550000, 9800000, 1500000, [
        {"text": "Топология и архитектура", "size": 3900, "bold": True, "color": ui.WHITE},
        {"text": "как клиент, Go API и PostgreSQL работают вместе", "size": 1800, "color": ui.MINT},
    ])
    ui.shape(st, 12, 760000, 4770000, 9200000, 600000, [
        {"text": "Next.js  ·  Go  ·  Supabase Auth  ·  PostgreSQL  ·  Docker", "size": 1550, "bold": True, "color": ui.WHITE}
    ])
    ui.shape(st, 13, 10150000, 5350000, 1200000, 650000, [
        {"text": "09", "size": 3600, "bold": True, "color": ui.ORANGE, "align": "r"}
    ])
    save_slide(archive, 1, root)

    # 2. Boundary and current/target state
    root = ET.fromstring(archive["ppt/slides/slide2.xml"])
    st = ui.base_slide(root, 2, "Границы системы")
    sid = ui.title(st, 10, "Что входит в систему", "Компоненты разделены по ответственности и обмениваются данными через явные интерфейсы.")
    sid = ui.card(st, sid, 700000, 1950000, 3400000, 2500000, "Клиент", "Next.js\nкаталог · карточки · формы\nличный кабинет", ui.GREEN)
    sid = ui.card(st, sid, 4400000, 1950000, 3400000, 2500000, "Прикладная логика", "Go API\nJWT · роли · проверки\nбизнес-транзакции", ui.ORANGE)
    sid = ui.card(st, sid, 8100000, 1950000, 3400000, 2500000, "Данные", "Supabase / PostgreSQL\n5 сервисных схем\nFK · CHECK · RLS", ui.GREEN)
    ui.shape(st, sid, 700000, 5100000, 10600000, 780000, [
        {"text": "Текущий стенд", "size": 1300, "bold": True, "color": ui.ORANGE},
        {"text": "e1: Go / health-check    ·    e2: Supabase и готовая модель данных    ·    сервисные API — следующий этап", "size": 1220, "color": ui.INK},
    ], fill=ui.MINT, radius=True)
    save_slide(archive, 2, root)

    # 3. Main topology
    root = ET.fromstring(archive["ppt/slides/slide3.xml"])
    st = ui.base_slide(root, 3, "Общая топология")
    sid = ui.title(st, 10, "Путь пользовательского запроса", "Пользователь не обращается к PostgreSQL напрямую.")
    # arrows behind cards
    ui.connector(st, 20, 2250000, 3100000, 3300000, 3100000, ui.GREEN)
    ui.connector(st, 21, 5000000, 3100000, 6050000, 3100000, ui.GREEN)
    ui.connector(st, 22, 7750000, 3100000, 8800000, 3100000, ui.GREEN)
    ui.connector(st, 23, 7000000, 4100000, 7000000, 4930000, ui.ORANGE)
    sid = ui.card(st, 30, 550000, 2450000, 1700000, 1300000, "Пользователь", "браузер / mobile", ui.GREEN)
    sid = ui.card(st, sid, 3300000, 2450000, 1700000, 1300000, "Cloudflare", "HTTPS · DNS", ui.GREEN)
    sid = ui.card(st, sid, 6050000, 2450000, 1700000, 1300000, "Next.js", "UI · REST/JSON", ui.GREEN)
    sid = ui.card(st, sid, 8800000, 2450000, 2400000, 1300000, "Go API", "права · валидация\nбизнес-логика", ui.ORANGE)
    sid = ui.card(st, sid, 5600000, 4930000, 2800000, 1250000, "Supabase Auth", "вход · JWT · сессия", ui.GREEN)
    ui.connector(st, 24, 10000000, 3750000, 10000000, 4930000, ui.GREEN)
    sid = ui.card(st, sid, 8700000, 4930000, 2600000, 1250000, "PostgreSQL", "SQL · транзакции\n5 схем", ui.GREEN)
    ui.shape(st, sid, 6300000, 4050000, 1250000, 360000, [{"text": "JWT", "size": 1050, "bold": True, "color": ui.ORANGE, "align": "ctr"}])
    ui.shape(st, sid + 1, 10050000, 4200000, 1000000, 360000, [{"text": "SQL", "size": 1050, "bold": True, "color": ui.GREEN, "align": "ctr"}])
    save_slide(archive, 3, root)

    # 4. Five services
    root = ET.fromstring(archive["ppt/slides/slide4.xml"])
    st = ui.base_slide(root, 4, "Сервисы")
    sid = ui.title(st, 10, "Пять модулей Go API", "На MVP могут запускаться одним приложением; границы уже совпадают со схемами БД.")
    cards = [
        (650000, 1900000, 3500000, 1450000, "profiles", "профили · роли · портфолио\nпредставители компаний", ui.GREEN),
        (4350000, 1900000, 3500000, 1450000, "catalog", "компании · поиск · избранное\nконтакты · модерация", ui.GREEN),
        (8050000, 1900000, 3500000, 1450000, "importer", "загрузка · очистка\nзащита от дублей", ui.GREEN),
        (2450000, 3850000, 3500000, 1450000, "orders", "публикация · бюджет · сроки\nжизненный цикл заказа", ui.ORANGE),
        (6250000, 3850000, 3500000, 1450000, "applications", "отклики · цена · срок\nвыбор исполнителя", ui.ORANGE),
    ]
    for x, y, w, h, head, body, accent in cards:
        sid = ui.card(st, sid, x, y, w, h, head, body, accent)
    ui.shape(st, sid, 2450000, 5650000, 7300000, 420000, [{"text": "Каждый модуль владеет одноимённой схемой PostgreSQL", "size": 1250, "bold": True, "color": ui.GREEN, "align": "ctr"}])
    save_slide(archive, 4, root)

    # 5. Authentication flow
    root = ET.fromstring(archive["ppt/slides/slide5.xml"])
    st = ui.base_slide(root, 5, "Авторизация")
    sid = ui.title(st, 10, "JWT проходит через весь запрос", "Supabase подтверждает личность; Go API принимает решение о доступе.")
    x_positions = [650000, 3050000, 5450000, 7850000, 10250000]
    labels = [
        ("1", "Вход", "email / пароль"),
        ("2", "Auth", "выдаёт JWT"),
        ("3", "Frontend", "добавляет Bearer"),
        ("4", "Go API", "проверяет роль"),
        ("5", "PostgreSQL", "RLS + SQL"),
    ]
    for i in range(4):
        ui.connector(st, 20 + i, x_positions[i] + 1550000, 3050000, x_positions[i + 1], 3050000, ui.ORANGE)
    for i, (num, head, body) in enumerate(labels):
        ui.shape(st, sid, x_positions[i], 2300000, 1650000, 1550000, [
            {"text": num, "size": 1150, "bold": True, "color": ui.ORANGE, "align": "ctr"},
            {"text": head, "size": 1450, "bold": True, "color": ui.GREEN, "align": "ctr"},
            {"text": body, "size": 1050, "color": ui.INK, "align": "ctr"},
        ], fill=ui.WHITE, line=ui.LINE, radius=True)
        sid += 1
    ui.shape(st, sid, 1150000, 4700000, 9900000, 950000, [
        {"text": "Правило доступа", "size": 1400, "bold": True, "color": ui.GREEN},
        {"text": "Разработчик управляет своими откликами; представитель — заказами своей компании; модератор — публикациями.", "size": 1250, "color": ui.INK},
    ], fill=ui.MINT, radius=True)
    save_slide(archive, 5, root)

    # 6. Import flow
    root = ET.fromstring(archive["ppt/slides/slide6.xml"])
    st = ui.base_slide(root, 6, "Импорт")
    sid = ui.title(st, 10, "От внешнего источника до каталога", "Сырые данные сохраняются отдельно, а каталог получает очищенную карточку.")
    ui.connector(st, 20, 2650000, 3050000, 3650000, 3050000, ui.GREEN)
    ui.connector(st, 21, 5550000, 3050000, 6550000, 3050000, ui.GREEN)
    ui.connector(st, 22, 8450000, 3050000, 9450000, 3050000, ui.GREEN)
    sid = ui.card(st, 30, 650000, 2300000, 2000000, 1550000, "Справочник", "API или тестовый CSV", ui.GREEN)
    sid = ui.card(st, sid, 3650000, 2300000, 1900000, 1550000, "import_runs", "статус · счётчики\nошибка импорта", ui.GREEN)
    sid = ui.card(st, sid, 6550000, 2300000, 1900000, 1550000, "source_records", "source + external_id\nraw_data", ui.ORANGE)
    sid = ui.card(st, sid, 9450000, 2300000, 2000000, 1550000, "companies", "очищенная карточка\nстатус сайта", ui.GREEN)
    ui.shape(st, sid, 1200000, 4550000, 9600000, 1050000, [
        {"text": "Защита качества", "size": 1400, "bold": True, "color": ui.GREEN},
        {"text": "UNIQUE(source, external_id) не создаёт дубль при повторном импорте", "size": 1220, "color": ui.INK, "bullet": True},
        {"text": "Ошибка источника фиксируется, но не останавливает каталог", "size": 1220, "color": ui.INK, "bullet": True},
    ], fill=ui.MINT, radius=True)
    save_slide(archive, 6, root)

    # 7. Deployment and decisions
    root = ET.fromstring(archive["ppt/slides/slide7.xml"])
    st = ui.base_slide(root, 7, "Развёртывание")
    sid = ui.title(st, 10, "Учебный стенд и следующий шаг", "Архитектура рассчитана на постепенное развитие без смены модели данных.")
    sid = ui.card(st, sid, 700000, 1950000, 3300000, 2300000, "e1.iq-home.kz", "Cloudflare / HTTPS\nGo backend\nконтейнерный запуск", ui.ORANGE)
    sid = ui.card(st, sid, 4450000, 1950000, 3300000, 2300000, "e2.iq-home.kz", "Supabase Auth\nPostgreSQL\n5 сервисных схем", ui.GREEN)
    sid = ui.card(st, sid, 8200000, 1950000, 3300000, 2300000, "Эксплуатация", "health-check\nжурналы\nрезервная копия", ui.GREEN)
    ui.shape(st, sid, 700000, 4950000, 10800000, 950000, [
        {"text": "Архитектурное решение", "size": 1450, "bold": True, "color": ui.GREEN},
        {"text": "На MVP — один Go-процесс с пятью модулями. При росте нагрузки модули можно выделить в отдельные сервисы без переноса таблиц между схемами.", "size": 1250, "color": ui.INK},
    ], fill=ui.MINT, radius=True)
    save_slide(archive, 7, root)


def main() -> None:
    with zipfile.ZipFile(SOURCE, "r") as zin:
        archive = {name: zin.read(name) for name in zin.namelist()}
    make_slides(archive)
    with zipfile.ZipFile(OUTPUT, "w", zipfile.ZIP_DEFLATED) as zout:
        for name, data in archive.items():
            zout.writestr(name, data)
    print(OUTPUT)


if __name__ == "__main__":
    main()
