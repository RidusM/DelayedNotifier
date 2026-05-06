package sender

import (
	"context"
	"fmt"

	"delayednotifier/internal/entity"
)

type NotificationSender interface {
	Send(ctx context.Context, n entity.Notification, recipient string) error
}

type MultiSender struct {
	senders map[entity.Channel]NotificationSender
}

func NewMultiSender() *MultiSender {
	return &MultiSender{
		senders: make(map[entity.Channel]NotificationSender),
	}
}

func (m *MultiSender) Register(channel entity.Channel, sender NotificationSender) {
	m.senders[channel] = sender
}

func (m *MultiSender) Send(ctx context.Context, n entity.Notification, recipient string) error {
	const op = "sender.MultiSender.Send"

	if !n.Channel.IsValid() {
		return fmt.Errorf("%s: invalid channel %q", op, n.Channel)
	}

	sender, ok := m.senders[n.Channel]
	if !ok {
		return fmt.Errorf("%s: no sender registered for channel %q", op, n.Channel)
	}

	if err := sender.Send(ctx, n, recipient); err != nil {
		return fmt.Errorf("%s: channel=%q: %w", op, n.Channel, err)
	}
	return nil
}
