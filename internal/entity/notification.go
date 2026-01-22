package entity

import (
	"time"

	"github.com/google/uuid"
)

// Notification представляет уведомление в системе
type Notification struct {
	ID                  uuid.UUID  `json:"id"`
	UserID              uuid.UUID  `json:"user_id"`
	Channel             Channel    `json:"channel"`
	Payload             string     `json:"payload"`
	RecipientIdentifier string     `json:"recipient_identifier"` // email или chat_id
	ScheduledAt         time.Time  `json:"scheduled_at"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	Status              Status     `json:"status"`
	RetryCount          int        `json:"retry_count"`
	LastError           string     `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// Channel представляет канал отправки уведомления
type Channel string

const (
	Telegram Channel = "telegram"
	Email    Channel = "email"
)

// String возвращает строковое представление канала
func (c Channel) String() string {
	return string(c)
}

// IsValid проверяет валидность канала
func (c Channel) IsValid() bool {
	switch c {
	case Telegram, Email:
		return true
	default:
		return false
	}
}

// Status представляет статус уведомления
type Status string

const (
	StatusWaiting   Status = "waiting"
	StatusInProcess Status = "in_process"
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// String возвращает строковое представление статуса
func (s Status) String() string {
	return string(s)
}
