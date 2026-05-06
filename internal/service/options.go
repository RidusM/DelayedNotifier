package service

import (
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
