CREATE TABLE telegram_subscribers (
    username   TEXT        PRIMARY KEY,
    chat_id    BIGINT      NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);