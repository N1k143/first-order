# Рабочее окружение «Первого заказа»

Адрес приложения: https://e1.iq-home.kz. Supabase проекта: https://e2.iq-home.kz.

## Развёртывание 19 ноября 2026

- Сделана закрытая резервная копия PostgreSQL, ролей, предыдущего backend и конфигурации. Путь сохранён в `.deploy/latest-backup`; каталог исключён из Git.
- Миграции `001_initial.sql` и `002_runtime_integrity.sql` применены под `supabase_admin`. Журнал: `public.firstorder_schema_migrations`.
- Создана отдельная runtime-роль `firstorder_backend`; подключение через Supavisor с именем `firstorder_backend.ingeneriya`. Пароль находится только в закрытом `backend/.env`.
- PM2 `ingeneriya-catalog` запускает полный Go API на `127.0.0.1:18082`; `ingeneriya-frontend` запускает production Next.js на `127.0.0.1:18080`. Конфигурация PM2 сохранена.
- Существующий Nginx уже направляет `e1.iq-home.kz` на порт 18080. Его конфигурация не изменялась: запросы health/ready/API проксируются frontend в Go API.
- `frontend/.env.local` включает `NEXT_PUBLIC_DATA_MODE=api` и `BACKEND_URL=http://127.0.0.1:18082`. Этот режим включён в опубликованную сборку.
- Supabase `SITE_URL` и разрешённый callback настроены на `https://e1.iq-home.kz` и `/auth/callback`. Подтверждение email осталось обязательным.

## Проверено на рабочей базе

API и настоящий браузер прошли: вход компании → создание черновика → публикация → вход разработчика → отклик → подтверждение выбора → отображение принятого отклика. Проверены refresh с ротацией токена, HttpOnly/Secure/SameSite cookies, выход и отказ без сессии.

В первом прогоне два помеченных аккаунта были созданы через административную генерацию signup-ссылок без отправки почты. Этот прогон проверил бизнес-сценарий, но не публичную регистрацию и доставку письма. После него удалены только эти два аккаунта, одна тестовая компания, два заказа и два отклика. Закрытые отчёты без паролей и токенов сохранены в `.deploy/`.

SMTP Gmail подключён с паролем приложения: проверены STARTTLS и аутентификация, пересоздан только контейнер Auth. Закрытые настройки находятся в `supabase/.env` и `supabase/.smtp.local.env`, вне Git. В финальной проверке два новых тестовых аккаунта зарегистрированы через публичную форму браузера (`201`), оба письма получены и оба адреса подтверждены настоящими ссылками. Повторная отправка подтверждения компании также проверена. Подтверждение email оставалось обязательным; административное подтверждение в этом прогоне не применялось.

После подтверждения обоих адресов production Playwright успешно завершил сценарий за 12,1 с: вход компании → создание черновика → публикация → отклик разработчика → выбор исполнителя → отображение принятого отклика. Закрытые доказательства сохранены в `.deploy/final-email-e2e-report.json` и `.deploy/full-flow-foe2email-20261119-122416.json`.

Очистка финального прогона завершена по точным UUID и метке с проверкой зависимостей: удалены 2 аккаунта Auth, 2 профиля, 1 компания, 1 membership, 1 заказ и 1 отклик. Повторная проверка показала 0 оставшихся тестовых записей. Отчёт сохранён в `.deploy/email-e2e-cleanup.json`; тестовые пароли удалены. После очистки `/health`, `/ready` с bearer-токеном и `/api/v1/companies` вернули `200`.

## Обновление профиля, настроек и кабинета компании

Этот раздел описывает подготовленную процедуру; сам по себе он не подтверждает её выполнение на рабочем сайте. Сначала завершите проверки и опубликуйте проверенный commit в `main`. Рабочая копия сборки и `/data/services/ingeneriya` должны содержать этот commit. Скрипты PM2 запускают API через `deploy/run-api.sh` (он читает закрытый `backend/.env`) и Next.js из рабочего `frontend/.next`; отдельного контейнера приложения в текущей схеме нет.

Готовьте сборки в отдельной копии: сборка поверх `.next` работающего frontend может нарушить выдачу страниц и ресурсов. Команды ниже рассчитаны на одну shell-сессию и существующие процессы PM2.

