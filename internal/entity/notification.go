package entity

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID                  uuid.UUID  `json:"id"                   validate:"required,uuid"`
	Channel             Channel    `json:"channel"              validate:"required,oneof=telegram email"`
	Payload             string     `json:"payload"`
	RecipientIdentifier string     `json:"recipient_identifier" validate:"required"`
	ScheduledAt         time.Time  `json:"scheduled_at"         validate:"required"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	Status              Status     `json:"status"               validate:"required,oneof=waiting in_process sent failed cancelled"`
	RetryCount          int        `json:"retry_count"`
	LastError           *string    `json:"last_error,omitempty"`

	CreatedAt time.Time `json:"created_at" validate:"-"`
}

func (n *Notification) NormalizeTimezones() {
	if !n.ScheduledAt.IsZero() {
		n.ScheduledAt = n.ScheduledAt.UTC()
	}
	if n.SentAt != nil && !n.SentAt.IsZero() {
		t := n.SentAt.UTC()
		n.SentAt = &t
	}
	if !n.CreatedAt.IsZero() {
		n.CreatedAt = n.CreatedAt.UTC()
	}
}
