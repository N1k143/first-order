from __future__ import annotations

import copy
import os
import zipfile
import xml.etree.ElementTree as ET


SOURCE = "/home/server/.codex/attachments/109fe611-5a1e-4629-b4f1-2228d2e2f88b/Первый_заказ_Неделя_7_Use_Case.pptx"
OUTPUT = "/data/services/ingeneriya/Первый_заказ_Неделя_8_Модель_данных.pptx"

P = "http://schemas.openxmlformats.org/presentationml/2006/main"
A = "http://schemas.openxmlformats.org/drawingml/2006/main"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
PR = "http://schemas.openxmlformats.org/package/2006/relationships"

ET.register_namespace("a", A)
ET.register_namespace("p", P)
ET.register_namespace("r", R)

W, H = 12191695, 6858000
BG = "F7F5EF"
INK = "17251F"
MUTED = "66736D"
GREEN = "174D3D"
MINT = "DCE9E3"
ORANGE = "F08B55"
WHITE = "FFFFFF"
LINE = "C9D3CE"
PALE = "EDE8DD"


def q(ns: str, tag: str) -> str:
    return f"{{{ns}}}{tag}"


def solid(parent: ET.Element, color: str) -> None:
    sf = ET.SubElement(parent, q(A, "solidFill"))
    ET.SubElement(sf, q(A, "srgbClr"), {"val": color})


def transform(parent: ET.Element, x: int, y: int, w: int, h: int) -> None:
    xf = ET.SubElement(parent, q(A, "xfrm"))
    ET.SubElement(xf, q(A, "off"), {"x": str(x), "y": str(y)})
    ET.SubElement(xf, q(A, "ext"), {"cx": str(w), "cy": str(h)})


def shape(
    sp_tree: ET.Element,
    sid: int,
    x: int,
    y: int,
    w: int,
    h: int,
    paragraphs: list[dict],
    fill: str | None = None,
    line: str | None = None,
    radius: bool = False,
    margin: int = 140000,
    valign: str = "ctr",
) -> None:
    sp = ET.SubElement(sp_tree, q(P, "sp"))
    nv = ET.SubElement(sp, q(P, "nvSpPr"))
    ET.SubElement(nv, q(P, "cNvPr"), {"id": str(sid), "name": f"Shape {sid}"})
    ET.SubElement(nv, q(P, "cNvSpPr"), {"txBox": "1"})
    ET.SubElement(nv, q(P, "nvPr"))

    sp_pr = ET.SubElement(sp, q(P, "spPr"))
    transform(sp_pr, x, y, w, h)
    geom = ET.SubElement(sp_pr, q(A, "prstGeom"), {"prst": "roundRect" if radius else "rect"})
    ET.SubElement(geom, q(A, "avLst"))
    if fill:
        solid(sp_pr, fill)
    else:
        ET.SubElement(sp_pr, q(A, "noFill"))
    ln = ET.SubElement(sp_pr, q(A, "ln"), {"w": "12000"})
    if line:
        solid(ln, line)
    else:
        ET.SubElement(ln, q(A, "noFill"))

    tx = ET.SubElement(sp, q(P, "txBody"))
    ET.SubElement(
        tx,
        q(A, "bodyPr"),
        {
            "wrap": "square",
            "anchor": valign,
            "lIns": str(margin),
            "rIns": str(margin),
            "tIns": str(margin),
            "bIns": str(margin),
        },
    )
    ET.SubElement(tx, q(A, "lstStyle"))

    for item in paragraphs:
        p = ET.SubElement(tx, q(A, "p"))
        attrs = {"algn": item.get("align", "l")}
        if item.get("bullet"):
            attrs.update({"marL": "190000", "indent": "-140000"})
        ppr = ET.SubElement(p, q(A, "pPr"), attrs)
        if item.get("bullet"):
            ET.SubElement(ppr, q(A, "buChar"), {"char": "•"})
        r = ET.SubElement(p, q(A, "r"))
        rpr = ET.SubElement(
            r,
            q(A, "rPr"),
            {
                "lang": "ru-RU",
                "sz": str(item.get("size", 1800)),
                "b": "1" if item.get("bold") else "0",
            },
        )
        solid(rpr, item.get("color", INK))
        ET.SubElement(rpr, q(A, "latin"), {"typeface": "Aptos"})
        ET.SubElement(r, q(A, "t")).text = item["text"]
        end = ET.SubElement(p, q(A, "endParaRPr"), {"lang": "ru-RU", "sz": str(item.get("size", 1800))})
        solid(end, item.get("color", INK))


