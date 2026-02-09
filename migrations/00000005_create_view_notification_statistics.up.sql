CREATE OR REPLACE VIEW notification_statistics AS
SELECT
    channel,
    status,
    COUNT(*) as count,
    MIN(created_at) as first_created,
    MAX(created_at) as last_created,
    AVG(
        CASE
            WHEN sent_at IS NOT NULL AND scheduled_at IS NOT NULL
            THEN EXTRACT(EPOCH FROM (sent_at - scheduled_at))
            ELSE NULL
        END
    ) as avg_delay_seconds,
    AVG(retry_count) as avg_retries
FROM notifications
GROUP BY channel, status;
