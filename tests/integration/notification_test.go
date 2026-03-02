package integration_test

import (
	"context"
	"time"

	"delayednotifier/internal/entity"
	"delayednotifier/internal/service"

	"github.com/google/uuid"
)

func (s *IntegrationTestSuite) TestCreateAndGetNotification() {
	ctx := context.Background()

	userID := uuid.New()
	_, err := s.db.Pool.Exec(ctx, `
        INSERT INTO user_email_links (user_id, email, verified)
        VALUES ($1, $2, true)
        ON CONFLICT (user_id) DO UPDATE SET email = EXCLUDED.email`,
		userID, "test.user@example.com")
	s.Require().NoError(err)

	req := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     `{"subject":"Test","body":"<p>Hello!</p>"}`,
		Recipient:   "test.user@example.com",
		ScheduledAt: time.Now().Add(1 * time.Hour).UTC(),
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)
	s.NotEqual(uuid.Nil, notificationID)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Equal(userID, notification.UserID)
	s.Equal(entity.Email, notification.Channel)
	s.Equal("waiting", string(notification.Status))
}

func (s *IntegrationTestSuite) TestCancelNotification() {
	ctx := context.Background()

	userID := uuid.New()
	_, err := s.db.Pool.Exec(ctx, `
        INSERT INTO user_telegram_links (user_id, telegram_chat_id, telegram_username)
        VALUES ($1, $2, $3)
        ON CONFLICT (user_id) DO UPDATE SET telegram_chat_id = EXCLUDED.telegram_chat_id`,
		userID, 123456789, "@test_user")
	s.Require().NoError(err)

	req := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Telegram,
		Payload:     "Test message",
		Recipient:   "123456789",
		ScheduledAt: time.Now().Add(1 * time.Hour).UTC(),
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)

	err = s.notifySvc.Cancel(ctx, notificationID)
	s.Require().NoError(err)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Equal("cancelled", string(notification.Status))
}

func (s *IntegrationTestSuite) TestProcessQueueMarksAsInProcess() {
	ctx := context.Background()

	userID := uuid.New()
	_, err := s.db.Pool.Exec(ctx, `
        INSERT INTO user_email_links (user_id, email, verified)
        VALUES ($1, $2, true)
        ON CONFLICT (user_id) DO UPDATE SET email = EXCLUDED.email`,
		userID, "test.queue@example.com")
	s.Require().NoError(err)

	// Добавлено: Recipient
	req := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     `{"subject":"Test","body":"Hello"}`,
		Recipient:   "test.queue@example.com",
		ScheduledAt: time.Now().Add(1 * time.Minute).UTC(),
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)

	stats, err := s.notifySvc.ProcessQueue(ctx)
	s.Require().NoError(err)
	s.Equal(1, stats.Processed)
	s.Equal(0, stats.Failed)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Equal("in_process", string(notification.Status))
}
