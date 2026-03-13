package repository

import (
	"context"
	"errors"
	"fmt"

	"delayednotifier/internal/entity"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
)

type TelegramRepository struct {
	db *pgxdriver.Postgres
}

func NewTelegramRepository(db *pgxdriver.Postgres) *TelegramRepository {
	return &TelegramRepository{db: db}
}

func (r *TelegramRepository) SaveSubscriber(ctx context.Context, username string, chatID int64) error {
	const op = "repository.telegram.SaveSubscriber"

	sql, args, err := r.db.Insert("telegram_subscribers").
		Columns("username", "chat_id").
		Values(username, chatID).
		Suffix("ON CONFLICT (username) DO UPDATE SET chat_id = EXCLUDED.chat_id").
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: build query: %w", op, err)
	}

	if _, err = r.db.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("%s: exec: %w", op, err)
	}
	return nil
}

func (r *TelegramRepository) GetChatID(ctx context.Context, username string) (int64, error) {
	const op = "repository.telegram.GetChatID"

	sql, args, err := r.db.Select("chat_id").
		From("telegram_subscribers").
		Where(squirrel.Eq{"username": username}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("%s: build query: %w", op, err)
	}

	var chatID int64
	err = r.db.QueryRow(ctx, sql, args...).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return chatID, nil
}
