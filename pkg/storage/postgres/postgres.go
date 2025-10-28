package postgres

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"github.com/!masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ridusm/delayednotifier/pkg/logger"
)

const (
	_defaultMaxPoolSize    = 100
	_defaultConnAttempts   = 10
	_defaultBaseRetryDelay = 100 * time.Millisecond
	_defaultMaxRetryDelay  = 5 * time.Second

	_backoffMultiplier = 2
)

type Postgres struct {
	Builder squirrel.StatementBuilderType
	Pool    *pgxpool.Pool

	connAttempts   int
	baseRetryDelay time.Duration
	maxRetryDelay  time.Duration
	maxPoolSize    int32
}

func NewPostgres(config *config.Postgres, log logger.Logger, opts ...Option) (*Postgres, error) {
	const op = "storage.postgres.NewPostgres"

	hostPort := net.JoinHostPort(config.Host, config.Port)
	url := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		config.User,
		config.Password,
		hostPort,
		config.Name,
		config.SSLMode,
	)

	pg := &Postgres{
		connAttempts:   _defaultConnAttempts,
		baseRetryDelay: _defaultBaseRetryDelay,
		maxRetryDelay:  _defaultMaxRetryDelay,
		maxPoolSize:    _defaultMaxPoolSize,
	}

	for _, opt := range opts {
		opt(pg)
	}
	if err := pg.validate(); err != nil {
		return nil, fmt.Errorf("%s: validation: %w", op, err)
	}

	pg.Builder = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("%s: parse pool config: %w", op, err)
	}

	poolConfig.MaxConns = pg.maxPoolSize

	currentBackoff := pg.baseRetryDelay
	for attemptCount := 1; attemptCount <= pg.connAttempts; attemptCount++ {
		pg.Pool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err == nil {
			return pg, nil
		}
		//nolint:gosec
		jitter := time.Duration(
			rand.Int64N(int64(currentBackoff * _backoffMultiplier)),
		)
		if jitter > pg.maxRetryDelay {
			jitter = pg.maxRetryDelay
		}

		log.Infow("PostgreSQL connection attempt failed",
			"operation", op,
			"attempt", attemptCount,
			"retry_after", jitter.String(),
			"error", err,
		)

		time.Sleep(jitter)

		nextBackoff := currentBackoff * _backoffMultiplier
		if nextBackoff > pg.maxRetryDelay {
			nextBackoff = pg.maxRetryDelay
		}
		currentBackoff = nextBackoff
	}
	if err != nil {
		return nil, fmt.Errorf("%s: create new pool: %w", op, err)
	}

	return pg, nil
}

func (p *Postgres) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
}
