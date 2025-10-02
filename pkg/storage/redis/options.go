package redis

import (
	"errors"
	"time"
)

type Option func(*Redis)

func PoolSize(size int) Option {
	return func(r *Redis) {
		r.poolSize = size
	}
}

func MinIdleCons(cons int) Option {
	return func(r *Redis) {
		r.minIdleCons = cons
	}
}

func PoolTimeout(timeout time.Duration) Option {
	return func(r *Redis) {
		r.poolTimeout = timeout
	}
}

func (r *Redis) validate() error {
	if r.poolSize <= 0 {
		return errors.New("invalid poolSize: must be > 0")
	}

	if r.minIdleCons <= 0 {
		return errors.New("invalid minIdleCons: must be > 0")
	}

	if r.poolTimeout <= 0 {
		return errors.New("invalid poolTimeout: must be > 0")
	}

	return nil
}
