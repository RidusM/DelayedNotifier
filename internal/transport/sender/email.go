// sender/email.go
package sender

import (
	"context"
	"delayednotifier/internal/entity"
	"fmt"

	"gopkg.in/gomail.v2"
)

type EmailSender struct {
	dialer *gomail.Dialer
	from   string
}

func NewEmailSender(smtpHost string, smtpPort int, username, password, from string) *EmailSender {
	dialer := gomail.NewDialer(smtpHost, smtpPort, username, password)
	return &EmailSender{
		dialer: dialer,
		from:   from,
	}
}

func (s *EmailSender) Send(ctx context.Context, notification entity.Notification) error {
	email := gomail.NewMessage()
	email.SetHeader("From", s.from)
	email.SetHeader("To", notification.RecipientIdentifier)
	email.SetHeader("Subject", "Notification")
	email.SetBody("text/plain", notification.Payload)

	if err := s.dialer.DialAndSend(email); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}
