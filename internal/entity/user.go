package entity

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID         uuid.UUID
	Name       string
	Email      string
	TelegramID *int64
	CreatedAt  time.Time
}
