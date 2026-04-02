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

	req := service.CreateNotificationRequest{
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
	s.Equal(entity.Email, notification.Channel)
	s.Equal("waiting", string(notification.Status))
}

func (s *IntegrationTestSuite) TestCancelNotification() {
	ctx := context.Background()

	req := service.CreateNotificationRequest{
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

	scheduledAt := time.Now().Add(100 * time.Millisecond).UTC()

	req := service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     `{"subject":"Test","body":"Hello"}`,
		Recipient:   "test.queue@example.com",
		ScheduledAt: scheduledAt,
	}

	notificationID, err := s.notifySvc.Create(ctx, req)
	s.Require().NoError(err)

	time.Sleep(200 * time.Millisecond)

	stats, err := s.notifySvc.ProcessQueue(ctx)
	s.Require().NoError(err)
	s.Equal(1, stats.Processed)
	s.Equal(0, stats.Failed)

	notification, err := s.notifySvc.GetStatus(ctx, notificationID)
	s.Require().NoError(err)
	s.Contains([]string{"sent", "in_process"}, string(notification.Status))
}
