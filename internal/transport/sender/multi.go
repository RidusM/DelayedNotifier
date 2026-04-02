package sender

import (
	"context"
	"fmt"

	"delayednotifier/internal/entity"
)

type NotificationSender interface {
	Send(ctx context.Context, notification entity.Notification) error
}

type MultiSender struct {
	senders map[entity.Channel]NotificationSender
}

func NewMultiSender() *MultiSender {
	return &MultiSender{
		senders: make(map[entity.Channel]NotificationSender),
	}
}

func (s *MultiSender) Register(channel entity.Channel, sender NotificationSender) {
	s.senders[channel] = sender
}

func (s *MultiSender) Send(ctx context.Context, n entity.Notification) error {
	const op = "transport.multi.Send"

	if !n.Channel.IsValid() {
		return fmt.Errorf("%s: invalid channel %q", op, n.Channel)
	}

	sender, ok := s.senders[n.Channel]
	if !ok {
		return fmt.Errorf("%s: no sender registered for channel %q", op, n.Channel)
	}

	err := sender.Send(ctx, n)
	if err != nil {
		return fmt.Errorf("%s: channel=%q: %w", op, n.Channel, err)
	}
	return nil
}
