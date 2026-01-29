package service

import (
	"errors"
	"time"
)

type Option func(*NotifyService)

func QueryLimit(limit uint64) Option {
	return func(s *NotifyService) {
		if limit > 0 {
			s.queryLimit = limit
		}
	}
}

func MaxRetries(retries uint32) Option {
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

func (s *NotifyService) validate() error {
	if s.queryLimit == 0 {
		return errors.New("invalid batch size: must be > 0")
	}
	if s.maxRetries == 0 {
		return errors.New("invalid max retries: must be > 0")
	}
	if s.retryDelay == 0 {
		return errors.New("invalid base retry delay: must be > 0")
	}
	if s.sender == nil {
		return errors.New("invalid sender: must be non-nil")
	}
	return nil
}
