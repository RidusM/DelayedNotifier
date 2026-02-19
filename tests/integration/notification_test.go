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

	req := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     `{"subject":"Test","body":"<p>Hello!</p>"}`,
		ScheduledAt: time.Now().Add(1 * time.Hour).UTC(),
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)
	s.NotEqual(s.T(), uuid.Nil, notificationID)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Equal(s.T(), userID, notification.UserID)
	s.Equal(s.T(), entity.Email, notification.Channel)
	s.Equal(s.T(), "waiting", string(notification.Status))
}

func (s *IntegrationTestSuite) TestCancelNotification() {
	ctx := context.Background()
	userID := uuid.New()

	req := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Telegram,
		Payload:     "Test message",
		ScheduledAt: time.Now().Add(1 * time.Hour).UTC(),
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)

	err = s.notifySvc.Cancel(ctx, notificationID)
	s.Require().NoError(err)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Equal(s.T(), "cancelled", string(notification.Status))
}

func (s *IntegrationTestSuite) TestProcessQueueMarksAsInProcess() {
	ctx := context.Background()
	userID := uuid.New()

	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO user_email_links (user_id, email, verified)
		VALUES ($1, $2, true)
		ON CONFLICT (user_id) DO NOTHING`,
		userID, "test@example.com")
	s.Require().NoError(err)

	req := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     `{"subject":"Test","body":"Hello"}`,
		ScheduledAt: time.Now().Add(-1 * time.Minute).UTC(),
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)

	stats, err := s.notifySvc.ProcessQueue(ctx)
	s.Require().NoError(err)
	s.Equal(s.T(), 1, stats.Processed)
	s.Equal(s.T(), 0, stats.Failed)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Equal(s.T(), "in_process", string(notification.Status))
}
