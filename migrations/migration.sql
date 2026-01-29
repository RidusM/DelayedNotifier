-- migrations/001_initial_schema.sql
-- Создание основной таблицы уведомлений

CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('telegram', 'email', 'sms', 'push')),
    payload TEXT NOT NULL,
    recipient_identifier VARCHAR(255) NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    sent_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL CHECK (status IN ('waiting', 'in_process', 'sent', 'failed', 'cancelled')),
    retry_count SERIAL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_status_scheduled
    ON notifications(status, scheduled_at)
    WHERE status = 'waiting';

CREATE INDEX idx_notifications_user_id
    ON notifications(user_id);

CREATE INDEX idx_notifications_created_status
    ON notifications(created_at, status);

CREATE INDEX idx_notifications_channel
    ON notifications(channel);

COMMENT ON TABLE notifications IS 'Таблица отложенных уведомлений';
COMMENT ON COLUMN notifications.id IS 'Уникальный идентификатор уведомления (UUID v7)';
COMMENT ON COLUMN notifications.user_id IS 'ID пользователя-получателя';
COMMENT ON COLUMN notifications.channel IS 'Канал отправки: telegram, email, sms, push';
COMMENT ON COLUMN notifications.payload IS 'Содержимое уведомления';
COMMENT ON COLUMN notifications.recipient_identifier IS 'Email адрес или Telegram chat_id';
COMMENT ON COLUMN notifications.scheduled_at IS 'Запланированное время отправки';
COMMENT ON COLUMN notifications.sent_at IS 'Фактическое время отправки';
COMMENT ON COLUMN notifications.status IS 'Статус: waiting, in_process, sent, failed, cancelled';
COMMENT ON COLUMN notifications.retry_count IS 'Количество попыток повторной отправки';
COMMENT ON COLUMN notifications.last_error IS 'Последняя ошибка при отправке';
COMMENT ON COLUMN notifications.created_at IS 'Время создания записи';

CREATE TABLE IF NOT EXISTS user_telegram_links (
    user_id UUID PRIMARY KEY,
    telegram_chat_id BIGINT NOT NULL UNIQUE,
    telegram_username VARCHAR(255),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_telegram_links_chat_id
    ON user_telegram_links(telegram_chat_id);

COMMENT ON TABLE user_telegram_links IS 'Связь пользователей с Telegram аккаунтами';
COMMENT ON COLUMN user_telegram_links.user_id IS 'ID пользователя в системе';
COMMENT ON COLUMN user_telegram_links.telegram_chat_id IS 'Telegram chat_id';
COMMENT ON COLUMN user_telegram_links.telegram_username IS 'Telegram username (опционально)';

CREATE TABLE IF NOT EXISTS user_email_links (
    user_id UUID PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    verified BOOLEAN DEFAULT FALSE,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_email_links_email
    ON user_email_links(email);

COMMENT ON TABLE user_email_links IS 'Связь пользователей с email адресами';
COMMENT ON COLUMN user_email_links.user_id IS 'ID пользователя в системе';
COMMENT ON COLUMN user_email_links.email IS 'Email адрес';
COMMENT ON COLUMN user_email_links.verified IS 'Флаг верификации email';

INSERT INTO user_telegram_links (user_id, telegram_chat_id, telegram_username)
VALUES
    ('550e8400-e29b-41d4-a716-446655440000', 123456789, 'testuser1'),
    ('550e8400-e29b-41d4-a716-446655440001', 987654321, 'testuser2')
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO user_email_links (user_id, email, verified)
VALUES
    ('550e8400-e29b-41d4-a716-446655440000', 'test1@example.com', true),
    ('550e8400-e29b-41d4-a716-446655440001', 'test2@example.com', true)
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO notifications (
    id,
    user_id,
    channel,
    payload,
    recipient_identifier,
    scheduled_at,
    status,
    created_at
)
VALUES (
    gen_random_uuid(),
    '550e8400-e29b-41d4-a716-446655440000',
    'telegram',
    'Это тестовое уведомление!',
    '123456789',
    NOW() + INTERVAL '1 hour',
    'waiting',
    NOW()
)
ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION cleanup_old_notifications(days_old INTEGER DEFAULT 30)
RETURNS TABLE(deleted_count BIGINT) AS $$
DECLARE
    count BIGINT;
BEGIN
    DELETE FROM notifications
    WHERE created_at < NOW() - (days_old || ' days')::INTERVAL
    AND status IN ('sent', 'failed', 'cancelled');

    GET DIAGNOSTICS count = ROW_COUNT;

    RETURN QUERY SELECT count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_old_notifications IS 'Удаляет старые обработанные уведомления';

CREATE OR REPLACE VIEW notification_statistics AS
SELECT
    channel,
    status,
    COUNT(*) as count,
    MIN(created_at) as first_created,
    MAX(created_at) as last_created,
    AVG(EXTRACT(EPOCH FROM (sent_at - scheduled_at))) as avg_delay_seconds,
    AVG(retry_count) as avg_retries
FROM notifications
GROUP BY channel, status;

COMMENT ON VIEW notification_statistics IS 'Статистика по уведомлениям';
