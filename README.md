# WalkTogether MVP на macOS

WalkTogether работает как Telegram Mini App с Go API, PostgreSQL, polling-ботом и notification worker. Включены друзья, приватное расписание, приглашения на прогулки, подбор общего времени, карта, избранные места и экспорт календаря.

## Требования

- macOS с Homebrew;
- Go 1.22 или новее;
- Node.js 22.12+ (проверено на Node 26), npm;
- PostgreSQL 15+;
- Telegram аккаунт и BotFather;
- Cloudflare Tunnel (`brew install cloudflared`) для HTTPS-доступа Telegram к локальному Vite.

## 1. PostgreSQL

Запустите установленную Homebrew версию PostgreSQL. В этом проектном окружении использовался PostgreSQL 17:

```bash
brew services start postgresql@17
pg_isready -h 127.0.0.1 -p 5432
```

Создайте отдельного владельца и базу WalkTogether. `createuser` запросит пароль в терминале; не отправляйте его в чат:

```bash
createuser -h 127.0.0.1 --login --pwprompt walktogether
createdb -h 127.0.0.1 -O walktogether walktogether
```

Если установлен другой formula PostgreSQL, используйте его имя в `brew services` и убедитесь, что сервер слушает порт `5432`.

## Первый администратор

Регистрация остаётся закрытой. После первого запуска API оно применит миграции; затем в отдельном терминале, где `.env` экспортирован, вручную создайте только собственную первичную запись. Укажите свой настоящий числовой Telegram ID локально и не публикуйте его:

```bash
psql "$POSTGRES_DSN" -v telegram_id=123456789 <<'SQL'
INSERT INTO users (telegram_id, display_name, role, is_admin)
VALUES (:'telegram_id'::bigint, 'WalkTogether Admin', 'admin', TRUE)
ON CONFLICT (telegram_id) DO UPDATE
SET role = 'admin', is_admin = TRUE, is_active = TRUE, is_deleted = FALSE, updated_at = NOW();
SQL
```

Установите `MAIN_ADMIN_TELEGRAM_ID` в `.env` равным тому же ID и перезапустите API. Теперь этот Telegram аккаунт может войти без приглашения и создать первую invite-ссылку; остальные регистрации по-прежнему требуют действующий invite token.

## 2. Секреты и переменные

В корне проекта:

```bash
cp .env.example .env
chmod 600 .env
```

Откройте `.env` локально и заполните:

- `POSTGRES_DSN`: DSN созданной локальной БД. Для указанного примера: `postgres://walktogether:<URL-encoded-password>@127.0.0.1:5432/walktogether?sslmode=disable`.
- `TELEGRAM_BOT_TOKEN`: новый token, созданный через BotFather. Старый token из прежнего example считать отозванным/скомпрометированным; не используйте его.
- `TELEGRAM_BOT_USERNAME`: username нового бота без `@`.
- `MAIN_ADMIN_TELEGRAM_ID`: числовой Telegram ID владельца первичной администраторской записи, тот же ID, который внесён в локальную bootstrap SQL команду.
- `TELEGRAM_MINI_APP_URL`: `https://t.me/<bot_username>` для Main Mini App, настроенного в BotFather.
- `JWT_SECRET`: случайная длинная строка. Пока `APP_ENV=production`, значение `change-me` запрещено.
- `APP_ENV=production`: включает HttpOnly, Secure session cookie. Mini App и API будут открываться через HTTPS tunnel.
- `VITE_API_BASE_URL`: оставить пустым, чтобы frontend использовал тот же origin.
- `VITE_TUNNEL_HOST`: заполнить hostname Cloudflare tunnel без `https://` и без пути, например `random-words.trycloudflare.com`.
- `TELEGRAM_BOT_MODE=polling`: локальный режим; webhook URL и secret не нужны.
- `WORKER_CONCURRENCY`: число outbox worker loops, от 1 до 32.
- `MAP_GEOCODING_URL` и `MAP_GEOCODING_USER_AGENT`: адресный поиск через Nominatim. Ключ не нужен. Для карты используется OpenStreetMap tiles; `VITE_MAP_TILE_URL` позволяет заменить поставщика плиток.

