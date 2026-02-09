CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('telegram', 'email')),
    payload TEXT NOT NULL,
    recipient_identifier VARCHAR(255) NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    sent_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL CHECK (status IN ('waiting', 'in_process', 'sent', 'failed', 'cancelled')),
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_status_scheduled
    ON notifications(status, scheduled_at)
    WHERE status = 'waiting';

CREATE INDEX idx_notifications_user_id
    ON notifications(user_id);

CREATE INDEX idx_notifications_created_at
    ON notifications(created_at);

CREATE INDEX idx_notifications_channel
    ON notifications(channel);

CREATE INDEX idx_notifications_waiting_covering
    ON notifications(status, scheduled_at)
    INCLUDE (id, user_id, channel, payload, recipient_identifier, retry_count)
    WHERE status = 'waiting';