```sh
set -euo pipefail
umask 077
release_source=/home/server/.codex/worktrees/merge-main/ingeneriya
live_root=/data/services/ingeneriya
release_stamp=$(date -u +%Y%m%d-%H%M%S)
release_stage="$live_root/.deploy/sections-$release_stamp"
release_backup="$live_root/.deploy/backup-sections-$release_stamp"
git -C "$live_root" pull --ff-only origin main
test "$(git -C "$release_source" rev-parse HEAD)" = "$(git -C "$live_root" rev-parse HEAD)"
mkdir -p "$release_stage" "$release_backup"
(
  cd "$release_source/backend"
  GOCACHE=/tmp/firstorder-go-cache /data/go/bin/go build -trimpath -o "$release_stage/api" ./cmd/api
)
(
  cd "$release_source/frontend"
  npm ci
  NEXT_PUBLIC_DATA_MODE=api BACKEND_URL=http://127.0.0.1:18082 npm run build
)
cp -a "$release_source/frontend/.next" "$release_stage/next"
cp -an "$live_root/frontend/.next/static/." "$release_stage/next/static/"
cp -p "$live_root/backend/bin/api" "$release_backup/api.before"
docker exec ingeneriya-supabase-db pg_dump -U supabase_admin -d postgres -Fc > "$release_backup/postgres.dump"
test -s "$release_backup/postgres.dump"
docker exec -i ingeneriya-supabase-db pg_restore --list < "$release_backup/postgres.dump" > "$release_backup/dump-contents.txt"
```

Миграция `003_profile_avatar.sql` добавляет только nullable `profiles.profiles.avatar_url text`. Если столбец уже существует, данные не меняются. Ниже её SQL и запись журнала выполняются в одной транзакции с тем же advisory lock, что использует мигратор. Проверяются ранее применённые `001`/`002`; пароли и административный DSN не передаются в командную строку.

```sh
docker exec -i ingeneriya-supabase-db psql -X -U supabase_admin -d postgres -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';
SELECT pg_advisory_xact_lock(1730191919);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.firstorder_schema_migrations WHERE name='001_initial.sql')
     OR NOT EXISTS (SELECT 1 FROM public.firstorder_schema_migrations WHERE name='002_runtime_integrity.sql') THEN
    RAISE EXCEPTION 'Apply and verify migrations 001 and 002 first';
  END IF;
END $$;
ALTER TABLE profiles.profiles ADD COLUMN IF NOT EXISTS avatar_url text;
INSERT INTO public.firstorder_schema_migrations(name)
VALUES ('003_profile_avatar.sql') ON CONFLICT (name) DO NOTHING;
COMMIT;
SELECT name FROM public.firstorder_schema_migrations ORDER BY name;
SELECT column_name,data_type,is_nullable FROM information_schema.columns
WHERE table_schema='profiles' AND table_name='profiles' AND column_name='avatar_url';
SQL
```

Обычный `backend/cmd/migrate` также поддерживает `003`: он использует административный `DATABASE_URL`, сортирует SQL-файлы и пропускает имена из журнала. Его SQL и запись журнала коммитятся отдельно, поэтому повтор после сбоя между ними должен оставаться идемпотентным; `003` этому условию соответствует. Не меняйте runtime DSN ради миграции.

После успешной миграции переключите готовую сборку. На этом шаге frontend кратковременно недоступен; в процессе сборки он продолжал работать.

```sh
pm2 stop ingeneriya-frontend
(
  cd "$live_root/frontend"
  npm ci
)
mv "$live_root/frontend/.next" "$release_backup/frontend.next.before"
mv "$release_stage/next" "$live_root/frontend/.next"
cp "$release_stage/api" "$live_root/backend/bin/api.next"
mv "$live_root/backend/bin/api.next" "$live_root/backend/bin/api"
pm2 restart ingeneriya-catalog
curl --fail --silent --show-error --retry 5 --retry-connrefused --retry-delay 1 http://127.0.0.1:18082/health
curl --fail --silent --show-error --retry 5 --retry-connrefused --retry-delay 1 'http://127.0.0.1:18082/api/v1/companies?limit=1' > /dev/null
pm2 restart ingeneriya-frontend
pm2 save
curl --fail --silent --show-error --retry 5 --retry-connrefused --retry-delay 1 https://e1.iq-home.kz/health
curl --fail --silent --show-error --retry 5 --retry-connrefused --retry-delay 1 https://e1.iq-home.kz/api/categories > /dev/null
```

При ошибке переключения остановите frontend, восстановите `api.before` и `frontend.next.before`, верните зависимости соответствующего commit и перезапустите процессы. Добавленный nullable-столбец совместим с предыдущим backend; удалять его при откате приложения не нужно. Рабочую БД из backup автоматически не восстанавливайте. Проверка новых разделов до публикации использует отдельную PostgreSQL и контролируемые Auth/API-ответы; реальные смены email/пароля и отправка писем не являются частью этих команд.
