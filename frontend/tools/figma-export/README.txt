FirstOrder — выгрузка дизайна для фронтенда

СПОСОБ 1. БРАУЗЕР / LINUX, БЕЗ ПЛАГИНА

1. Откройте файл FirstOrder:
   https://www.figma.com/design/eEBIHk6nceh158jZmZxCPe/FirstOrder
2. Откройте страницу «01 · Desktop». Кликните по пустому холсту,
   затем Ctrl+A, чтобы выделить верхнеуровневые макеты.
3. В правой панели найдите Export, нажмите +, выберите SVG.
   В настройках SVG выключите Outline text и включите Include id attribute.
   Нажмите Export. Сохраните все экраны, обычно Figma предлагает ZIP.
4. Для тех же экранов добавьте экспорт PNG с масштабом 1x и сохраните.
   PNG нужен для точной визуальной сверки, SVG — для размеров, цветов и текста.
5. Повторите для «02 · Mobile» и «00 · UI kit».
6. Прикрепите полученные архивы к этой задаче или положите их в:
   /home/nikita/Документы/first-order/frontend/design-inbox/
   После этого сообщите, что файлы готовы.

Если интерфейс Figma показывает английские названия: Export → SVG →
More export settings (…) → Outline text: off; Include id attribute: on.
Экспорт не требует лимита MCP и не изменяет файл.

СПОСОБ 2. FIGMA DESKTOP (WINDOWS / MACOS)

Этот плагин собирает один архив с JSON (включая auto layout и стили),
SVG, PNG, изображениями и переменными. Работает локально, без API-ключа.

1. Распакуйте пакет. В Figma Desktop откройте нужный дизайн.
2. Plugins → Development → Import plugin from manifest,
   выберите manifest.json из папки figma-export.
3. Если Figma требует ID: Plugins → Development → New plugin →
   Figma Design → Custom UI. Сохраните назначенный Figma id.
   Скопируйте code.js и ui.html из пакета в папку нового плагина.
   Перенесите остальные поля нашего manifest.json в сгенерированный
   manifest, сохранив выданный id. ID не выдуман и не привязан к чужому плагину.
4. Запустите FirstOrder — Export for frontend, выберите все страницы,
   нажмите «Подготовить ZIP». Дождитесь результата и нажмите
   «Скачать архив». При ненулевом количестве ошибок проверьте index.json.
5. Передайте firstorder-design-export.zip в эту задачу или design-inbox/.

Плагин не меняет слои, не отправляет данные в сеть и не запрашивает токен.
Исходные растровые изображения сохраняются в assets/*.bin: их фактический
формат определяется по сигнатуре, байты оригинала не изменяются.

Проверки пакета: синтаксис JS, тест чтения ZIP. Запуск в Figma ещё не проверен.

Официальная документация:
https://help.figma.com/hc/en-us/articles/13402894554519-Export-formats-and-settings-for-static-designs
https://developers.figma.com/docs/plugins/api/ExportSettings/
https://developers.figma.com/docs/plugins/manifest/