def connector(sp_tree: ET.Element, sid: int, x1: int, y1: int, x2: int, y2: int, color: str = LINE, arrow: bool = True) -> None:
    cx = ET.SubElement(sp_tree, q(P, "cxnSp"))
    nv = ET.SubElement(cx, q(P, "nvCxnSpPr"))
    ET.SubElement(nv, q(P, "cNvPr"), {"id": str(sid), "name": f"Connector {sid}"})
    ET.SubElement(nv, q(P, "cNvCxnSpPr"))
    ET.SubElement(nv, q(P, "nvPr"))
    pr = ET.SubElement(cx, q(P, "spPr"))
    xf = ET.SubElement(pr, q(A, "xfrm"), {"flipV": "1" if y2 < y1 else "0", "flipH": "1" if x2 < x1 else "0"})
    ET.SubElement(xf, q(A, "off"), {"x": str(min(x1, x2)), "y": str(min(y1, y2))})
    ET.SubElement(xf, q(A, "ext"), {"cx": str(abs(x2 - x1)), "cy": str(abs(y2 - y1))})
    geom = ET.SubElement(pr, q(A, "prstGeom"), {"prst": "line"})
    ET.SubElement(geom, q(A, "avLst"))
    ln = ET.SubElement(pr, q(A, "ln"), {"w": "22000"})
    solid(ln, color)
    if arrow:
        ET.SubElement(ln, q(A, "tailEnd"), {"type": "none"})
        ET.SubElement(ln, q(A, "headEnd"), {"type": "triangle", "w": "sm", "len": "sm"})


def base_slide(root: ET.Element, number: int, section: str, dark: bool = False) -> ET.Element:
    c_sld = root.find(q(P, "cSld"))
    sp_tree = c_sld.find(q(P, "spTree"))
    for child in list(sp_tree)[2:]:
        sp_tree.remove(child)

    bg = GREEN if dark else BG
    shape(sp_tree, 2, 0, 0, W, H, [], fill=bg)
    if not dark:
        shape(sp_tree, 3, 0, 0, 180000, H, [], fill=GREEN)
        shape(sp_tree, 4, 700000, 285000, 2200000, 220000, [{"text": section.upper(), "size": 1050, "bold": True, "color": ORANGE}])
        shape(sp_tree, 5, 11200000, 285000, 420000, 220000, [{"text": f"{number:02d}", "size": 1050, "bold": True, "color": MUTED, "align": "r"}])
    return sp_tree


def title(sp_tree: ET.Element, sid: int, text: str, subtitle: str | None = None) -> int:
    shape(sp_tree, sid, 700000, 640000, 10500000, 720000, [{"text": text, "size": 2800, "bold": True, "color": INK}])
    sid += 1
    if subtitle:
        shape(sp_tree, sid, 700000, 1320000, 10400000, 430000, [{"text": subtitle, "size": 1450, "color": MUTED}])
        sid += 1
    return sid


def card(sp_tree: ET.Element, sid: int, x: int, y: int, w: int, h: int, head: str, body: str, accent: str = GREEN) -> int:
    shape(sp_tree, sid, x, y, w, h, [{"text": head, "size": 1550, "bold": True, "color": accent}, {"text": body, "size": 1120, "color": INK}], fill=WHITE, line=LINE, radius=True, margin=190000)
    return sid + 1


