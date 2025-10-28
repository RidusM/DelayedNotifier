package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type QueryExecuter interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type TxQueryExecuter struct {
	Tx pgx.Tx
}

func (t *TxQueryExecuter) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := t.Tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.postgres.TxQueryExecuter.Query: %w", err)
	}
	return rows, nil
}

func (t *TxQueryExecuter) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return t.Tx.QueryRow(ctx, sql, args...)
}

func (t *TxQueryExecuter) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	commTag, err := t.Tx.Exec(ctx, sql, args...)
	if err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("storage.postgres.TxQueryExecuter.Exec: %w", err)
	}
	return commTag, nil
}
