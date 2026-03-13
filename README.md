# DelayedNotifier

Сервис отложенных уведомлений. Принимает запросы на создание уведомлений с указанием времени отправки, хранит их в PostgreSQL, публикует в RabbitMQ в нужное время и отправляет через Email или Telegram. При ошибке отправки повторяет попытку с экспоненциальной задержкой.

## Возможности

- REST API для создания, получения статуса и отмены уведомлений
- Поддержка каналов: **Email** (SMTP) и **Telegram** (Bot API)
- Фоновая обработка очереди с настраиваемым интервалом
- Повторные попытки с экспоненциальной задержкой (до `MAX_RETRIES` раз)
- Кэширование статусов уведомлений в Redis
- Swagger UI по адресу `/swagger/index.html`
- Простой веб-интерфейс по адресу `/`

## Архитектура

```
HTTP Request
    │
    ▼
NotifyHandler (transport/http)
    │
    ▼
NotifyService (service)
    │
    ├── NotifyRepository (PostgreSQL) — хранение уведомлений
    ├── CacheRepository (Redis)       — кэш статусов
    ├── TelegramRepository (PostgreSQL) — подписчики бота
    └── Publisher (RabbitMQ)          — публикация в очередь
           │
           ▼
    Consumer (RabbitMQ)
           │
           ▼
    MultiSender
    ├── TelegramSender
    └── EmailSender
```

**Жизненный цикл уведомления:**

```
waiting → in_process → sent
                    ↘ failed → waiting (retry с задержкой)
                             → failed (max retries достигнут)
cancelled (отменено до отправки)
```

## Требования

- Go 1.22+
- PostgreSQL 14+
- Redis 7+
- RabbitMQ 3.12+

## Быстрый старт

```bash
# Клонировать репозиторий
git clone https://github.com/your-org/delayed-notifier
cd delayed-notifier

# Запустить инфраструктуру
docker compose up -d postgres redis rabbitmq

# Применить миграции
make migrate

# Запустить сервис
make run
```

## Конфигурация

Все параметры задаются через переменные окружения.

### Приложение

| Переменная | По умолчанию | Описание |
|---|---|---|
| `APP_NAME` | `delayed-notifier` | Название сервиса |
| `APP_VERSION` | `1.0.0` | Версия |
| `ENV` | `local` | Окружение (`local`, `dev`, `staging`, `prod`) |

### Сервис

| Переменная | По умолчанию | Описание |
|---|---|---|
| `SERVICE_QUERY_LIMIT` | `10` | Количество уведомлений за один цикл обработки |
| `SERVICE_RETRY_DELAY` | `5m` | Базовая задержка перед повторной попыткой |
| `SERVICE_MAX_RETRIES` | `3` | Максимальное количество попыток |

### База данных

| Переменная | По умолчанию | Описание |
|---|---|---|
| `DB_DSN` | `postgres://user:pass@localhost:5432/delayed_notifier?sslmode=disable` | Строка подключения |
| `DB_POOL_MAX` | `20` | Максимальный размер пула соединений |
| `DB_CONN_ATTEMPTS` | `5` | Количество попыток подключения при старте |
| `DB_BASE_RETRY_DELAY` | `100ms` | Начальная задержка между попытками |
| `DB_MAX_RETRY_DELAY` | `5s` | Максимальная задержка между попытками |

### Redis

| Переменная | По умолчанию | Описание |
|---|---|---|
| `CACHE_ADDR` | `localhost:6379` | Адрес Redis |
| `CACHE_PASSWORD` | _(пусто)_ | Пароль |

### RabbitMQ

| Переменная | По умолчанию | Описание |
|---|---|---|
| `RABBIT_URL` | — | AMQP URL (обязательно) |
| `RABBIT_CONNECTION_NAME` | `delayed-notifier-publisher` | Имя соединения |
| `RABBIT_EXCHANGE` | `notifications` | Имя exchange |
| `RABBIT_CONTENT_TYPE` | `application/json` | Content-Type сообщений |
| `RABBIT_ATTEMPTS` | `3` | Попыток переподключения |
| `RABBIT_DELAY` | `1s` | Задержка между попытками |
| `RABBIT_BACKOFF` | `2.0` | Множитель backoff |

### Email (SMTP)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `SMTP_HOST` | `smtp.gmail.com` | SMTP сервер (пусто — sender отключён) |
| `SMTP_PORT` | `587` | Порт |
| `SMTP_USERNAME` | _(пусто)_ | Логин |
| `SMTP_PASSWORD` | _(пусто)_ | Пароль |
| `SMTP_FROM` | `noreply@example.com` | Адрес отправителя |

### Telegram

| Переменная | По умолчанию | Описание |
|---|---|---|
| `TG_TOKEN` | — | Токен бота (пусто — sender отключён) |

### HTTP сервер

