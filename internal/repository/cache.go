// nolint:musttag
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"delayednotifier/internal/entity"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	rediswbf "github.com/wb-go/wbf/redis"
)

const (
	_failedNotificationTTL = 10 * time.Minute

	_cacheKeyPrefix = "notify:"
	_defaultTTL     = 5 * time.Minute
)

type CacheRepository struct {
	rdb *rediswbf.Client
}

func NewCacheRepository(rdb *rediswbf.Client) *CacheRepository {
	return &CacheRepository{rdb: rdb}
}

func (r *CacheRepository) cacheKey(id uuid.UUID) string {
	return _cacheKeyPrefix + id.String()
}

func (r *CacheRepository) Get(
	ctx context.Context,
	id uuid.UUID,
) (*entity.Notification, error) {
	const op = "repository.cache.Get"

	cached, err := r.rdb.Get(ctx, r.cacheKey(id))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, entity.ErrDataNotFound
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if cached == "" {
		return nil, entity.ErrDataNotFound
	}

	var notify entity.Notification
	if err = json.Unmarshal([]byte(cached), &notify); err != nil {
		return nil, fmt.Errorf("%s: unmarshal: %w", op, err)
	}

	return &notify, nil
}

func (r *CacheRepository) Save(
	ctx context.Context,
	n *entity.Notification,
) error {
	const op = "repository.cache.Save"

	ttl := r.ttlForStatus(n.Status)

	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", op, err)
	}

	if err = r.rdb.SetWithExpiration(ctx, r.cacheKey(n.ID), data, ttl); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (r *CacheRepository) Invalidate(
	ctx context.Context,
	id uuid.UUID,
) error {
	const op = "repository.cache.Invalidate"

	if err := r.rdb.Del(ctx, r.cacheKey(id)); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *CacheRepository) ttlForStatus(status entity.Status) time.Duration {
	switch status {
	case entity.StatusSent, entity.StatusCancelled:
		return 1 * time.Hour
	case entity.StatusFailed:
		return _failedNotificationTTL
	case entity.StatusWaiting, entity.StatusInProcess:
		return 1 * time.Minute
	default:
		return _defaultTTL
	}
}
