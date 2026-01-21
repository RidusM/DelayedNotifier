package entity

import "errors"

var (
	ErrDataNotFound            = errors.New("data not found")
	ErrConflictingData         = errors.New("data conflicts with existing data in unique column")
	ErrInvalidData             = errors.New("invalid data")
	ErrConfigPathNotSet        = errors.New("CONFIG_PATH not set and -config flag not provided")
	ErrNotificationNotFound    = errors.New("notification not found")
	ErrNotificationAlreadySent = errors.New("notification already sent")
	ErrNotificationCancelled   = errors.New("notification already cancelled")
	ErrInvalidScheduledTime    = errors.New("scheduled_at must be in the future")
	ErrEmptyBatch              = errors.New("batch cannot be empty")
)
