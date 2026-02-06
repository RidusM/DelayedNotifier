package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"delayednotifier/internal/entity"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
)

const (
	notificationColumns = "id, user_id, channel, payload, scheduled_at, sent_at, status, retry_count, last_error, created_at, recipient_identifier"
)

type NotifyRepository struct {
	db *pgxdriver.Postgres
}

func NewNotifyRepository(db *pgxdriver.Postgres) *NotifyRepository {
	return &NotifyRepository{db: db}
}

func (r *NotifyRepository) Create(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	notify entity.Notification,
) error {
	const op = "repository.notify.Create"

	executor := r.exec(qe)

	sql, args, err := r.db.Insert("notifications").
		Columns("id", "user_id", "channel", "payload", "scheduled_at", "status", "created_at", "recipient_identifier").
		Values(notify.ID, notify.UserID, notify.Channel, notify.Payload, notify.ScheduledAt, notify.Status, notify.CreatedAt, notify.RecipientIdentifier).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: insert query: %w", op, err)
	}

	_, err = executor.Exec(ctx, sql, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%s: %w", op, entity.ErrConflictingData)
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *NotifyRepository) GetByID(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	id uuid.UUID,
) (*entity.Notification, error) {
	const op = "repository.notify.GetByID"

	executor := r.exec(qe)

	sql, args, err := r.db.Select(notificationColumns).
		From("notifications").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: select query: %w", op, err)
	}

	result := &entity.Notification{}
	err = executor.QueryRow(ctx, sql, args...).Scan(
		&result.ID,
		&result.UserID,
		&result.Channel,
		&result.Payload,
		&result.ScheduledAt,
		&result.SentAt,
		&result.Status,
		&result.RetryCount,
		&result.LastError,
		&result.CreatedAt,
		&result.RecipientIdentifier,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return result, nil
}

func (r *NotifyRepository) GetForProcess(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	limit uint64,
) ([]entity.Notification, error) {
	const op = "repository.notify.GetForProcess"

	executor := r.exec(qe)

	if limit == 0 {
		return nil, fmt.Errorf("%s: limit must be > 0", op)
	}

	sql, args, err := r.db.Select(notificationColumns).
		From("notifications").
		Where(squirrel.Eq{"status": entity.StatusWaiting}).
		Where(squirrel.LtOrEq{"scheduled_at": time.Now().UTC()}).
		OrderBy("scheduled_at ASC").
		Limit(limit).
		Suffix("FOR UPDATE SKIP LOCKED").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: select query: %w", op, err)
	}

	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var notifies []entity.Notification
	for rows.Next() {
		var notify entity.Notification
		if err = rows.Scan(
			&notify.ID,
			&notify.UserID,
			&notify.Channel,
			&notify.Payload,
			&notify.ScheduledAt,
			&notify.SentAt,
			&notify.Status,
			&notify.RetryCount,
			&notify.LastError,
			&notify.CreatedAt,
			&notify.RecipientIdentifier,
		); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		notifies = append(notifies, notify)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: rows error: %w", op, err)
	}

	return notifies, nil
}

func (r *NotifyRepository) UpdateStatus(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	id uuid.UUID,
	status entity.Status,
	lastErr *string,
) error {
	const op = "repository.notify.UpdateStatus"

	executor := r.exec(qe)

	update := r.db.Update("notifications").
		Set("status", status).
		Where(squirrel.Eq{"id": id})

	if lastErr != nil {
		update = update.Set("last_error", *lastErr)
	} else {
		update = update.Set("last_error", nil)
	}

	switch status {
	case entity.StatusSent:
		update = update.Set("sent_at", time.Now().UTC())
	case entity.StatusFailed:
		update = update.
			Set("retry_count", squirrel.Expr("retry_count + 1"))
	case entity.StatusCancelled:
		update = update.Set("sent_at", nil)
	}

	sql, args, err := update.ToSql()
	if err != nil {
		return fmt.Errorf("%s: update query: %w", op, err)
	}

	res, err := executor.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
	}

	return nil
}

func (r *NotifyRepository) RescheduleNotification(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	id uuid.UUID,
	newScheduledAt time.Time,
) error {
	const op = "repository.notify.RescheduleNotification"

	executor := r.exec(qe)

	sql, args, err := r.db.Update("notifications").
		Set("scheduled_at", newScheduledAt).
		Set("status", entity.StatusWaiting).
		Set("last_error", nil).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: update query: %w", op, err)
	}

	res, err := executor.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
	}

	return nil
}

func (r *NotifyRepository) exec(qe pgxdriver.QueryExecuter) pgxdriver.QueryExecuter {
	if qe != nil {
		return qe
	}
	return r.db
}
