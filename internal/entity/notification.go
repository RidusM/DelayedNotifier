package entity

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Channel string
type Status int

const (
	Email    Channel = "EMAIL"
	Telegram Channel = "TELEGRAM"

	StatusSent      Status = 2
	StatusInProcess Status = 1
	StatusWaiting   Status = 0
	StatusCancelled Status = -1
	StatusFailed    Status = -2
)

func (s Status) String() string {
	switch s {
	case StatusSent:
		return "sent"
	case StatusInProcess:
		return "in_process"
	case StatusWaiting:
		return "waiting"
	case StatusCancelled:
		return "cancelled"
	case StatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown_status(%d)", int(s))
	}
}

func (c Channel) String() string {
	return string(c)
}

type Notification struct {
	ID                  uuid.UUID  `db:"id" json:"notify_uid" validate:"required"`
	UserID              uuid.UUID  `db:"user_id" json:"user_uid" validate:"required"`
	Channel             Channel    `db:"channel" json:"channel" validate:"required,oneof=EMAIL TELEGRAM"`
	Payload             string     `db:"payload" json:"message" validate:"required,max=255"`
	ScheduledAt         time.Time  `db:"scheduled_at" json:"scheduled_at" validate:"required"`
	SentAt              *time.Time `db:"sent_at" json:"sent_at,omitempty"`
	Status              Status     `db:"status" json:"status"`
	RetryCount          int        `db:"retry_count" json:"retry_count"`
	LastError           string     `db:"last_error" json:"last_error"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	RecipientIdentifier string     `db:"recipient_identifier" json:"recipient_identifier" validate:"required"`
}

func NewNotification() Notification {
	id, _ := uuid.NewV7()
	return Notification{
		ID:        id,
		Status:    StatusWaiting,
		CreatedAt: time.Now().UTC(),
	}
}
