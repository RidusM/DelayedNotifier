package sender

import (
	"context"
	"delayednotifier/internal/entity"
	"fmt"

	"github.com/wb-go/wbf/logger"
	"gopkg.in/gomail.v2"
)

// EmailSender отправляет уведомления через Email
type EmailSender struct {
	dialer *gomail.Dialer
	from   string
	log    logger.Logger
}

// NewEmailSender создает новый EmailSender
func NewEmailSender(smtpHost string, smtpPort int, username, password, from string, log logger.Logger) *EmailSender {
	dialer := gomail.NewDialer(smtpHost, smtpPort, username, password)

	log.LogAttrs(context.Background(), logger.InfoLevel, "email sender initialized",
		logger.String("smtp_host", smtpHost),
		logger.Int("smtp_port", smtpPort),
		logger.String("from", from),
	)

	return &EmailSender{
		dialer: dialer,
		from:   from,
		log:    log,
	}
}

// Send отправляет email уведомление
func (s *EmailSender) Send(ctx context.Context, notification entity.Notification) error {
	email := gomail.NewMessage()
	email.SetHeader("From", s.from)
	email.SetHeader("To", notification.RecipientIdentifier)
	email.SetHeader("Subject", "Notification")
	email.SetBody("text/html", notification.Payload)

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending email",
		logger.String("to", notification.RecipientIdentifier),
		logger.String("notification_id", notification.ID.String()),
	)

	if err := s.dialer.DialAndSend(email); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "email sent",
		logger.String("to", notification.RecipientIdentifier),
		logger.String("notification_id", notification.ID.String()),
	)

	return nil
}
