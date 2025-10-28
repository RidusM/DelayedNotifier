package transaction

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/ridusm/delayednotifier/pkg/logger"
	"github.com/ridusm/delayednotifier/pkg/metric"
)

const (
	_defaultMaxAttempts    = 3
	_defaultBaseRetryDelay = 10 * time.Millisecond
	_defaultMaxRetryDelay  = 100 * time.Millisecond

	_backoffMultiplier = 2
)

type Manager interface {
	ExecuteInTransaction(
		ctx context.Context,
		operation string,
		fn func(tx postgres.QueryExecuter) error,
	) error
}

type manager struct {
	pool    *postgres.Postgres
	log     logger.Logger
	metrics metric.Transaction

	maxAttempts    int
	baseRetryDelay time.Duration
	maxRetryDelay  time.Duration
}

func NewManager(
	pool *postgres.Postgres,
	log logger.Logger,
	metrics metric.Transaction,
	opts ...Option,
) (Manager, error) {
	tm := &manager{
		pool:    pool,
		log:     log,
		metrics: metrics,

		maxAttempts:    _defaultMaxAttempts,
		baseRetryDelay: _defaultBaseRetryDelay,
		maxRetryDelay:  _defaultMaxRetryDelay,
	}

	for _, opt := range opts {
		opt(tm)
	}
	if err := tm.validate(); err != nil {
		return nil, fmt.Errorf("storage.postgres.transaction.NewManager: %w", err)
	}

	return tm, nil
}

func (tm *manager) ExecuteInTransaction(
	ctx context.Context,
	operation string,
	fn func(tx postgres.QueryExecuter) error,
) error {
	const op = "storage.postgres.transaction.ExecuteInTransaction"

	return tm.withRetry(ctx, operation, func() error {
		tx, err := tm.pool.Pool.BeginTx(ctx, pgx.TxOptions{
			IsoLevel:   pgx.ReadCommitted,
			AccessMode: pgx.ReadWrite,
		})
		if err != nil {
			return fmt.Errorf("%s: begin tx: %w", op, err)
		}
		defer tm.safelyRollback(ctx, tx, operation)

		txExecuter := &postgres.TxQueryExecuter{Tx: tx}
		if err = fn(txExecuter); err != nil {
			handledErr := HandleError(operation, "execute", err)
			return fmt.Errorf("%s: with retry function: %w", op, handledErr)
		}

		return tx.Commit(ctx)
	}, _defaultMaxAttempts)
}

func (tm *manager) safelyRollback(ctx context.Context, tx pgx.Tx, operation string) {
	const op = "storage.postgres.transaction.safelyRollback"

	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		tm.log.LogAttrs(ctx, logger.ErrorLevel, "rollback failed",
			logger.String("operation", op),
			logger.String("transaction", operation),
			logger.Any("error", err),
		)
	}
}

func (tm *manager) withRetry(
	ctx context.Context,
	operation string,
	fn func() error,
	maxAttempts int,
) error {
	const op = "storage.postgres.transaction.withRetry"
	var lastErr error

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		tm.metrics.ObserveDuration(operation, duration)
	}()

	currentBackoff := _defaultBaseRetryDelay
	for i := range maxAttempts {
		//nolint:gosec
		jitter := time.Duration(
			rand.Int64N(int64(currentBackoff * _backoffMultiplier)),
		)
		if jitter > _defaultMaxRetryDelay {
			jitter = _defaultMaxRetryDelay
		}

		tm.log.LogAttrs(ctx, logger.InfoLevel, "retrying transaction",
			logger.String("operation", op),
			logger.String("transaction", operation),
			logger.Int("attempt", i+1),
			logger.Int("max_attempts", maxAttempts),
			logger.String("retry_after", jitter.String()),
			logger.Any("error", lastErr),
		)

		timer := time.NewTimer(jitter)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			tm.metrics.IncrementFailures(operation)
			return fmt.Errorf("%s: context canceled: %w", op, ctx.Err())
		}

		err := fn()
		if err == nil {
			return nil
		}

		if !isRetryableError(err) {
			tm.metrics.IncrementFailures(operation)
			return err
		}

		tm.metrics.IncrementRetries(operation)
		lastErr = err

		nextBackoff := currentBackoff * _backoffMultiplier
		if nextBackoff > _defaultMaxRetryDelay {
			nextBackoff = _defaultMaxRetryDelay
		}
		currentBackoff = nextBackoff
	}
	tm.metrics.IncrementFailures(operation)
	return fmt.Errorf(
		"%s: max attempts (%d) exceeded for %s: %w",
		op,
		maxAttempts,
		operation,
		lastErr,
	)
}

func isRetryableError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40P01", "40001", "08000", "08003", "08006", "08001", "08004", "08007", "08P01":
			return true
		}
	}

	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return false
	}

	if errors.Is(err, pgx.ErrTxClosed) {
		return true
	}

	return false
}
