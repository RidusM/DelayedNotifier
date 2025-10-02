package entity

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	Id        uuid.UUID `json:"id"          validate:"required,uuid_strict"`
	Timestamp time.Time `json:"timestamp"       validate:"required"`
	Channel   string    `json:"channel"        validate:"required,max=50"`
	Message   string    `json:"message"        validate:"required,max=255"`
	User      string    `json:"user" validate:"required"`
}
