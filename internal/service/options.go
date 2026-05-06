package service

import (
	"errors"
	"fmt"
	"time"
)

type Option func(*NotifyService)

func MaxRetries(retries int) Option {
	return func(s *NotifyService) {
		if retries > 0 {
			s.maxRetries = retries
		}
	}
}

func RetryDelay(delay time.Duration) Option {
	return func(s *NotifyService) {
		if delay > 0 {
			s.retryDelay = delay
		}
	}
}

func QueryLimit(limit uint64) Option {
	return func(s *NotifyService) {
		if limit > 0 {
			s.queryLimit = limit
		}
	}
}

func (s *NotifyService) validate() error {
	if s.sender == nil {
		return errors.New("sender is required")
	}
	if s.maxRetries <= 0 {
		return fmt.Errorf("maxRetries must be > 0, got %d", s.maxRetries)
	}
	if s.retryDelay <= 0 {
		return fmt.Errorf("retryDelay must be > 0, got %v", s.retryDelay)
	}
	if s.queryLimit <= 0 {
		return fmt.Errorf("queryLimit must be > 0, got %d", s.queryLimit)
	}
	return nil
}