Публичные сервисы OpenStreetMap работают по правилам использования и без SLA. В API адресный поиск ограничен одним запросом в секунду. Для большого production трафика укажите собственный или коммерческий geocoding/tile provider через эти конфигурации.

Секреты не помещайте в frontend переменные и не присылайте их в чат. Корневой `.env` исключён из Git.

Go не загружает `.env` автоматически. В каждом новом терминале из корня проекта экспортируйте значения:

```bash
cd /path/to/WalkTogether
set -a
source .env
set +a
```

Повторите эти три команды в каждом терминале, где запускается Go или Vite процесс.

## 3. Установить frontend зависимости

Из корня проекта:

```bash
cd frontend
npm ci
```

Версии Vite/Vitest в lockfile обновлены до patched releases; `npm audit` должен завершаться с `found 0 vulnerabilities`.

## 4. Запустить процессы

Каждый процесс запускайте в отдельном терминале. Перед каждой командой экспортируйте `.env` как указано выше.

API и автоматические миграции:

```bash
cd backend
go run ./cmd/api
```

Bot long polling:

```bash
cd backend
go run ./cmd/bot
```

Notification worker:

```bash
cd backend
go run ./cmd/worker
```

Bot и API завершаются с ошибкой при недоступной БД/неудачных миграциях. Запускайте один bot process: он получает PostgreSQL advisory lock. Worker использует `SKIP LOCKED`, поэтому несколько worker loops могут работать в одном процессе.

## 5. HTTPS через Cloudflare Quick Tunnel

В отдельном терминале запустите frontend и tunnel. Для первого запуска Vite может быть открыт только на localhost; tunnel URL будет выведен Cloudflare:

```bash
cd frontend
npm run dev -- --host 0.0.0.0
```

В другом терминале:

```bash
cloudflared tunnel --url http://127.0.0.1:3000
```

Скопируйте hostname вида `random-words.trycloudflare.com` в `.env`:

```dotenv
VITE_TUNNEL_HOST=random-words.trycloudflare.com
VITE_API_BASE_URL=
```

Остановите и заново запустите Vite с заново экспортированным `.env`. Vite разрешает только указанный tunnel hostname и проксирует `/api/*` на `http://127.0.0.1:8080`. Telegram WebView видит frontend, API и session cookie на одном HTTPS origin; backend выставляет Secure cookie из-за `APP_ENV=production`. CORS не требуется и не включён.

Quick Tunnel hostname временный. При его изменении обновите `VITE_TUNNEL_HOST`, перезапустите Vite и измените URL Mini App в BotFather. Для повторяющихся тестов можно настроить именованный Cloudflare Tunnel, не меняя приложение.

## 6. BotFather

В BotFather для нового бота:

1. Выполните `/newbot`, задайте имя и username, сохраните новый token только в локальный `.env`.
2. Откройте настройки бота, выберите **Bot Settings → Configure Mini App → Enable Mini App** (или **Main Mini App**).
3. Укажите текущий `https://<VITE_TUNNEL_HOST>` URL и сохраните.
4. В `.env` установите `TELEGRAM_BOT_USERNAME` и `TELEGRAM_MINI_APP_URL=https://t.me/<bot_username>`.
5. Проверьте `TELEGRAM_BOT_MODE=polling`. Webhook на локальном этапе не настраивается.

Invite link открывает чат бота через `?start=TOKEN`. Бот отвечает кнопкой на настроенное Telegram Mini App; параметр передаётся как `startapp=TOKEN`. Mini App читает его из Telegram SDK `start_param`, а backend самостоятельно проверяет подписанный `initData` и одноразовый invite token.

## 7. Ручная проверка в Telegram

