CREATE TABLE IF NOT EXISTS notifications (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL,
    channel      VARCHAR(20) NOT NULL,
    payload      TEXT NOT NULL,
    scheduled_at TIMESTAMP WITH TIME ZONE NOT NULL,
    sent_at      TIMESTAMP WITH TIME ZONE,
    status       SMALLINT NOT NULL DEFAULT 0,
    retry_count  INT NOT NULL DEFAULT 0,
    last_error   TEXT DEFAULT '',
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_processing
ON notifications (status, scheduled_at)
WHERE status = 'waiting';

CREATE INDEX idx_notifications_user_id ON notifications (user_id);

CREATE TABLE user_telegram_links (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    telegram_chat_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE user_email_links (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT NOW()
);
