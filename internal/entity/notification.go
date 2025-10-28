package entity

import (
	"time"

	"github.com/google/uuid"
)

type Channel string

type Status int

const (
	Email    Channel = "EMAIL"
	Telegram Channel = "TELEGRAM"
	PUSH     Channel = "PUSH"

	Send    Status = 1
	Waiting Status = 0
	Lose    Status = -1
)

type Notification struct {
	Id          uuid.UUID `json:"notify_uid"          validate:"required,uuid_strict"` // make v7
	UserID      uuid.UUID `json:"user_uid" validate:"required,uuid_strict"`
	Channel     Channel   `json:"channel"        validate:"required"`
	Payload     string    `json:"message" validate:"required,max=255"`
	ScheduledAt time.Time `json:"scheduled_at" validate:"required"`
	SentAt      time.Time `json:"sent_at" validate:"required"`
	Status      Status    `json:"status" validate:"required"`
	RetryCount  int       `json:"retry_count" validate:"required"`
	LastError   string    `json:"last_error" validate:"required"`
	CreatedAt   time.Time `json:"created_at"       validate:"required"`
}
