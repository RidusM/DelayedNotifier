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
	_notificationColumns = "id, user_id, channel, payload, scheduled_at, sent_at, status, retry_count, last_error, created_at"
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
	n entity.Notification,
) error {
	const op = "repository.notify.Create"

	sql, args, err := r.db.Insert("notifications").
		Columns("id", "user_id", "channel", "payload", "scheduled_at", "status", "created_at").
		Values(n.ID, n.UserID, n.Channel, n.Payload, n.ScheduledAt, n.Status, n.CreatedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = execOrDB(qe, r.db).Exec(ctx, sql, args...)
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
	forUpdate bool,
) (*entity.Notification, error) {
	const op = "repository.notify.GetByID"

	query := r.db.Select(_notificationColumns).
		From("notifications").
		Where(squirrel.Eq{"id": id})

	if forUpdate {
		query = query.Suffix("FOR UPDATE")
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var n entity.Notification
	err = execOrDB(qe, r.db).QueryRow(ctx, sql, args...).Scan(
		&n.ID,
		&n.UserID,
		&n.Channel,
		&n.Payload,
		&n.ScheduledAt,
		&n.SentAt,
		&n.Status,
		&n.RetryCount,
		&n.LastError,
		&n.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &n, nil
}

func (r *NotifyRepository) GetForProcess(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	limit uint64,
) ([]entity.Notification, error) {
	const op = "repository.notify.GetForProcess"

	if qe == nil {
		return nil, fmt.Errorf("%s: QueryExecuter is required for FOR UPDATE SKIP LOCKED", op)
	}

	sql, args, err := r.db.Select(_notificationColumns).
		From("notifications").
		Where(squirrel.Eq{"status": entity.StatusWaiting}).
		Where(squirrel.LtOrEq{"scheduled_at": time.Now()}).
		OrderBy("scheduled_at ASC").
		Limit(limit).
		Suffix("FOR UPDATE SKIP LOCKED").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	rows, err := qe.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var notifies []entity.Notification
	for rows.Next() {
		var n entity.Notification
		if err = rows.Scan(
			&n.ID,
			&n.UserID,
			&n.Channel,
			&n.Payload,
			&n.ScheduledAt,
			&n.SentAt,
			&n.Status,
			&n.RetryCount,
			&n.LastError,
			&n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		notifies = append(notifies, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
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

	query := r.db.Update("notifications").
		Set("status", status).
		Set("last_error", lastErr).
		Where(squirrel.Eq{"id": id})

	switch status {
	case entity.StatusSent:
		query = query.Set("sent_at", time.Now())
	case entity.StatusFailed:
		query = query.Set("retry_count", squirrel.Expr("retry_count + 1"))
	case entity.StatusCancelled, entity.StatusInProcess, entity.StatusWaiting:
		// no fields to update
	default:
		return fmt.Errorf("%s: unknown status: %s", op, status)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	notify, err := execOrDB(qe, r.db).Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if notify.RowsAffected() == 0 {
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

	sql, args, err := r.db.Update("notifications").
		Set("scheduled_at", newScheduledAt).
		Set("status", entity.StatusWaiting).
		Set("last_error", nil).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	notify, err := execOrDB(qe, r.db).Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if notify.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
	}

	return nil
}
