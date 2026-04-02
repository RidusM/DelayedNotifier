# delayed-notifier

Сервис отложенных уведомлений. Принимает запросы на создание уведомлений с указанием времени отправки, хранит их в PostgreSQL, публикует в RabbitMQ в нужное время и отправляет через Email (SMTP) или Telegram (Bot API). При ошибке повторяет попытку с экспоненциальной задержкой.

---

## Содержание

- [Возможности](#возможности)
- [Архитектура](#архитектура)
- [Быстрый старт](#быстрый-старт)
- [Конфигурация](#конфигурация)
- [API](#api)
- [Telegram: подписка пользователей](#telegram-подписка-пользователей)
- [Настройка Email (Gmail)](#настройка-email-gmail)
- [Разработка](#разработка)
- [Структура проекта](#структура-проекта)

---

## Возможности

- **REST API** — создание, получение статуса и отмена уведомлений
- **Два канала доставки** — Email (SMTP) и Telegram (Bot API)
- **Фоновая обработка** — периодический опрос БД, публикация в RabbitMQ
- **Retry с экспоненциальной задержкой** — до `SERVICE_MAX_RETRIES` попыток
- **Redis-кэш** — быстрый ответ на `GET /notify/{id}` без похода в БД
- **Swagger UI** — `/swagger/index.html`
- **Веб-интерфейс** — `/` для создания и мониторинга уведомлений без curl

---

## Архитектура

```
HTTP-запрос
    │
    ▼
NotifyHandler (transport/http)
    │
    ▼
NotifyService (service)
    │
    ├── NotifyRepository    (PostgreSQL) — хранение уведомлений
    ├── CacheRepository     (Redis)      — кэш статусов
    ├── TelegramRepository  (PostgreSQL) — chat_id подписчиков бота
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

| Переменная    | По умолчанию       | Описание                                  |
|---------------|--------------------|-------------------------------------------|
| `APP_NAME`    | `delayed-notifier` | Название сервиса (используется в логах)   |
| `APP_VERSION` | `1.0.0`            | Версия                                    |
| `ENV`         | `local`            | Окружение: `local`, `dev`, `staging`, `prod` |

### Сервис (логика retry)

| Переменная              | По умолчанию | Описание                              |
|-------------------------|--------------|---------------------------------------|
| `SERVICE_QUERY_LIMIT`   | `10`         | Уведомлений за один цикл обработки    |
| `SERVICE_RETRY_DELAY`   | `5m`         | Базовая задержка перед повтором       |
| `SERVICE_MAX_RETRIES`   | `3`          | Максимальное число попыток            |

### База данных

| Переменная           | По умолчанию                                                    |
|----------------------|-----------------------------------------------------------------|
| `DB_DSN`             | `postgres://postgres:postgres@db:5432/notify_db?sslmode=disable` |
| `DB_POOL_MAX`        | `20`                                                            |
| `DB_CONN_ATTEMPTS`   | `5`                                                             |

### Redis

| Переменная      | По умолчанию     |
|-----------------|------------------|
| `CACHE_ADDR`    | `redis:6379`     |
| `CACHE_PASSWORD`| _(пусто)_        |

### RabbitMQ

| Переменная              | По умолчанию                    |
|-------------------------|---------------------------------|
| `RABBIT_URL`            | `amqp://guest:guest@rabbitmq:5672/` |
| `RABBIT_EXCHANGE`       | `notifications`                 |
| `RABBIT_WORKERS`        | `2`                             |
| `RABBIT_PREFETCH`       | `10`                            |
| `RABBIT_QUEUE_PROCESS_INTERVAL` | `5s`                  |

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

### HTTP-сервер

| Переменная              | По умолчанию |
|-------------------------|--------------|
| `HTTP_HOST`             | `0.0.0.0`    |
| `HTTP_PORT`             | `8080`       |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s`        |

---

## API

Полная документация доступна в Swagger UI: `http://localhost:8080/swagger/index.html`

### `POST /notify` — создать уведомление

```bash
curl -X POST http://localhost:8080/notify \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "email",
    "recipient": "user@example.com",
    "payload": "Ваш заказ готов!",
    "scheduled_at": "2026-04-01T10:00:00Z"
  }'
```

**Ответ `201 Created`:**
```json
{
  "id": "019ce71c-4088-76a2-adca-a77577abcdef",
  "channel": "email",
  "recipient": "user@example.com",
  "payload": "Ваш заказ готов!",
  "scheduled_at": "2026-04-01T10:00:00Z",
  "message": "Notification created successfully"
}
```

**Поле `recipient`:**
- `email` — email-адрес: `user@example.com`
- `telegram` — числовой `chat_id` пользователя: `123456789` (получается командой `/start` боту)

**Payload для email** поддерживает JSON с отдельной темой:
```json
{
  "channel": "email",
  "recipient": "user@example.com",
  "payload": "{\"subject\": \"Напоминание\", \"body\": \"<b>Ваш заказ готов!</b>\"}",
  "scheduled_at": "2026-04-01T10:00:00Z"
}
```
Если `payload` — не JSON, он используется как тело письма, тема будет `"Notification"`.

---

### `GET /notify/{id}` — статус уведомления

```bash
curl http://localhost:8080/notify/019ce71c-4088-76a2-adca-a77577abcdef
```

**Ответ `200 OK`:**
```json
{
  "id": "019ce71c-4088-76a2-adca-a77577abcdef",
  "channel": "email",
  "status": "sent",
  "payload": "Ваш заказ готов!",
  "scheduled_at": "2026-04-01T10:00:00Z",
  "sent_at": "2026-04-01T10:00:03Z",
  "retry_count": 0,
  "created_at": "2026-03-30T12:00:00Z"
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

### `DELETE /notify/{id}` — отменить уведомление

```bash
curl -X DELETE http://localhost:8080/notify/019ce71c-4088-76a2-adca-a77577abcdef
```

Отмена возможна только для уведомлений в статусе `waiting`. Попытка отменить `sent` или `cancelled` вернёт `409 Conflict`.

---

### `GET /health` — проверка работоспособности

```bash
curl http://localhost:8080/health
# {"status":"ok","time":"2026-04-01T10:00:00Z"}
```

---

## Telegram: подписка пользователей

Чтобы отправить уведомление в Telegram, нужен числовой `chat_id` пользователя.

1. Создайте бота через [@BotFather](https://t.me/botfather), получите токен
2. Установите `TG_TOKEN=<токен>` в `.env`
3. Пользователь пишет боту команду `/start`
4. Бот сохраняет `chat_id` в базу
5. Используйте этот `chat_id` как `recipient` при создании уведомления

```bash
curl -X POST http://localhost:8080/notify \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "telegram",
    "recipient": "123456789",
    "payload": "Привет из DelayedNotifier!",
    "scheduled_at": "2026-04-01T10:00:00Z"
  }'
```

---

## Настройка Email (Gmail)

Gmail требует **App Password** — обычный пароль не подходит.

1. Включите двухфакторную аутентификацию в аккаунте Google
2. Перейдите: [Google Account → Security → App Passwords](https://myaccount.google.com/apppasswords)
3. Создайте пароль для приложения "Mail"
4. Заполните `.env`:

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
│   ├── entity/                  # Доменные типы: Notification, Status, Channel, ошибки
│   ├── repository/              # Реализации репозиториев (PostgreSQL, Redis)
│   │   └── mock/                # Сгенерированные моки для тестов
│   ├── service/                 # Бизнес-логика: Create, GetStatus, Cancel, ProcessQueue
│   └── transport/
│       ├── http/                # HTTP handlers, middleware, роутер (Gin)
│       └── sender/              # EmailSender, TelegramSender, MultiSender
│           └── mock/
├── migrations/                  # SQL-миграции (up/down)
├── tests/
│   └── integration/             # Интеграционные тесты + docker-compose для них
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
-- Уведомления
CREATE TABLE notifications (
    id                   UUID        PRIMARY KEY,
    channel              TEXT        NOT NULL CHECK (channel IN ('telegram', 'email')),
    payload              TEXT        NOT NULL,
    recipient_identifier TEXT        NOT NULL,
    scheduled_at         TIMESTAMPTZ NOT NULL,
    sent_at              TIMESTAMPTZ,
    status               TEXT        NOT NULL DEFAULT 'waiting'
                                     CHECK (status IN ('waiting', 'in_process', 'sent', 'failed', 'cancelled')),
    retry_count          INT         NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error           TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Индекс для быстрого выбора уведомлений к отправке
CREATE INDEX idx_notifications_waiting_scheduled
    ON notifications (scheduled_at, status)
    WHERE status = 'waiting';

-- Подписчики Telegram-бота (chat_id по username)
CREATE TABLE telegram_subscribers (
    username   TEXT        PRIMARY KEY,
    chat_id    BIGINT      NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```