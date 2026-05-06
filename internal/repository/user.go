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

const _userColumns = "id, name, email, telegram_id, created_at"

type UserRepository struct {
	db *pgxdriver.Postgres
}

func NewUserRepository(db *pgxdriver.Postgres) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	u entity.User,
) error {
	const op = "repository.user.Create"

	sql, args, err := r.db.Insert("users").
		Columns(_userColumns).
		Values(u.ID, u.Name, u.Email, u.TelegramID, u.CreatedAt).
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

func (r *UserRepository) GetByID(ctx context.Context,
	qe pgxdriver.QueryExecuter,
	id uuid.UUID,
) (*entity.User, error) {
	const op = "repository.user.GetByID"
	sql, args, err := r.db.Select(_userColumns).
		From("users").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var u entity.User
	err = execOrDB(qe, r.db).QueryRow(ctx, sql, args...).Scan(
		&u.ID,
		&u.Name,
		&u.Email,
		&u.TelegramID,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &u, nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context,
	qe pgxdriver.QueryExecuter,
	chatID *int64,
) (*entity.User, error) {
	const op = "repository.user.GetByTelegramID"
	sql, args, err := r.db.Select(_userColumns).
		From("users").
		Where(squirrel.Eq{"telegram_id": chatID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var u entity.User
	err = execOrDB(qe, r.db).QueryRow(ctx, sql, args...).Scan(
		&u.ID,
		&u.Name,
		&u.Email,
		&u.TelegramID,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &u, nil
}

func (r *UserRepository) UpdateTelegramID(ctx context.Context,
	qe pgxdriver.QueryExecuter,
	userID uuid.UUID,
	chatID *int64,
) error {
	const op = "repository.user.UpdateTelegramID"
	sql, args, err := r.db.Update("users").
		Set("telegram_id", chatID).
		Where(squirrel.Eq{"id": userID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	res, err := execOrDB(qe, r.db).Exec(ctx, sql, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%s: %w", op, entity.ErrConflictingData)
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	if res.RowsAffected() == 0 {
		return entity.ErrDataNotFound
	}
	return nil
}

func (r *UserRepository) CreateLinkToken(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	userID uuid.UUID,
	token string,
	expiresAt time.Time,
) error {
	const op = "repository.user.CreateLinkToken"

	sql, args, err := r.db.Insert("user_link_tokens").
		Columns("token", "user_id", "expires_at").
		Values(token, userID, expiresAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = execOrDB(qe, r.db).Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (r *UserRepository) GetUserByLinkToken(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	token string,
) (uuid.UUID, error) {
	const op = "repository.user.GetUserByLinkToken"

	sqlSelect, argsSelect, err := r.db.Select("user_id", "expires_at").
		From("user_link_tokens").
		Where(squirrel.Eq{"token": token}).
		ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s: build select sql: %w", op, err)
	}

	var userID uuid.UUID
	var expiresAt time.Time

	err = execOrDB(qe, r.db).QueryRow(ctx, sqlSelect, argsSelect...).Scan(&userID, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("%s: %w", op, entity.ErrDataNotFound)
		}
		return uuid.Nil, fmt.Errorf("%s: query token: %w", op, err)
	}

	if time.Now().UTC().After(expiresAt) {
		return uuid.Nil, fmt.Errorf("%s: %w", op, entity.ErrInvalidData)
	}

	return userID, nil
}

func (r *UserRepository) DeleteLinkToken(
	ctx context.Context,
	qe pgxdriver.QueryExecuter,
	token string,
) error {
	const op = "repository.user.DeleteLinkToken"
	
	sql, args, err := r.db.Delete("user_link_tokens").
		Where(squirrel.Eq{"token": token}).
		ToSql()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	
	_, err = execOrDB(qe, r.db).Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}