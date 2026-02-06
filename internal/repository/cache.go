package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"delayednotifier/internal/entity"

	"github.com/google/uuid"
	"github.com/wb-go/wbf/redis"
)

const (
	_cacheTTL       = 5 * time.Minute
	_cacheKeyPrefix = "notify:"
)

type CacheRepository struct {
	rdb *redis.Client
}

func NewCacheRepository(rdb *redis.Client) *CacheRepository {
	return &CacheRepository{rdb: rdb}
}

func (s *CacheRepository) GetCacheKey(id uuid.UUID) string {
	return _cacheKeyPrefix + id.String()
}

func (s *CacheRepository) GetFromCache(ctx context.Context, key string) (*entity.Notification, error) {
	const op = "repository.cache.GetFromCache"

	cached, err := s.rdb.Get(ctx, key)
	if err != nil || cached == "" {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var notification entity.Notification
	if unmarshErr := json.Unmarshal([]byte(cached), &notification); unmarshErr != nil {
		return nil, fmt.Errorf("%s: unmarshal json: %w", op, unmarshErr)
	}

	return &notification, nil
}

func (s *CacheRepository) SaveToCache(ctx context.Context, key string, notification *entity.Notification) error {
	const op = "repository.cache.SaveToCache"
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("%s: marshal json: %w", op, err)
	}
	if setErr := s.rdb.SetWithExpiration(ctx, key, data, _cacheTTL); setErr != nil {
		return fmt.Errorf("%s: %w", op, setErr)
	}
	return nil
}

func (s *CacheRepository) InvalidateCache(ctx context.Context, id uuid.UUID) error {
	const op = "repository.cache.InvalidateCache"

	if err := s.rdb.Del(ctx, s.GetCacheKey(id)); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
