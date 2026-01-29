package entity

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID                  uuid.UUID  `json:"id"`
	UserID              uuid.UUID  `json:"user_id"`
	Channel             Channel    `json:"channel"`
	Payload             string     `json:"payload"`
	RecipientIdentifier string     `json:"recipient_identifier"`
	ScheduledAt         time.Time  `json:"scheduled_at"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	Status              Status     `json:"status"`
	RetryCount          uint32     `json:"retry_count"`
	LastError           string     `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
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
