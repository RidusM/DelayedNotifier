package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"delayednotifier/internal/entity"
	mock_repository "delayednotifier/internal/repository/mock"
	mock_sender "delayednotifier/internal/transport/sender/mock"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/logger"
	"go.uber.org/mock/gomock"
)

type MockTM struct{}

func (m *MockTM) ExecuteInTransaction(ctx context.Context, name string, fn func(pgxdriver.QueryExecuter) error) error {
	return fn(nil)
}

func setup(
	t *testing.T,
) (*NotifyService, *mock_repository.MockNotifyRepository, *mock_repository.MockUserRepository, *mock_repository.MockCacheRepository, *mock_sender.MockNotificationSender, *gomock.Controller) {
	ctrl := gomock.NewController(t)
	notifyRepo := mock_repository.NewMockNotifyRepository(ctrl)
	userRepo := mock_repository.NewMockUserRepository(ctrl)
	cacheRepo := mock_repository.NewMockCacheRepository(ctrl)
	sender := mock_sender.NewMockNotificationSender(ctrl)

	log, err := logger.NewZapAdapter("test-app-notify", "test", logger.WithLevel(logger.ErrorLevel))
	if err != nil {
		return nil, nil, nil, nil, nil, nil
	}

	svc := &NotifyService{
		notifyRepo: notifyRepo,
		userRepo:   userRepo,
		cache:      cacheRepo,
		sender:     sender,
		tm:         &MockTM{},
		log:        log,
		maxRetries: 3,
		queryLimit: 10,
		retryDelay: 5 * time.Minute,
	}
	return svc, notifyRepo, userRepo, cacheRepo, sender, ctrl
}

