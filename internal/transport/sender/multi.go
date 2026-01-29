package sender

import (
	"context"
	"errors"
	"fmt"

	"delayednotifier/internal/entity"
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
	const op = "transport.sender.MultiSender"
	switch notification.Channel {
	case entity.Telegram:
		if s.telegram == nil {
			return errors.New("telegram sender not configured")
		}
		err := s.telegram.Send(ctx, notification)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	case entity.Email:
		if s.email == nil {
			return errors.New("email sender not configured")
		}
		err := s.email.Send(ctx, notification)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	default:
		return errors.New("unsupported channel")
	}
	return nil
}
