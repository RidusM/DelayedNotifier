package service

import (
	"errors"
	"time"
)

type Option func(*NotifyService)

func WithBatchSize(size uint64) Option {
	return func(s *NotifyService) {
		if size > 0 && size <= _maxBatchSize {
			s.batchSize = size
		}
	}
}

func WithMaxRetries(retries int) Option {
	return func(s *NotifyService) {
		if retries > 0 {
			s.maxRetries = retries
		}
	}
}

func WithBaseRetryDelay(delay time.Duration) Option {
	return func(s *NotifyService) {
		if delay > 0 {
			s.baseRetryDelay = delay
		}
	}
}

func WithCleanupAge(age time.Duration) Option {
	return func(s *NotifyService) {
		if age > 0 {
			s.cleanupAge = age
		}
	}
}

func WithSender(sender NotificationSender) Option {
	return func(s *NotifyService) {
		if sender != nil {
			s.sender = sender
		}
	}
}

func (s *NotifyService) validate() error {
	if s.batchSize == 0 {
		return errors.New("invalid batch size: must be > 0")
	}
	if s.maxRetries == 0 {
		return errors.New("invalid max retries: must be > 0")
	}
	if s.baseRetryDelay == 0 {
		return errors.New("invalid base retry delay: must be > 0")
	}
	if s.cleanupAge == 0 {
		return errors.New("invalid cleanup age: must be > 0")
	}
	if s.sender == nil {
		return errors.New("invalid sender: must be non-nil")
	}
	return nil
}
