package repository

import (
	"context"
	"delayednotifier/internal/entity"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
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

func (r *NotifyRepository) scanNotification(scanner rowScanner) (*entity.Notification, error) {
	var n entity.Notification
	var sentAt pgtype.Timestamptz
	var lastError pgtype.Text
	var retryCount pgtype.Int4
	var recipientIdentifier pgtype.Text // <-- НОВОЕ

	err := scanner.Scan(
		&n.ID,
		&n.UserID,
		&n.Channel,
		&n.Payload,
		&n.ScheduledAt,
		&sentAt,
		&n.Status,
		&retryCount,
		&lastError,
		&n.CreatedAt,
		&recipientIdentifier, // <-- НОВОЕ
	)
	if err != nil {
		return nil, err
	}

	if sentAt.Valid {
		n.SentAt = &sentAt.Time
	}
	if lastError.Valid {
		n.LastError = lastError.String
	}
	if retryCount.Valid {
		n.RetryCount = int(retryCount.Int32)
	}
	if recipientIdentifier.Valid { // <-- НОВОЕ
		n.RecipientIdentifier = recipientIdentifier.String
	}

	return &n, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *NotifyRepository) Create(ctx context.Context, qe pgxdriver.QueryExecuter, notify entity.Notification) (*entity.Notification, error) {
	const op = "repository.NotifyRepository.Create"

	var err error
	notify.ID, err = uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("%s: new v7 uuid: %w", op, err)
	}

	if notify.CreatedAt.IsZero() {
		notify.CreatedAt = time.Now().UTC()
	}

	sql, args, err := r.db.Insert("notifications").
		Columns("id", "user_id", "channel", "payload", "scheduled_at", "status", "created_at", "recipient_identifier").                                    // <-- Добавлено recipient_identifier
		Values(notify.ID, notify.UserID, notify.Channel, notify.Payload, notify.ScheduledAt, notify.Status, notify.CreatedAt, notify.RecipientIdentifier). // <-- Добавлено notify.RecipientIdentifier
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: building query: %w", op, err)
	}

	_, err = qe.Exec(ctx, sql, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%s: %w", op, entity.ErrConflictingData)
		}
		return nil, fmt.Errorf("%s: exec: %w", op, err)
	}

	return &notify, nil
}

func (r *NotifyRepository) GetByID(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID) (*entity.Notification, error) {
	const op = "repository.NotifyRepository.GetByID"

	sql, args, err := r.db.Select(notificationColumns).
		From("notifications").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: building query: %w", op, err)
	}

	n, err := r.scanNotification(qe.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return nil, fmt.Errorf("%s: scan row: %w", op, err)
	}

	return n, nil
}

func (r *NotifyRepository) GetForProcess(ctx context.Context, qe pgxdriver.QueryExecuter, limit uint64) ([]entity.Notification, error) {
	const op = "repository.NotifyRepository.GetForProcess"

	sql, args, err := r.db.Select(notificationColumns).
		From("notifications").
		Where(squirrel.Eq{"status": entity.StatusWaiting}).
		Where("scheduled_at <= NOW()").
		OrderBy("scheduled_at ASC").
		Limit(limit).
		Suffix("FOR UPDATE SKIP LOCKED").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: building sql: %w", op, err)
	}

	rows, err := qe.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	var results []entity.Notification
	for rows.Next() {
		n, err := r.scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: scan row: %w", op, err)
		}
		results = append(results, *n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: rows error: %w", op, err)
	}

	return results, nil
}

func (r *NotifyRepository) UpdateStatus(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID, status entity.Status, lastErr *string) error {
	const op = "repository.NotifyRepository.UpdateStatus"

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
		update = update.Set("sent_at", squirrel.Expr("NOW()"))
	case entity.StatusFailed:
		update = update.Set("retry_count", squirrel.Expr("COALESCE(retry_count, 0) + 1"))
	}

	sql, args, err := update.ToSql()
	if err != nil {
		return fmt.Errorf("%s: building query: %w", op, err)
	}

	res, err := qe.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: exec: %w", op, err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
	}

	return nil
}

func (r *NotifyRepository) RescheduleNotification(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID, newScheduledAt time.Time) error {
	const op = "repository.NotifyRepository.RescheduleNotification"

	sql, args, err := r.db.Update("notifications").
		Set("scheduled_at", newScheduledAt).
		Set("status", entity.StatusWaiting).
		Set("last_error", nil).
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: building query: %w", op, err)
	}

	res, err := qe.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: exec: %w", op, err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
	}

	return nil
}

func (r *NotifyRepository) GetTelegramChatIDByUserID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID) (int64, error) {
	const op = "repository.NotifyRepository.GetTelegramChatIDByUserID"

	var chatID int64
	sql, args, err := r.db.Select("telegram_chat_id").
		From("user_telegram_links").
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("%s: build query: %w", op, err)
	}

	err = qe.QueryRow(ctx, sql, args...).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return 0, fmt.Errorf("%s: query: %w", op, err)
	}

	return chatID, nil
}

func (r *NotifyRepository) GetUserEmailByUserID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID) (string, error) {
	const op = "repository.NotifyRepository.GetUserEmailByUserID"

	var email string
	sql, args, err := r.db.Select("email").
		From("user_email_links u").
		Where(squirrel.Eq{"u.user_id": userID}).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("%s: build query: %w", op, err)
	}

	err = qe.QueryRow(ctx, sql, args...).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return "", fmt.Errorf("%s: query: %w", op, err)
	}

	return email, nil
}
