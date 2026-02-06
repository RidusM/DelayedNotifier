package repository

import (
	"context"
	"delayednotifier/internal/entity"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
)

type UserRepository struct {
	db *pgxdriver.Postgres
}

func NewUserRepository(db *pgxdriver.Postgres) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetTelegramChatID(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	userID uuid.UUID,
) (int64, error) {
	const op = "repository.user.GetTelegramChatID"

	if qe == nil {
		qe = r.db
	}

	var chatID int64
	sql, args, err := r.db.Select("telegram_chat_id").
		From("user_telegram_links").
		Where(squirrel.Eq{"user_id": userID}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("%s: select query: %w", op, err)
	}

	err = qe.QueryRow(ctx, sql, args...).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return chatID, nil
}

func (r *UserRepository) GetEmail(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	userID uuid.UUID,
) (string, error) {
	const op = "repository.user.GetEmail"

	if qe == nil {
		qe = r.db
	}

	var email string
	sql, args, err := r.db.Select("email").
		From("user_email_links u").
		Where(squirrel.Eq{"u.user_id": userID}).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("%s: select query: %w", op, err)
	}

	err = qe.QueryRow(ctx, sql, args...).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return email, nil
}
