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
	_cacheKeyPrefix = "notify:"
	_defaultTTL     = 5 * time.Minute
)

type CacheRepository struct {
	rdb *rediswbf.Client
}

func NewCacheRepository(rdb *rediswbf.Client) *CacheRepository {
	return &CacheRepository{rdb: rdb}
}

func (r *CacheRepository) GetCacheKey(id uuid.UUID) string {
	return _cacheKeyPrefix + id.String()
}

func (r *CacheRepository) GetFromCache(ctx context.Context, key string) (*entity.Notification, error) {
	const op = "repository.cache.GetFromCache"

	cached, err := r.rdb.Get(ctx, key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, entity.ErrDataNotFound
		}
		return nil, fmt.Errorf("%s: redis get: %w", op, err)
	}
	if cached == "" {
		return nil, entity.ErrDataNotFound
	}

	var notification entity.Notification
	if unmarshErr := json.Unmarshal([]byte(cached), &notification); unmarshErr != nil {
		return nil, fmt.Errorf("%s: unmarshal: %w", op, unmarshErr)
	}

	return &notification, nil
}

func (r *CacheRepository) SaveToCache(ctx context.Context, key string, notification *entity.Notification) error {
	const op = "repository.cache.SaveToCache"

	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", op, err)
	}

	if setErr := r.rdb.SetWithExpiration(ctx, key, data, _defaultTTL); setErr != nil {
		return fmt.Errorf("%s: redis set: %w", op, setErr)
	}

	return nil
}

func (r *CacheRepository) InvalidateCache(ctx context.Context, id uuid.UUID) error {
	const op = "repository.cache.InvalidateCache"

	if err := r.rdb.Del(ctx, r.GetCacheKey(id)); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("%s: redis del: %w", op, err)
	}

	return nil
}
