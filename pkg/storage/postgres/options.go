package postgres

import (
	"errors"
	"time"
)

type Option func(*Postgres)

func MaxPoolSize(size int32) Option {
	return func(p *Postgres) {
		p.maxPoolSize = size
	}
}

func MaxConnAttempts(attempts int) Option {
	return func(p *Postgres) {
		p.connAttempts = attempts
	}
}

func BaseRetryDelay(delay time.Duration) Option {
	return func(p *Postgres) {
		p.baseRetryDelay = delay
	}
}

func MaxRetryDelay(delay time.Duration) Option {
	return func(p *Postgres) {
		p.maxRetryDelay = delay
	}
}

func (p *Postgres) validate() error {
	if p.maxPoolSize <= 0 {
		return errors.New("invalid maxPoolSize: must be > 0")
	}

	if p.connAttempts <= 0 {
		return errors.New("invalid connAttempts: must be > 0")
	}

	if p.baseRetryDelay <= 0 {
		return errors.New("invalid base retry delay: must be > 0")
	}

	if p.maxRetryDelay <= 0 {
		return errors.New("invalid max retry delay: must be > 0")
	}

	if p.baseRetryDelay > p.maxRetryDelay {
		return errors.New("baseRetryDelay cannot exceed maxRetryDelay")
	}
	return nil
}