| Переменная | По умолчанию | Описание |
|---|---|---|
| `HTTP_HOST` | `0.0.0.0` | Хост |
| `HTTP_PORT` | `8080` | Порт |
| `HTTP_READ_TIMEOUT` | `5s` | Таймаут чтения |
| `HTTP_WRITE_TIMEOUT` | `5s` | Таймаут записи |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | Таймаут graceful shutdown |

## API

### POST /notify — создать уведомление

```bash
curl -X POST http://localhost:8080/notify \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "email",
    "recipient": "user@example.com",
    "payload": "Ваш заказ готов!",
    "scheduled_at": "2026-03-14T10:00:00Z"
  }'
```

Для Telegram `recipient` — это `chat_id` пользователя (числовой ID). Чтобы получить `chat_id`, пользователь должен написать боту `/start`.

**Ответ `201 Created`:**
```json
{
  "id": "019ce71c-4088-76a2-adca-a77577...",
  "channel": "email",
  "recipient": "user@example.com",
  "payload": "Ваш заказ готов!",
  "scheduled_at": "2026-03-14T10:00:00Z",
  "message": "Notification created successfully"
}
```

### GET /notify/{id} — статус уведомления

```bash
curl http://localhost:8080/notify/019ce71c-4088-76a2-adca-a77577...
```

**Ответ `200 OK`:**
```json
{
  "id": "019ce71c-...",
  "channel": "email",
  "status": "sent",
  "payload": "Ваш заказ готов!",
  "scheduled_at": "2026-03-14T10:00:00Z",
  "sent_at": "2026-03-14T10:00:02Z",
  "retry_count": 0,
  "created_at": "2026-03-13T12:00:00Z"
}
```

**Статусы:**

| Статус | Описание |
|---|---|
| `waiting` | Ожидает отправки |
| `in_process` | Публикуется в очередь |
| `sent` | Успешно отправлено |
| `failed` | Ошибка (будет повторная попытка) |
| `cancelled` | Отменено |

### DELETE /notify/{id} — отменить уведомление

```bash
curl -X DELETE http://localhost:8080/notify/019ce71c-...
```

Отмена возможна только для уведомлений со статусом `waiting`. Уже отправленные (`sent`) или отменённые (`cancelled`) отменить нельзя — вернётся `409 Conflict`.

### GET /health — проверка работоспособности

```bash
curl http://localhost:8080/health
# {"status":"ok","time":"2026-03-13T12:00:00Z"}
```

## Telegram: подписка пользователей

Для получения уведомлений в личные сообщения пользователь должен написать боту `/start`. Бот сохранит его `chat_id` и свяжет с `username`.

После этого можно использовать числовой `chat_id` как `recipient` при создании уведомления.

## Payload для Email

Email поддерживает отдельные `subject` и `body` через JSON:

```json
{
  "channel": "email",
  "recipient": "user@example.com",
  "payload": "{\"subject\": \"Напоминание\", \"body\": \"<b>Ваш заказ готов!</b>\"}",
  "scheduled_at": "2026-03-14T10:00:00Z"
}
```

Если `payload` не является валидным JSON — он используется как тело письма, тема будет `"Notification"`. Тело поддерживает HTML.

## Схема базы данных

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

CREATE INDEX idx_notifications_status_scheduled
    ON notifications (status, scheduled_at)
    WHERE status = 'waiting';

-- Подписчики Telegram бота
CREATE TABLE telegram_subscribers (
    username   TEXT        PRIMARY KEY,  -- "@username"
    chat_id    BIGINT      NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Разработка

```bash
# Запустить тесты
make test

# Запустить тесты с покрытием
make test-cover

# Линтер
make lint

# Сгенерировать моки
make generate

# Сгенерировать Swagger документацию
make swagger
```

## Структура проекта

```
.
├── cmd/
│   └── main.go
├── internal/
│   ├── app/            # Инициализация и запуск приложения
│   ├── config/         # Конфигурация через env
│   ├── entity/         # Доменные типы и ошибки
│   ├── repository/     # Работа с PostgreSQL и Redis
│   │   └── mock/       # Моки для тестов
│   ├── service/        # Бизнес-логика
│   └── transport/
│       ├── http/       # HTTP handlers, middleware, роутер
│       └── sender/     # Email и Telegram отправщики
│           └── mock/
└── migrations/         # SQL миграции
```

## Соответствие заданию

| Требование | Статус |
|---|---|
| `POST /notify` — создание уведомления | ✅ |
| `GET /notify/{id}` — статус уведомления | ✅ |
| `DELETE /notify/{id}` — отмена уведомления | ✅ |
| Фоновая обработка очереди | ✅ |
| Отправка в указанное время | ✅ |
| Повторные попытки с экспоненциальной задержкой | ✅ |
| Redis кэширование | ✅ |
| Несколько каналов (Email, Telegram) | ✅ |
| Простой UI | ✅ `/` |