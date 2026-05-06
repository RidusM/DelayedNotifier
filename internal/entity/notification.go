package entity

import (
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
