package entity

import "errors"

var (
	ErrDataNotFound            = errors.New("data not found")
	ErrConflictingData         = errors.New("conflicting data")
	ErrInvalidData             = errors.New("invalid data")
	ErrNotificationAlreadySent = errors.New("notification already sent")
	ErrNotificationCancelled   = errors.New("notification already cancelled")
	ErrRecipientNotFound       = errors.New("recipient not found")
)
