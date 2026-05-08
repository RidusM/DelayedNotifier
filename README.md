# Delayed Notifier

Сервис отложенных уведомлений. Принимает запросы на создание уведомлений с указанием времени отправки, хранит их в PostgreSQL, публикует в RabbitMQ в нужное время и отправляет через Email (SMTP) или Telegram (Bot API). При ошибке повторяет попытку с экспоненциальной задержкой.
Поддерживает гибкую регистрацию пользователей: через сайт (Email) или Telegram-бота, с возможностью привязки аккаунтов через одноразовые токены.

---

## Содержание

- [Возможности](#возможности)
- [Архитектура](#архитектура)
- [Быстрый старт](#быстрый-старт)
- [Конфигурация](#конфигурация)
- [API](#api)
- [Telegram: привязка аккаунта](#telegram-привязка-аккаунта)
- [Настройка Email (Gmail)](#настройка-email-gmail)
- [Разработка](#разработка)
- [Структура проекта](#структура-проекта)
- [Схема БД](#схема-бд)

---

## Возможности

- **REST API** - регистрация пользователей, создание, получение статуса и отмена уведомлений
- **Два канала доставки** - Email (SMTP) и Telegram (Bot API)
- Гибкая идентификация - получатель определяется автоматически по `user_id` (Email берется из профиля, Telegram ID - из профиля или подписки бота)
- **Привязка аккаунтов** - механизм Deep Linking (/start=TOKEN) для связи Email-аккаунта с Telegram
- **Фоновая обработка** - периодический опрос БД, публикация в RabbitMQ
- **Retry с экспоненциальной задержкой** - до `SERVICE_MAX_RETRIES` попыток
- **Redis-кэш** - быстрый ответ на `GET /notify/{id}` без похода в БД
- **Swagger UI** - `/swagger/index.html`
- **Веб-интерфейс** - `/` для управления сервисом без curl

---

## Архитектура

```
HTTP-запрос / Telegram Bot
    │
    ▼
NotifyHandler / Telegram Polling
    │
    ▼
NotifyService (Бизнес-логика)
    │
    ├── UserRepository      (PostgreSQL) — пользователи (email, telegram_id)
    ├── NotifyRepository    (PostgreSQL) — уведомления
    ├── CacheRepository     (Redis)      — кэш статусов
    └── Publisher           (RabbitMQ)   — публикация в очередь
           │
           ▼
    Consumer (RabbitMQ)
           │
           ▼
    MultiSender
    ├── EmailSender    — отправка через SMTP
    └── TelegramSender — отправка через Bot API
```

**Жизненный цикл уведомления:**

```
waiting → in_process → sent
                    ↘ failed → waiting  (retry с задержкой, до MAX_RETRIES)
                             → failed   (исчерпаны все попытки)
cancelled (отменено до отправки)
```

**Retry-задержки** (базовая задержка 5 минут, множитель 2):

| Попытка | Задержка |
|---------|----------|
| 1       | 5 мин    |
| 2       | 10 мин   |
| 3       | 20 мин   |

---

## Быстрый старт

### Требования

- [Docker](https://docs.docker.com/get-docker/) и Docker Compose
- Go 1.24+ (только для локальной разработки без Docker)

### Запуск через Docker Compose (рекомендуется)

```bash
# 1. Клонировать репозиторий
git clone https://github.com/RidusM/delayed-notifier
cd delayed-notifier

# 2. Скопировать конфиг и заполнить переменные
cp .env.example .env
# Отредактировать .env: добавить SMTP-креденшалы или TG_TOKEN

# 3. Запустить всё (БД + Redis + RabbitMQ + приложение)
make compose-up
```

Сервис будет доступен на `http://localhost:8080`.

### Запуск только инфраструктуры (для локальной разработки)

```bash
# Поднять БД, Redis, RabbitMQ
make infra-up

# Применить миграции
make migrate-up

# Запустить приложение
make run
```

---

## Конфигурация

Все параметры задаются через переменные окружения (файл `.env`).  
Скопируйте `.env.example` в `.env` и заполните нужные значения.

### Приложение

| Переменная    | По умолчанию       | Описание                                     |
|---------------|--------------------|----------------------------------------------|
| `APP_NAME`    | `delayed-notifier` | Название сервиса (используется в логах)      |
| `APP_VERSION` | `1.0.0`            | Версия                                       |
| `ENV`         | `local`            | Окружение: `local`, `dev`, `staging`, `prod` |

### Сервис (логика retry)

| Переменная              | По умолчанию | Описание                              |
|-------------------------|--------------|---------------------------------------|
| `SERVICE_QUERY_LIMIT`   | `10`         | Уведомлений за один цикл обработки    |
| `SERVICE_RETRY_DELAY`   | `5m`         | Базовая задержка перед повтором       |
| `SERVICE_MAX_RETRIES`   | `3`          | Максимальное число попыток            |

### База данных

| Переменная           | По умолчанию                                                     |
|----------------------|------------------------------------------------------------------|
| `DB_DSN`             | `postgres://postgres:postgres@db:5432/notify_db?sslmode=disable` |
| `DB_POOL_MAX`        | `20`                                                             |
| `DB_CONN_ATTEMPTS`   | `5`                                                              |

### Redis

| Переменная      | По умолчанию       |
|-----------------|--------------------|
| `CACHE_ADDR`          | `redis:6379` |
| `CACHE_PASSWORD`      | _(пусто)_    |
| `CACHE_DB`            | `0`          |
| `CACHE_DIAL_TIMEOUT`  | `5s`         |
| `CACHE_READ_TIMEOUT`  | `3s`         |
| `CACHE_WRITE_TIMEOUT` | `3s`         |
| `CACHE_POOL_SIZE`     | `20`         |

### RabbitMQ

| Переменная               | По умолчанию                               |
|--------------------------|--------------------------------------------|
| `RABBIT_URL`                    | `amqp://guest:guest@rabbitmq:5672/` |
| `RABBIT_CONNECTION_NAME`        | _(пусто)_                           |
| `RABBIT_CONNECT_TIMEOUT`        | `30s`                               |
| `RABBIT_HEARTBEAT`              | `10s`                               |
| `RABBIT_EXCHANGE`               | `notifications`                     |
| `RABBIT_CONTENT_TYPE`           | _(пусто)_                           |
| `RABBIT_ATTEMPTS`               | `3`                                 |
| `RABBIT_DELAY`                  | `1s`                                |
| `RABBIT_BACKOFF`                | `2.0`                               |
| `RABBIT_WORKERS`                | `2`                                 |
| `RABBIT_PREFETCH`               | `10`                                |
| `RABBIT_QUEUE_PROCESS_INTERVAL` | `5s`                                |

### Email (SMTP)

> Если `SMTP_HOST` не задан — email-отправка отключена.

| Переменная      | По умолчанию          | Описание               |
|-----------------|-----------------------|------------------------|
| `SMTP_HOST`     | `smtp.gmail.com`      | SMTP-сервер            |
| `SMTP_PORT`     | `587`                 | Порт (STARTTLS)        |
| `SMTP_USERNAME` | _(пусто)_             | Логин                  |
| `SMTP_PASSWORD` | _(пусто)_             | Пароль / App Password  |
| `SMTP_FROM`     | `noreply@example.com` | Адрес отправителя      |

### Telegram

> Если `TG_TOKEN` не задан — Telegram-отправка отключена.

| Переменная | Описание       |
|------------|----------------|
| `TG_TOKEN` | Токен бота     |
| `TG_ALIAS` | Название бота  |

### HTTP-сервер

| Переменная                 | По умолчанию |
|----------------------------|--------------|
| `HTTP_HOST`                | `0.0.0.0`    |
| `HTTP_PORT`                | `8080`       |
| `HTTP_READ_TIMEOUT`        | `5s`         |
| `HTTP_WRITE_TIMEOUT`       | `5s`         |
| `HTTP_IDLE_TIMEOUT`        | `60s`        |
| `HTTP_SHUTDOWN_TIMEOUT`    | `10s`        |
| `HTTP_READ_HEADER_TIMEOUT` | `5s`         |
| `HTTP_MAX_HEADER_BYTES`    | `1048576`    |

### Logger

| Переменная           | По умолчанию                  |
|----------------------|-------------------------------|
| `LOGGER_LEVEL`       | `info`                        |
| `LOGGER_FILENAME`    | `./logs/delayed-notifier.log` |
| `LOGGER_MAX_SIZE`    | `100`                         |
| `LOGGER_MAX_BACKUPS` | `3`                           |
| `LOGGER_MAX_AGE`     | `28`                          |


---

## API

Полная документация доступна в Swagger UI: `http://localhost:8080/swagger/index.html`

### `POST /users` — Регистрация пользователя

Регистрирует нового пользователя. После регистрации можно сгенерировать токен для привязки Telegram.

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Ivan Ivanov",
    "email": "ivan@example.com"
  }'
```

**Ответ `201 Created`:**
```json
{
  "user_id": "019dfc49-c0e1-7c10-ac4d-857493938405",
  "message": "Registered via Email"
}
```

---

### `POST /users/:user_id/link-token` — Генерация ссылки для Telegram 

Создает одноразовую ссылку для привязки Telegram-аккаунта к зарегистрированному пользователю. Ссылка действует 1 час.

```bash
curl -X POST http://localhost:8080/users/019dfc49-c0e1-7c10-ac4d-857493938405/link-token
```

**Ответ `200 OK`:**
```json
{
  "token": "a1b2c3d4...",
  "link": "https://t.me/MyBot?start=a1b2c3d4...",
  "message": "Click the link in Telegram to link your account",
  "expires_in": "1 hour"
}
```

---

### `POST /notify` — Создать уведомление

Создает отложенное уведомление для зарегистрированного пользователя. Канал (Email/Telegram) выбирается автоматически на основе данных пользователя.

```bash
curl -X POST http://localhost:8080/notify \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "019dfc49-c0e1-7c10-ac4d-857493938405",
    "channel": "email",
    "payload": "Ваш заказ готов!",
    "scheduled_at": "2026-05-06T10:00:00Z"
  }'
```

**Поле `channel`:**
- `email` — отправка на Email пользователя (должен быть указан при регистрации).
- `telegram` — отправка в Telegram (пользователь должен быть привязан через токен или зарегистрирован через бота).

**Payload для email** поддерживает JSON с отдельной темой:

```json
{
  "user_id": "...",
  "channel": "email",
  "payload": "{\"subject\": \"Напоминание\", \"body\": \"<b>Ваш заказ готов!</b>\"}",
  "scheduled_at": "2026-05-06T10:00:00Z"
}
```

**Ответ `201 Created`:**
```json
{
  "id": "019ce71c-4088-76a2-adca-a77577abcdef",
  "message": "Notification scheduled successfully"
}
```

---

### `GET /notify/{id}` — Статус уведомления

```bash
curl http://localhost:8080/notify/019ce71c-4088-76a2-adca-a77577abcdef
```

**Ответ `200 OK`:**
```json
{
  "id": "019ce71c-4088-76a2-adca-a77577abcdef",
  "user_id": "019dfc49-c0e1-7c10-ac4d-857493938405",
  "channel": "email",
  "status": "sent",
  "payload": "Ваш заказ готов!",
  "scheduled_at": "2026-05-06T10:00:00Z",
  "sent_at": "2026-05-06T10:00:03Z",
  "retry_count": 0,
  "created_at": "2026-05-06T09:00:00Z"
}
```

**Статусы:**

| Статус       | Описание                                |
|--------------|-----------------------------------------|
| `waiting`    | Ожидает наступления времени отправки    |
| `in_process` | Опубликовано в RabbitMQ, воркер обрабатывает |
| `sent`       | Успешно доставлено                      |
| `failed`     | Ошибка, будет повторная попытка         |
| `cancelled`  | Отменено пользователем                  |

---

### `DELETE /notify/{id}` — Отменить уведомление

```bash
curl -X DELETE http://localhost:8080/notify/019ce71c-4088-76a2-adca-a77577abcdef
```

Отмена возможна только для уведомлений в статусе `waiting`.

---

### `GET /health` — Проверка работоспособности

```bash
curl http://localhost:8080/health
# {"status":"ok","time":"2026-05-06T10:00:00Z"}
```

---

## Telegram: Привязка аккаунта

Чтобы получать уведомления в Telegram, пользователь должен быть зарегистрирован в системе. Есть два способа:

### Способ 1: Регистрация через бота

- Пользователь пишет боту `/start`.
- Бот создает нового пользователя в БД, привязывая его `chat_id`.
- Уведомления можно слать на канал `telegram` для этого `user_id`.

### Способ 2: Привязка к существующему Email-аккаунту (Deep Linking)

- Пользователь регистрируется на сайте через `POST /users` (получает `user_id`).
- Сайт генерирует ссылку через `POST /users/:user_id/link-token`.
- Пользователь переходит по ссылке `t.me/Bot?start=TOKEN` и нажимает "Start".
- Бот распознает токен, находит `user_id` и привязывает к нему текущий `chat_id`.
- Теперь уведомления на канал `telegram` будут приходить этому пользователю.

---

## Настройка Email (Gmail)

Gmail требует **App Password** — обычный пароль не подходит.

- Включите двухфакторную аутентификацию в аккаунте Google
- Перейдите: [Google Account → Security → App Passwords](https://myaccount.google.com/apppasswords)
- Создайте пароль для приложения "Mail"
- Заполните `.env`:

```env
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your@gmail.com
SMTP_PASSWORD=xxxx-xxxx-xxxx-xxxx   # App Password из шага 3
SMTP_FROM=your@gmail.com
```

---

## Разработка

```bash
# Установить инструменты разработки
make install-tools

# Запустить юнит-тесты
make test

# Запустить интеграционные тесты (требует Docker)
make integration-test

# Запустить все тесты
make test-all

# Линтер
make lint

# Форматирование кода
make format

# Сгенерировать моки
make mocks

# Сгенерировать Swagger-документацию
make swagger

# Собрать бинарник (linux/amd64)
make build

# Проверить уязвимости в зависимостях
make deps-audit
```

---

## Структура проекта

```
delayed-notifier/
├── cmd/
│   └── delayed-notifier/
│       └── main.go              # Точка входа
├── configs/                     # Конфиги для окружений (.env файлы)
├── docs/                        # Swagger (генерируется make swagger)
├── internal/
│   ├── app/
│   │   └── app.go               # Инициализация и запуск всех компонентов
│   ├── config/
│   │   └── config.go            # Конфигурация через env-переменные
│   ├── entity/                  # Доменные типы: Notification, User, Status, Channel
│   ├── repository/              # Реализации репозиториев (PostgreSQL, Redis)
│   ├── service/                 # Бизнес-логика: Register, Create, GetStatus, Cancel, ProcessQueue
│   └── transport/
│       ├── http/                # HTTP handlers, middleware, роутер (Gin)
│       └── sender/              # EmailSender, TelegramSender, MultiSender
│           └── mock/
├── migrations/                  # SQL-миграции (up/down)
├── web/
│   └── index.html               # Веб-интерфейс
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── .env.example
```

---

## Схема БД

```sql
-- Пользователи
CREATE TABLE users (
    id           UUID        PRIMARY KEY,
    name         TEXT        NOT NULL,
    email        TEXT        UNIQUE,            -- Может быть NULL, если регистрация через ТГ
    telegram_id  BIGINT      UNIQUE,            -- Может быть NULL, если регистрация через Сайт
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Индексы для быстрого определения email/telegram
CREATE INDEX idx_users_email ON users (email) WHERE email IS NOT NULL;
CREATE INDEX idx_users_telegram_id ON users (telegram_id) WHERE telegram_id IS NOT NULL;

-- Токены для привязки Telegram (одноразовые)
CREATE TABLE user_link_tokens (
    token      TEXT        PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Уведомления
CREATE TABLE notifications (
    id           UUID        PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel      TEXT        NOT NULL CHECK (channel IN ('telegram', 'email')),
    payload      TEXT        NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    sent_at      TIMESTAMPTZ,
    status       TEXT        NOT NULL DEFAULT 'waiting'
                             CHECK (status IN ('waiting', 'in_process', 'sent', 'failed', 'cancelled')),
    retry_count  INT         NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Индекс для быстрого выбора уведомлений к отправке
CREATE INDEX idx_notifications_waiting_scheduled
    ON notifications (scheduled_at ASC, id ASC)
    WHERE status = 'waiting';
```