func TestCreate_Success_Email(t *testing.T) {
	svc, notifyRepo, userRepo, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	userID := uuid.New()
	userRepo.EXPECT().GetEmail(ctx, nil, userID).Return("a@b.com", nil)
	notifyRepo.EXPECT().Create(ctx, nil, gomock.Any()).Return(nil)
	id, err := svc.Create(ctx, CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}

func TestCreate_Success_Telegram(t *testing.T) {
	svc, notifyRepo, userRepo, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	userID := uuid.New()
	userRepo.EXPECT().GetTelegramChatID(ctx, nil, userID).Return(int64(123), nil)
	notifyRepo.EXPECT().Create(ctx, nil, gomock.Any()).Return(nil)
	_, err := svc.Create(ctx, CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Telegram,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
}

func TestCreate_UserIDRequired(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	_, err := svc.Create(context.Background(), CreateNotificationRequest{
		UserID:      uuid.Nil,
		Channel:     entity.Email,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id is required")
}

func TestCreate_ChannelRequired(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	_, err := svc.Create(context.Background(), CreateNotificationRequest{
		UserID:      uuid.New(),
		Channel:     "",
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel is required")
}

func TestCreate_PayloadRequired(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	_, err := svc.Create(context.Background(), CreateNotificationRequest{
		UserID:      uuid.New(),
		Channel:     entity.Email,
		Payload:     "",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload is required")
}

func TestCreate_ScheduledAtRequired(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	_, err := svc.Create(context.Background(), CreateNotificationRequest{
		UserID:  uuid.New(),
		Channel: entity.Email,
		Payload: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduled_at is required")
}

func TestCreate_MustBeFuture(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	_, err := svc.Create(context.Background(), CreateNotificationRequest{
		UserID:      uuid.New(),
		Channel:     entity.Email,
		Payload:     "test",
		ScheduledAt: time.Now().Add(-1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in future")
}

func TestCreate_PayloadTooLarge(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	_, err := svc.Create(context.Background(), CreateNotificationRequest{
		UserID:      uuid.New(),
		Channel:     entity.Email,
		Payload:     string(make([]byte, 100001)),
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestCreate_EmailNotFound(t *testing.T) {
	svc, _, userRepo, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	userID := uuid.New()
	userRepo.EXPECT().GetEmail(ctx, nil, userID).Return("", entity.ErrDataNotFound)
	_, err := svc.Create(ctx, CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email not found")
}

func TestCreate_TelegramNotFound(t *testing.T) {
	svc, _, userRepo, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	userID := uuid.New()
	userRepo.EXPECT().GetTelegramChatID(ctx, nil, userID).Return(int64(0), entity.ErrDataNotFound)
	_, err := svc.Create(ctx, CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Telegram,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram chat_id not found")
}

func TestCreate_RepositoryError(t *testing.T) {
	svc, notifyRepo, userRepo, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	userID := uuid.New()
	dbErr := errors.New("db error")
	userRepo.EXPECT().GetEmail(ctx, nil, userID).Return("a@b.com", nil)
	notifyRepo.EXPECT().Create(ctx, nil, gomock.Any()).Return(dbErr)
	_, err := svc.Create(ctx, CreateNotificationRequest{
		UserID:      userID,
		Channel:     entity.Email,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
}

func TestGetStatus_FromCache(t *testing.T) {
	svc, _, _, cacheRepo, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	key := "key"
	cached := &entity.Notification{ID: id, Status: entity.StatusWaiting}
	cacheRepo.EXPECT().GetCacheKey(id).Return(key)
	cacheRepo.EXPECT().GetFromCache(ctx, key).Return(cached, nil)
	result, err := svc.GetStatus(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, cached, result)
}

func TestGetStatus_FromDB(t *testing.T) {
	svc, notifyRepo, _, cacheRepo, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	key := "key"
	notif := &entity.Notification{ID: id, Status: entity.StatusSent}
	cacheRepo.EXPECT().GetCacheKey(id).Return(key)
	cacheRepo.EXPECT().GetFromCache(ctx, key).Return(nil, errors.New("miss"))
	notifyRepo.EXPECT().GetByID(ctx, nil, id, false).Return(notif, nil)
	cacheRepo.EXPECT().SaveToCache(ctx, key, notif).Return(nil)
	result, err := svc.GetStatus(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, notif, result)
}

func TestGetStatus_NotFound(t *testing.T) {
	svc, notifyRepo, _, cacheRepo, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	key := "key"
	cacheRepo.EXPECT().GetCacheKey(id).Return(key)
	cacheRepo.EXPECT().GetFromCache(ctx, key).Return(nil, errors.New("miss"))
	notifyRepo.EXPECT().GetByID(ctx, nil, id, false).Return(nil, entity.ErrDataNotFound)
	_, err := svc.GetStatus(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotificationNotFound)
}

func TestCancel_Success(t *testing.T) {
	svc, notifyRepo, _, cacheRepo, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notif := &entity.Notification{ID: id, Status: entity.StatusWaiting}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(notif, nil)
	notifyRepo.EXPECT().UpdateStatus(ctx, nil, id, entity.StatusCancelled, gomock.Any()).Return(nil)
	cacheRepo.EXPECT().InvalidateCache(ctx, id).Return(nil)
	err := svc.Cancel(ctx, id)
	require.NoError(t, err)
}

func TestCancel_NotFound(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(nil, entity.ErrDataNotFound)
	err := svc.Cancel(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotificationNotFound)
}

func TestCancel_AlreadySent(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notif := &entity.Notification{ID: id, Status: entity.StatusSent}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(notif, nil)
	err := svc.Cancel(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotificationAlreadySent)
}

func TestCancel_AlreadyCancelled(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notif := &entity.Notification{ID: id, Status: entity.StatusCancelled}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(notif, nil)
	err := svc.Cancel(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotificationCancelled)
}

// ============ PROCESSQUEUE TESTS ============

func TestProcessQueue_NoNotifications(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	notifyRepo.EXPECT().GetForProcess(ctx, nil, uint64(10)).Return([]entity.Notification{}, nil)
	stats, err := svc.ProcessQueue(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Processed)
	assert.Equal(t, 0, stats.Failed)
}

func TestProcessQueue_RepositoryError(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	dbErr := errors.New("db error")
	notifyRepo.EXPECT().GetForProcess(ctx, nil, uint64(10)).Return(nil, dbErr)
	stats, err := svc.ProcessQueue(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
	assert.Equal(t, 0, stats.Processed)
}

// ============ WORKERHANDLER TESTS ============

func TestWorkerHandler_Success(t *testing.T) {
	svc, notifyRepo, _, cacheRepo, sender, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess, Channel: entity.Email}
	body, _ := json.Marshal(notif)
	delivery := amqp091.Delivery{Body: body}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(&notif, nil)
	sender.EXPECT().Send(ctx, notif).Return(nil)
	notifyRepo.EXPECT().UpdateStatus(ctx, nil, id, entity.StatusSent, nil).Return(nil)
	cacheRepo.EXPECT().InvalidateCache(ctx, id).Return(nil)
	handler := svc.GetWorkerHandler()
	err := handler(ctx, delivery)
	require.NoError(t, err)
}

func TestWorkerHandler_NotFound(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notif := entity.Notification{ID: id}
	body, _ := json.Marshal(notif)
	delivery := amqp091.Delivery{Body: body}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(nil, entity.ErrDataNotFound)
	handler := svc.GetWorkerHandler()
	err := handler(ctx, delivery)
	require.NoError(t, err)
}

func TestWorkerHandler_StatusChanged(t *testing.T) {
	svc, notifyRepo, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess}
	current := &entity.Notification{ID: id, Status: entity.StatusCancelled}
	body, _ := json.Marshal(notif)
	delivery := amqp091.Delivery{Body: body}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(current, nil)
	handler := svc.GetWorkerHandler()
	err := handler(ctx, delivery)
	require.NoError(t, err)
}

func TestWorkerHandler_SendFailed_WithRetry(t *testing.T) {
	svc, notifyRepo, _, cacheRepo, sender, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	sendErr := errors.New("send failed")
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess, RetryCount: 0}
	body, _ := json.Marshal(notif)
	delivery := amqp091.Delivery{Body: body}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(&notif, nil).Times(2)
	sender.EXPECT().Send(ctx, notif).Return(sendErr)
	notifyRepo.EXPECT().UpdateStatus(ctx, nil, id, entity.StatusFailed, gomock.Any()).Return(nil)
	notifyRepo.EXPECT().RescheduleNotification(ctx, nil, id, gomock.Any()).Return(nil)
	cacheRepo.EXPECT().InvalidateCache(ctx, id).Return(nil)
	handler := svc.GetWorkerHandler()
	err := handler(ctx, delivery)
	require.Error(t, err)
}

func TestWorkerHandler_MaxRetriesReached(t *testing.T) {
	svc, notifyRepo, _, cacheRepo, sender, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	id := uuid.New()
	sendErr := errors.New("send failed")
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess, RetryCount: 3}
	body, _ := json.Marshal(notif)
	delivery := amqp091.Delivery{Body: body}
	notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(&notif, nil).Times(2)
	sender.EXPECT().Send(ctx, notif).Return(sendErr)
	notifyRepo.EXPECT().UpdateStatus(ctx, nil, id, entity.StatusFailed, gomock.Any()).Return(nil)
	cacheRepo.EXPECT().InvalidateCache(ctx, id).Return(nil)
	handler := svc.GetWorkerHandler()
	err := handler(ctx, delivery)
	require.Error(t, err)
}

func TestWorkerHandler_InvalidJSON(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()
	ctx := context.Background()
	delivery := amqp091.Delivery{Body: []byte("invalid")}
	handler := svc.GetWorkerHandler()
	err := handler(ctx, delivery)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestCalculateNextAttempt(t *testing.T) {
	svc, _, _, _, _, ctrl := setup(t)
	defer ctrl.Finish()

	tests := []struct {
		retry int
		delay time.Duration
	}{
		{0, 5 * time.Minute},
		{1, 5 * time.Minute},
		{2, 5 * time.Minute},
		{10, 5 * time.Minute},
		{-1, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("retry_% d", tt.retry), func(t *testing.T) {
			now := time.Now().UTC()
			next := svc.calculateNextAttempt(tt.retry)
			delta := next.Sub(now)

			assert.InDelta(t, tt.delay.Seconds(), delta.Seconds(), 1.0)
		})
	}
}
