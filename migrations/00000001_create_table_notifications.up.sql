CREATE TABLE notifications (
    id                   UUID        PRIMARY KEY,
    channel              TEXT        NOT NULL CHECK (channel IN ('telegram', 'email')),
    payload              TEXT        NOT NULL,
    recipient_identifier TEXT        NOT NULL,
    scheduled_at         TIMESTAMPTZ NOT NULL,
    sent_at              TIMESTAMPTZ,
    status               TEXT        NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'in_process', 'sent', 'failed', 'cancelled')),
    retry_count          INT         NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error           TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_status_scheduled
    ON notifications (status, scheduled_at)
    WHERE status = 'waiting';