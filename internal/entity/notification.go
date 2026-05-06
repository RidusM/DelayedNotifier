package entity

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Channel     Channel
	Payload     string
	ScheduledAt time.Time
	SentAt      *time.Time
	Status      Status
	RetryCount  int
	LastError   *string
	CreatedAt   time.Time
}

func (n *Notification) Validate() error {
	if n.ID == uuid.Nil {
		return fmt.Errorf("notification id is empty: %w", ErrInvalidData)
	}
	if n.UserID == uuid.Nil {
		return fmt.Errorf("user id is empty: %w", ErrInvalidData)
	}
	if !n.Channel.IsValid() {
		return fmt.Errorf("invalid channel '%s': %w", n.Channel, ErrInvalidData)
	}
	if n.Payload == "" {
		return fmt.Errorf("payload is empty: %w", ErrInvalidData)
	}
	if n.ScheduledAt.IsZero() {
		return fmt.Errorf("scheduled_at is zero: %w", ErrInvalidData)
	}
	if n.Status == "" {
		return fmt.Errorf("status is empty: %w", ErrInvalidData)
	}
	if !n.Status.IsValid() {
		return fmt.Errorf("invalid status '%s': %w", n.Status, ErrInvalidData)
	}
	return nil
}
