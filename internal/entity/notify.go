package entity

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID                  uuid.UUID  `json:"id" validate:"required,uuid_strict"`
	UserID              uuid.UUID  `json:"user_id" validate:"required,uuid_strict"`
	Channel             Channel    `json:"channel" validate:"required,oneof=telegram email"`
	Payload             string     `json:"payload" validate:"required"`
	RecipientIdentifier string     `json:"recipient_identifier" validate:"required"`
	ScheduledAt         time.Time  `json:"scheduled_at" validate:"required"`
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

func NewNotification(
	userID uuid.UUID,
	channel Channel,
	recipient string,
	payload string,
	scheduledAt time.Time,
) (*Notification, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user_id required: %w", ErrInvalidData)
	}
	if !channel.IsValid() {
		return nil, fmt.Errorf("invalid channel %q: %w", channel, ErrInvalidData)
	}
	if recipient == "" {
		return nil, fmt.Errorf("recipient identifier required: %w", ErrInvalidData)
	}
	if payload == "" {
		return nil, fmt.Errorf("payload required: %w", ErrInvalidData)
	}
	if scheduledAt.IsZero() {
		return nil, fmt.Errorf("scheduled time required: %w", ErrInvalidData)
	}

	var err error
	nId, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("entity.notify.NewNotification: v7 uuid: %w", err)
	}

	scheduledAt = scheduledAt.UTC()
	now := time.Now().UTC()
	if !scheduledAt.After(now) {
		return nil, fmt.Errorf(
			"scheduled time must be in future (now=%s, scheduled=%s): %w",
			now.Format(time.RFC3339),
			scheduledAt.Format(time.RFC3339),
			ErrInvalidData,
		)
	}

	return &Notification{
		ID:                  nId,
		UserID:              userID,
		Channel:             channel,
		RecipientIdentifier: recipient,
		Payload:             payload,
		ScheduledAt:         scheduledAt,
		Status:              StatusWaiting,
		RetryCount:          0,
		CreatedAt:           now,
	}, nil
}

func (n *Notification) CanBeSent(now time.Time) bool {
	return n.Status == StatusWaiting &&
		n.ScheduledAt.Before(now) &&
		!n.IsTerminalStatus()
}

func (n *Notification) IsTerminalStatus() bool {
	return n.Status == StatusSent ||
		n.Status == StatusCancelled ||
		(n.Status == StatusFailed && n.RetryCount >= 10)
}

func (n *Notification) MarkAsSent() error {
	if n.Status == StatusSent {
		return fmt.Errorf("notification already sent: %w", ErrNotificationAlreadySent)
	}
	if n.Status == StatusCancelled {
		return fmt.Errorf("notification cancelled: %w", ErrNotificationCancelled)
	}

	n.Status = StatusSent
	now := time.Now().UTC()
	n.SentAt = &now
	return nil
}

func (n *Notification) MarkAsFailed(errMsg string) bool {
	n.Status = StatusFailed
	n.RetryCount++
	n.LastError = errMsg
	return n.RetryCount >= 10
}

func (n *Notification) MarkAsCancelled(reason string) error {
	if n.Status == StatusSent {
		return fmt.Errorf("cannot cancel sent notification: %w", ErrNotificationAlreadySent)
	}
	if n.Status == StatusCancelled {
		return fmt.Errorf("notification already cancelled: %w", ErrNotificationCancelled)
	}

	n.Status = StatusCancelled
	n.LastError = reason
	return nil
}

func (n *Notification) ValidateRecipientFormat() error {
	switch n.Channel {
	case Email:
		if !strings.Contains(n.RecipientIdentifier, "@") {
			return fmt.Errorf("invalid email format %q: %w", n.RecipientIdentifier, ErrInvalidData)
		}
	case Telegram:
		if _, err := strconv.ParseInt(n.RecipientIdentifier, 10, 64); err != nil {
			return fmt.Errorf("telegram recipient must be numeric chat ID %q: %w", n.RecipientIdentifier, ErrInvalidData)
		}
	default:
		return fmt.Errorf("unknown channel %q: %w", n.Channel, ErrInvalidData)
	}
	return nil
}
