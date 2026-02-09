CREATE OR REPLACE FUNCTION cleanup_old_notifications(days_old INTEGER DEFAULT 30)
RETURNS TABLE(deleted_count BIGINT) AS $$
DECLARE
    count BIGINT;
    lock_key BIGINT := 123456789;
BEGIN
    IF pg_try_advisory_lock(lock_key) THEN
        BEGIN
            DELETE FROM notifications
            WHERE created_at < NOW() - (days_old || ' days')::INTERVAL
            AND status IN ('sent', 'failed', 'cancelled');

            GET DIAGNOSTICS count = ROW_COUNT;
        EXCEPTION WHEN OTHERS THEN
            PERFORM pg_advisory_unlock(lock_key);
            RAISE;
        END;
        PERFORM pg_advisory_unlock(lock_key);
    ELSE
        count := 0;
    END IF;

    RETURN QUERY SELECT count;
END;
$$ LANGUAGE plpgsql;
