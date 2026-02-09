CREATE TABLE IF NOT EXISTS user_telegram_links (
    user_id UUID PRIMARY KEY,
    telegram_chat_id BIGINT NOT NULL UNIQUE,
    telegram_username VARCHAR(255),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_telegram_links_chat_id
    ON user_telegram_links(telegram_chat_id);