def make_slides(archive: dict[str, bytes]) -> None:
    # Slide 1
    root = ET.fromstring(archive["ppt/slides/slide1.xml"])
    st = base_slide(root, 1, "Модель данных", dark=True)
    shape(st, 10, 760000, 650000, 2300000, 260000, [{"text": "НЕДЕЛЯ 8 · FIRSTORDER", "size": 1150, "bold": True, "color": ORANGE}])
    shape(st, 11, 760000, 1720000, 9300000, 1350000, [{"text": "Модель данных", "size": 4200, "bold": True, "color": WHITE}, {"text": "от каталога компаний до выбора исполнителя", "size": 1900, "color": MINT}])
    shape(st, 12, 760000, 4800000, 9000000, 560000, [{"text": "5 сервисных схем  ·  13 таблиц  ·  PostgreSQL / Supabase", "size": 1600, "bold": True, "color": WHITE}])
    shape(st, 13, 10150000, 5350000, 1200000, 650000, [{"text": "08", "size": 3600, "bold": True, "color": ORANGE, "align": "r"}])
    archive["ppt/slides/slide1.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    # Slide 2
    root = ET.fromstring(archive["ppt/slides/slide2.xml"])
    st = base_slide(root, 2, "Контекст")
    sid = title(st, 10, "Два сценария — одна модель", "Структура данных следует пользовательским действиям, а не набору случайных таблиц.")
    sid = card(st, sid, 750000, 2050000, 5000000, 2650000, "01 · Найти компанию", "Категория → карточка компании → избранное → отметка о контакте", GREEN)
    sid = card(st, sid, 6300000, 2050000, 5000000, 2650000, "02 · Получить заказ", "Компания → заказ → отклики → один выбранный исполнитель", ORANGE)
    shape(st, sid, 750000, 5350000, 10550000, 700000, [{"text": "Общие данные: пользователи, роли, портфолио, импорт и модерация", "size": 1550, "bold": True, "color": GREEN, "align": "ctr"}], fill=MINT, radius=True)
    archive["ppt/slides/slide2.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    # Slide 3
    root = ET.fromstring(archive["ppt/slides/slide3.xml"])
    st = base_slide(root, 3, "Архитектура данных")
    sid = title(st, 10, "Пять схем — пять зон ответственности", "Каждый Go-сервис работает со своей частью предметной области.")
    cards = [
        (700000, 1950000, 3500000, 1500000, "profiles", "профили · портфолио\nпредставители компаний", GREEN),
        (4350000, 1950000, 3500000, 1500000, "catalog", "компании · категории\nизбранное · модерация", GREEN),
        (8000000, 1950000, 3500000, 1500000, "importer", "запуски импорта\nисходные записи", GREEN),
        (2550000, 3900000, 3500000, 1500000, "orders", "заказы · бюджет\nстатусы · исполнитель", ORANGE),
        (6300000, 3900000, 3500000, 1500000, "applications", "отклики · цена\nсрок · статус", ORANGE),
    ]
    for x, y, w, h, head, body, accent in cards:
        sid = card(st, sid, x, y, w, h, head, body, accent)
    archive["ppt/slides/slide3.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    # Slide 4
    root = ET.fromstring(archive["ppt/slides/slide4.xml"])
    st = base_slide(root, 4, "ER-диаграмма")
    sid = title(st, 10, "Ключевые связи", "Основной путь данных: пользователь → отклик → заказ → компания.")
    # Connectors first, so they stay behind cards.
    connector(st, 20, 2600000, 3000000, 3750000, 3000000, GREEN)
    connector(st, 21, 5550000, 3000000, 6700000, 3000000, GREEN)
    connector(st, 22, 8500000, 3000000, 9650000, 3000000, GREEN)
    connector(st, 23, 10300000, 3850000, 10300000, 4550000, ORANGE)
    connector(st, 24, 1800000, 3850000, 1800000, 4550000, ORANGE)
    connector(st, 25, 3850000, 5200000, 4900000, 5200000, LINE)
    connector(st, 26, 6750000, 5200000, 7800000, 5200000, LINE)
    sid = card(st, 30, 700000, 2400000, 1900000, 1250000, "profiles", "user_id · role", GREEN)
    sid = card(st, sid, 3750000, 2400000, 1800000, 1250000, "applications", "order_id\ndeveloper_id", ORANGE)
    sid = card(st, sid, 6700000, 2400000, 1800000, 1250000, "orders", "company_id\nselected_application", ORANGE)
    sid = card(st, sid, 9650000, 2400000, 1900000, 1250000, "companies", "category_id\nwebsite_status", GREEN)
    sid = card(st, sid, 700000, 4550000, 2200000, 1250000, "portfolio_items", "1 профиль → N работ", GREEN)
    sid = card(st, sid, 3850000, 4550000, 2900000, 1250000, "company_members", "M пользователей ↔ N компаний", GREEN)
    sid = card(st, sid, 7800000, 4550000, 2600000, 1250000, "source_records", "источник → компания", GREEN)
    archive["ppt/slides/slide4.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    # Slide 5
    root = ET.fromstring(archive["ppt/slides/slide5.xml"])
    st = base_slide(root, 5, "Каталог")
    sid = title(st, 10, "Компания — центр каталога", "Связанные таблицы добавляют фильтрацию, персональные действия и контроль качества.")
    connector(st, 20, 5550000, 3250000, 3800000, 2350000, LINE)
    connector(st, 21, 5550000, 3250000, 3800000, 4300000, LINE)
    connector(st, 22, 7600000, 3250000, 9250000, 2350000, LINE)
    connector(st, 23, 7600000, 3250000, 9250000, 4300000, LINE)
    sid = card(st, 30, 4900000, 2550000, 2700000, 1650000, "catalog.companies", "контакты · сайт · источник\nстатус проверки · видимость", GREEN)
    sid = card(st, sid, 900000, 1750000, 2900000, 1250000, "categories", "фильтр по сфере", GREEN)
    sid = card(st, sid, 900000, 4050000, 2900000, 1250000, "favorites", "личное избранное", GREEN)
    sid = card(st, sid, 9250000, 1750000, 2300000, 1250000, "company_contacts", "план → контакт → ответ", GREEN)
    sid = card(st, sid, 9250000, 4050000, 2300000, 1250000, "correction_requests", "проверка исправлений", GREEN)
    shape(st, sid, 4200000, 5150000, 4200000, 650000, [{"text": "moderation_actions · история решений", "size": 1350, "bold": True, "color": ORANGE, "align": "ctr"}], fill=PALE, radius=True)
    archive["ppt/slides/slide5.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    # Slide 6
    root = ET.fromstring(archive["ppt/slides/slide6.xml"])
    st = base_slide(root, 6, "Заказы и отклики")
    sid = title(st, 10, "Один заказ → один исполнитель", "Выбор выполняется транзакционно: права и состояние проверяются повторно.")
    y = 2300000
    xs = [700000, 3000000, 5450000, 7900000, 10100000]
    labels = [("draft", "черновик"), ("open", "приём откликов"), ("contractor_selected", "исполнитель выбран"), ("completed", "завершён"), ("closed", "закрыт")]
    for i in range(4):
        connector(st, 20 + i, xs[i] + 1650000, y + 480000, xs[i + 1], y + 480000, ORANGE)
    for i, (head, body) in enumerate(labels):
        sid = card(st, sid, xs[i], y, 1800000, 980000, head, body, ORANGE if i == 2 else GREEN)
    sid = card(st, sid, 800000, 4100000, 3000000, 1250000, "submitted", "отклик подан", GREEN)
    sid = card(st, sid, 4550000, 4100000, 3000000, 1250000, "accepted", "один выбранный отклик", ORANGE)
    sid = card(st, sid, 8300000, 4100000, 3000000, 1250000, "rejected", "остальные отклонены", GREEN)
    shape(st, sid, 800000, 5650000, 10300000, 430000, [{"text": "UNIQUE(order_id, developer_id) · FK · CHECK(status) · select_application()", "size": 1250, "bold": True, "color": GREEN, "align": "ctr"}])
    archive["ppt/slides/slide6.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)

    # Slide 7
    root = ET.fromstring(archive["ppt/slides/slide7.xml"])
    st = base_slide(root, 7, "Результат")
    sid = title(st, 10, "Модель готова к следующему этапу", "Структура согласована с Use Case недели 7 и фактической базой на e2.iq-home.kz.")
    metrics = [
        (700000, "5", "сервисных схем"),
        (3500000, "13", "прикладных таблиц"),
        (6300000, "0", "невалидных FK"),
        (9100000, "1", "исполнитель на заказ"),
    ]
    for x, value, label in metrics:
        shape(st, sid, x, 2050000, 2350000, 1700000, [{"text": value, "size": 3400, "bold": True, "color": ORANGE, "align": "ctr"}, {"text": label, "size": 1250, "bold": True, "color": INK, "align": "ctr"}], fill=WHITE, line=LINE, radius=True)
        sid += 1
    shape(st, sid, 700000, 4450000, 10650000, 1250000, [
        {"text": "Целостность", "size": 1450, "bold": True, "color": GREEN},
        {"text": "FK связывают пользователей, компании, заказы и отклики", "size": 1250, "color": INK, "bullet": True},
        {"text": "CHECK ограничивает статусы, бюджет и видимость", "size": 1250, "color": INK, "bullet": True},
        {"text": "RLS разделяет данные по ролям; backend остаётся единой точкой доступа", "size": 1250, "color": INK, "bullet": True},
    ], fill=MINT, radius=True, valign="ctr")
    archive["ppt/slides/slide7.xml"] = ET.tostring(root, encoding="utf-8", xml_declaration=True)


def trim_presentation(archive: dict[str, bytes]) -> None:
    pres = ET.fromstring(archive["ppt/presentation.xml"])
    sld_lst = pres.find(q(P, "sldIdLst"))
    for child in list(sld_lst)[7:]:
        sld_lst.remove(child)
    archive["ppt/presentation.xml"] = ET.tostring(pres, encoding="utf-8", xml_declaration=True)

    rels = ET.fromstring(archive["ppt/_rels/presentation.xml.rels"])
    for child in list(rels):
        target = child.attrib.get("Target", "")
        if target in {"slides/slide8.xml", "slides/slide9.xml", "slides/slide10.xml"}:
            rels.remove(child)
    archive["ppt/_rels/presentation.xml.rels"] = ET.tostring(rels, encoding="utf-8", xml_declaration=True)

    for n in (8, 9, 10):
        archive.pop(f"ppt/slides/slide{n}.xml", None)
        archive.pop(f"ppt/slides/_rels/slide{n}.xml.rels", None)

    if "docProps/app.xml" in archive:
        app = ET.fromstring(archive["docProps/app.xml"])
        ns = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
        slides = app.find(q(ns, "Slides"))
        if slides is not None:
            slides.text = "7"
        archive["docProps/app.xml"] = ET.tostring(app, encoding="utf-8", xml_declaration=True)


def main() -> None:
    with zipfile.ZipFile(SOURCE, "r") as zin:
        archive = {name: zin.read(name) for name in zin.namelist()}
    make_slides(archive)
    trim_presentation(archive)
    with zipfile.ZipFile(OUTPUT, "w", zipfile.ZIP_DEFLATED) as zout:
        for name, data in archive.items():
            zout.writestr(name, data)
    print(OUTPUT)


if __name__ == "__main__":
    main()
