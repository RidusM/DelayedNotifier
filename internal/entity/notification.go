package entity

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID                  uuid.UUID  `json:"id" validate:"required,uuid_strict"`
	UserID              uuid.UUID  `json:"user_id" validate:"required,uuid_strict"`
	Channel             Channel    `json:"channel" validate:"required,oneof=telegram email"`
	Payload             string     `json:"payload" validate:"required"`
	RecipientIdentifier string     `json:"recipient_identifier" validate:"required"`
	ScheduledAt         time.Time  `json:"scheduled_at" validate:"required,gtfield=CreatedAt"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	Status              Status     `json:"status" validate:"required,oneof=waiting in_process sent failed cancelled"`
	RetryCount          int        `json:"retry_count" validate:"required,min=0,max=10"`
	LastError           string     `json:"last_error,omitempty" validate:"max=1000"`
	CreatedAt           time.Time  `json:"created_at" validate:"required"`
}

type Channel string

const (
	Telegram Channel = "telegram"
	Email    Channel = "email"
)

func (c Channel) String() string {
	return string(c)
}

func (c Channel) IsValid() bool {
	switch c {
	case Telegram, Email:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusWaiting   Status = "waiting"
	StatusInProcess Status = "in_process"
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsValid() bool {
	switch s {
	case StatusWaiting, StatusInProcess, StatusSent, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}
