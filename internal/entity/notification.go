package entity

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID                  uuid.UUID  `json:"id"                   validate:"required,uuid_strict"`
	Channel             Channel    `json:"channel"              validate:"required,oneof=telegram email"`
	Payload             string     `json:"payload"              validate:"required"`
	RecipientIdentifier string     `json:"recipient_identifier" validate:"required"`
	ScheduledAt         time.Time  `json:"scheduled_at"         validate:"required"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	Status              Status     `json:"status"               validate:"required,oneof=waiting in_process sent failed cancelled"`
	RetryCount          int        `json:"retry_count"          validate:"min=0,max=10"`

	LastError *string `json:"last_error,omitempty"`

	CreatedAt time.Time `json:"created_at" validate:"required"`
}
