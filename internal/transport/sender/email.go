package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"time"

	"delayednotifier/internal/entity"

	"github.com/wb-go/wbf/logger"
	"gopkg.in/gomail.v2"
)

type EmailSender struct {
	dialer *gomail.Dialer
	from   string
	log    logger.Logger
}

func NewEmailSender(smtpHost string, smtpPort int, username, password, from string, log logger.Logger) *EmailSender {
	return &EmailSender{
		dialer: gomail.NewDialer(smtpHost, smtpPort, username, password),
		from:   from,
		log:    log,
	}
}

func (s *EmailSender) Send(ctx context.Context, n entity.Notification) error {
	const op = "sender.email.Send"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if len(n.RecipientIdentifier) > 254 {
		return fmt.Errorf("%s: recipient email too long (max 254 chars): %w", op, entity.ErrInvalidData)
	}

	var payload struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if err := json.Unmarshal([]byte(n.Payload), &payload); err != nil {
		payload.Body = n.Payload
		payload.Subject = "Notification"
	} else {
		if payload.Subject == "" {
			payload.Subject = "Notification"
		}
	}

	if len(payload.Body) > 100000 {
		return fmt.Errorf("%s: email body too large (max 100KB): %w", op, entity.ErrInvalidData)
	}
	if len(payload.Subject) > 255 {
		return fmt.Errorf("%s: email subject too long (max 255 chars): %w", op, entity.ErrInvalidData)
	}

	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", n.RecipientIdentifier)
	m.SetHeader("Subject", mime.QEncoding.Encode("utf-8", payload.Subject))
	m.SetBody("text/html", payload.Body)

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending email",
		logger.String("to", n.RecipientIdentifier),
		logger.String("notification_id", n.ID.String()),
		logger.String("subject", payload.Subject),
	)

	done := make(chan error, 1)
	go func() {
		done <- s.dialer.DialAndSend(m)
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s: dial and send: %w", op, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", op, ctx.Err())
	case <-time.After(10 * time.Second):
		return fmt.Errorf("%s: timeout after 10s", op)
	}
}
