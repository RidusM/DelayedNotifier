package sender

import (
	"context"
	"delayednotifier/internal/entity"
	"fmt"
)

type NotificationSender interface {
	Send(ctx context.Context, notification entity.Notification) error
}

type MultiSender struct {
	telegram NotificationSender
	email    NotificationSender
}

func NewMultiSender(telegram, email NotificationSender) *MultiSender {
	return &MultiSender{
		telegram: telegram,
		email:    email,
	}
}

func (s *MultiSender) Send(ctx context.Context, notification entity.Notification) error {
	switch notification.Channel {
	case entity.Telegram:
		if s.telegram == nil {
			return fmt.Errorf("telegram sender not configured")
		}
		return s.telegram.Send(ctx, notification)

	case entity.Email:
		if s.email == nil {
			return fmt.Errorf("email sender not configured")
		}
		return s.email.Send(ctx, notification)

	default:
		return fmt.Errorf("unsupported channel: %s", notification.Channel)
	}
}
