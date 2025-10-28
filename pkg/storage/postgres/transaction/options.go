package transaction

import (
	"errors"
	"time"
)

type Option func(*manager)

func MaxAttempts(attempts int) Option {
	return func(m *manager) {
		m.maxAttempts = attempts
	}
}

func BaseRetryDelay(delay time.Duration) Option {
	return func(m *manager) {
		m.baseRetryDelay = delay
	}
}

func MaxRetryDelay(delay time.Duration) Option {
	return func(m *manager) {
		m.maxRetryDelay = delay
	}
}

func (m *manager) validate() error {
	if m.maxAttempts <= 0 {
		return errors.New("invalid connAttempts: must be > 0")
	}

	if m.baseRetryDelay <= 0 {
		return errors.New("invalid base retry delay: must be > 0")
	}

	if m.maxRetryDelay <= 0 {
		return errors.New("invalid max retry delay: must be > 0")
	}

	if m.baseRetryDelay > m.maxRetryDelay {
		return errors.New("baseRetryDelay cannot exceed maxRetryDelay")
	}
	return nil
}