1. Убедитесь, что PostgreSQL принимает соединения и API, bot, worker и Vite работают в своих терминалах.
2. Убедитесь, что Cloudflare Tunnel показывает активный HTTPS hostname и он совпадает с `VITE_TUNNEL_HOST`.
3. Откройте в Telegram профиль нового бота и нажмите **Start**; затем откройте Main Mini App.
4. Проверьте в Mini App успешный вход и отсутствие ошибок в API/Vite терминалах. Не выводите initData или token в логи.
5. В «Друзья → Приглашения» создайте ссылку. Откройте её со второго Telegram аккаунта: бот должен показать кнопку запуска Mini App, новый пользователь регистрируется только с валидным приглашением.
6. С двух аккаунтов найдите друг друга, отправьте запрос, примите его и проверьте список друзей.
7. Убедитесь, что worker отправил уведомление в Telegram адресату запроса/принятия.
8. Остановите и перезапустите API, bot, worker и PostgreSQL. Пользователь, отношения и приглашения должны сохраниться.

Эти Telegram UI шаги требуют реального бота и двух аккаунтов; автоматически они не запускались.

## Проверки разработчика

```bash
cd backend
gofmt -w cmd/api/*.go cmd/bot/*.go cmd/worker/*.go internal/auth/*.go internal/config/*.go internal/db/*.go
TEST_POSTGRES_DSN='postgres://walktogether:<password>@127.0.0.1:5432/walktogether_test?sslmode=disable' go test ./... -count=1
```

Создайте `walktogether_test` отдельно от рабочей `walktogether` БД. PostgreSQL integration tests применяют все миграции и очищают таблицы пользователей тестовой БД.

```bash
cd frontend
npm audit
npm test
npm run build
```

## Диагностика

- API завершается на старте: проверьте `pg_isready`, DSN и `POSTGRES_DSN`; API намеренно не работает без PostgreSQL и успешных миграций.
- `password authentication failed`: проверьте пароль роли PostgreSQL и URL encoding пароля в DSN.
- Бот сообщает о занятом process lock: остановите второй `cmd/bot`; polling и webhook одновременно не поддерживаются.
- Telegram показывает недоступную страницу: проверьте BotFather Mini App URL, активность Cloudflare Tunnel и совпадение `VITE_TUNNEL_HOST`.
- Vite отвечает 403 на tunnel host: укажите только hostname, без схемы, пути и завершающего `/`, затем перезапустите Vite.
- API 401 после HTTPS запуска: проверьте `APP_ENV=production`, session cookie и что вызовы frontend идут на том же origin (`VITE_API_BASE_URL` пустой).
- Запросы к `/api` не проходят: убедитесь, что Go API слушает `:8080`, а frontend запущен через `npm run dev` с Vite proxy.
- `npm audit` сообщает advisory: не используйте `--force` без проверки совместимости; текущий lockfile проверен и содержит 0 известных уязвимостей.

## Текущий объём MVP

- В расписании можно переключать месяцы, отмечать занятость дня, добавлять/редактировать/удалять интервалы и задавать повторяющиеся правила.
- Доступность друзей показывается с учётом privacy настроек расписания. Если точные интервалы скрыты, API не использует их для расчёта общих слотов.
- Прогулки поддерживают создание, приглашение друзей, принятие/отклонение, ожидание ответа от всех обязательных участников, перенос с повторным подтверждением, отмену, завершение и историю.
- Место встречи можно выбрать на карте, найти по адресу, выбрать из избранного или получить через геолокацию после нажатия пользователем.
- Подтверждённые прогулки можно скачать как ICS или открыть в Google Calendar/Outlook. Автоматической синхронизации нет.

Предложение альтернативного времени участником пока не реализовано. Прогулки без ответа истекают по времени начала; подтверждённые встречи ставят напоминание за час до начала. Перед production проверьте реальный Telegram сценарий с двумя аккаунтами, задайте новый bot token и настройте постоянный backend, PostgreSQL и worker. Публичный бесплатный тариф может останавливать фоновые процессы или удалять базу; храните резервную копию и проверьте ограничения выбранного хостинга.

Для бесплатного постоянного тестового размещения подготовлена инструкция OCI Always Free ARM VM в [`deploy/oci-arm64/README.md`](deploy/oci-arm64/README.md). Там же лежат systemd units и Nginx reverse proxy; развертывание требует вашей OCI учётной записи, DNS hostname и новых Telegram секретов.
