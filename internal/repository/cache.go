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
	ttl time.Duration
}

func NewCacheRepository(rdb *rediswbf.Client, ttl time.Duration) *CacheRepository {
	if ttl == 0 {
		ttl = _defaultTTL
	}
	return &CacheRepository{rdb: rdb, ttl: ttl}
}

func (s *CacheRepository) GetCacheKey(id uuid.UUID) string {
	return _cacheKeyPrefix + id.String()
}

func (s *CacheRepository) GetFromCache(ctx context.Context, key string) (*entity.Notification, error) {
	const op = "repository.cache.GetFromCache"

	cached, err := s.rdb.Get(ctx, key)
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

func (s *CacheRepository) SaveToCache(ctx context.Context, key string, notification *entity.Notification) error {
	const op = "repository.cache.SaveToCache"

	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", op, err)
	}

	if setErr := s.rdb.SetWithExpiration(ctx, key, data, s.ttl); setErr != nil {
		return fmt.Errorf("%s: redis set: %w", op, setErr)
	}

	return nil
}

func (s *CacheRepository) InvalidateCache(ctx context.Context, id uuid.UUID) error {
	const op = "repository.cache.InvalidateCache"

	if err := s.rdb.Del(ctx, s.GetCacheKey(id)); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("%s: redis del: %w", op, err)
	}

	return nil
}
