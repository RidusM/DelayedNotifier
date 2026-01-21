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
	telegramSender NotificationSender
	emailSender    NotificationSender
}

func NewMultiSender(tg, email, push NotificationSender) *MultiSender {
	return &MultiSender{
		telegramSender: tg,
		emailSender:    email,
	}
}

func (s *MultiSender) Send(ctx context.Context, notification entity.Notification) error {
	switch notification.Channel {
	case entity.Telegram:
		if s.telegramSender != nil {
			return s.telegramSender.Send(ctx, notification)
		}
	case entity.Email:
		if s.emailSender != nil {
			return s.emailSender.Send(ctx, notification)
		}
	default:
		return fmt.Errorf("unsupported channel: %s", notification.Channel)
	}
	return nil
}
