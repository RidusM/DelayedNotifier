package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"delayednotifier/internal/entity"
	mock_repository "delayednotifier/internal/repository/mock"
	"delayednotifier/internal/service"
	mock_sender "delayednotifier/internal/transport/sender/mock"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/logger"
	"github.com/wb-go/wbf/rabbitmq"
	"go.uber.org/mock/gomock"
)

type testEnv struct {
	svc        *service.NotifyService
	notifyRepo *mock_repository.MockNotifyRepository
	teleRepo   *mock_repository.MockTelegramRepository
	cacheRepo  *mock_repository.MockCacheRepository
	sender     *mock_sender.MockNotificationSender
	ctrl       *gomock.Controller
}

type MockTM struct{}

func (m *MockTM) ExecuteInTransaction(_ context.Context, _ string, fn func(pgxdriver.QueryExecuter) error) error {
	return fn(nil)
}

type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) Publish(
	ctx context.Context,
	body []byte,
	routingKey string,
	_ ...rabbitmq.PublishOption,
) error {
	args := m.Called(ctx, body, routingKey)
	return args.Error(0)
}

func (m *MockPublisher) GetExchangeName() string {
	args := m.Called()
	return args.String(0)
}

func setup(t *testing.T) *testEnv {
	t.Helper()

	ctrl := gomock.NewController(t)
	notifyRepo := mock_repository.NewMockNotifyRepository(ctrl)
	teleRepo := mock_repository.NewMockTelegramRepository(ctrl)
	cacheRepo := mock_repository.NewMockCacheRepository(ctrl)
	sender := mock_sender.NewMockNotificationSender(ctrl)

	log, err := logger.NewZapAdapter("test-app-notify", "test", logger.WithLevel(logger.ErrorLevel))
	require.NoError(t, err)

	svc, err := service.NewNotifyService(
		notifyRepo,
		teleRepo,
		cacheRepo,
		sender,
		&MockTM{},
		&MockPublisher{},
		log,
	)
	if err != nil {
		t.Fatalf("failed to init service: %v", err)
	}

	return &testEnv{
		svc:        svc,
		notifyRepo: notifyRepo,
		teleRepo:   teleRepo,
		cacheRepo:  cacheRepo,
		sender:     sender,
		ctrl:       ctrl,
	}
}

func TestCreate_Success_Email(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	env.notifyRepo.EXPECT().Create(ctx, nil, gomock.Any()).Return(nil)

	id, err := env.svc.Create(ctx, service.CreateNotificationRequest{
		UserID:      uuid.New(),
		Channel:     entity.Email,
		Payload:     "test",
		Recipient:   "test@example.com",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}

func TestCreate_Success_Telegram(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	env.notifyRepo.EXPECT().Create(ctx, nil, gomock.Any()).Return(nil)

	id, err := env.svc.Create(ctx, service.CreateNotificationRequest{
		Channel:     entity.Telegram,
		Payload:     "test",
		Recipient:   "123456789",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}

func TestCreate_InvalidEmail(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     "test",
		Recipient:   "invalid-email",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid email format")
}

func TestCreate_InvalidTelegramID(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Telegram,
		Payload:     "test",
		Recipient:   "invalid-telegram-id",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid telegram ID format")
}

func TestCreate_ChannelRequired(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Recipient:   "test@example.com",
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel is required")
}

func TestCreate_PayloadRequired(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Email,
		Recipient:   "test@example.com",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload is required")
}

func TestCreate_ScheduledAtRequired(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:   entity.Email,
		Payload:   "test",
		Recipient: "test@example.com",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduled_at is required")
}

func TestCreate_WithoutUserID(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()
	ctx := context.Background()

	env.notifyRepo.EXPECT().Create(ctx, nil, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ pgxdriver.QueryExecuter, notify entity.Notification) error {
			assert.NotEqual(t, uuid.Nil, notify.ID)
			return nil
		}).Times(1)

	id, err := env.svc.Create(ctx, service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     "test",
		Recipient:   "test@example.com",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}

func TestCreate_MustBeFuture(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     "test",
		Recipient:   "test@example.com",
		ScheduledAt: time.Now().Add(-1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in future")
}

func TestCreate_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     string(make([]byte, 100001)),
		Recipient:   "test@example.com",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestCreate_RecipientRequired(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     "test",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipient is required")
}

func TestCreate_RepositoryError(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	dbErr := errors.New("db error")
	env.notifyRepo.EXPECT().Create(gomock.Any(), nil, gomock.Any()).Return(dbErr)

	_, err := env.svc.Create(context.Background(), service.CreateNotificationRequest{
		Channel:     entity.Email,
		Payload:     "test",
		Recipient:   "test@example.com",
		ScheduledAt: time.Now().Add(1 * time.Hour),
	})

	require.ErrorIs(t, err, dbErr)
}

func TestGetStatus_FromCache(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	id := uuid.New()
	cached := &entity.Notification{ID: id, Status: entity.StatusWaiting}
	env.cacheRepo.EXPECT().GetCacheKey(id).Return("key")
	env.cacheRepo.EXPECT().GetFromCache(gomock.Any(), "key").Return(cached, nil)

	result, err := env.svc.GetStatus(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, cached, result)
}

func TestGetStatus_FromDB(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	key := "key"
	notif := &entity.Notification{ID: id, Status: entity.StatusSent}
	env.cacheRepo.EXPECT().GetCacheKey(id).Return(key)
	env.cacheRepo.EXPECT().GetFromCache(ctx, key).Return(nil, errors.New("miss"))
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, false).Return(notif, nil)
	env.cacheRepo.EXPECT().SaveToCache(ctx, key, notif).Return(nil)

	result, err := env.svc.GetStatus(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, notif, result)
}

func TestGetStatus_NotFound(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	key := "key"
	env.cacheRepo.EXPECT().GetCacheKey(id).Return(key)
	env.cacheRepo.EXPECT().GetFromCache(ctx, key).Return(nil, errors.New("miss"))
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, false).Return(nil, entity.ErrDataNotFound)

	_, err := env.svc.GetStatus(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, entity.ErrNotificationNotFound)
}

func TestCancel_Success(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	id := uuid.New()
	notif := &entity.Notification{ID: id, Status: entity.StatusWaiting}
	env.notifyRepo.EXPECT().GetByID(gomock.Any(), nil, id, true).Return(notif, nil)
	env.notifyRepo.EXPECT().UpdateStatus(gomock.Any(), nil, id, entity.StatusCancelled, gomock.Any()).Return(nil)
	env.cacheRepo.EXPECT().InvalidateCache(gomock.Any(), id).Return(nil)

	err := env.svc.Cancel(context.Background(), id)
	require.NoError(t, err)
}

func TestCancel_NotFound(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(nil, entity.ErrDataNotFound)

	err := env.svc.Cancel(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, entity.ErrNotificationNotFound)
}

func TestCancel_AlreadySent(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	id := uuid.New()
	notif := &entity.Notification{ID: id, Status: entity.StatusSent}
	env.notifyRepo.EXPECT().GetByID(gomock.Any(), nil, id, true).Return(notif, nil)

	err := env.svc.Cancel(context.Background(), id)
	require.ErrorIs(t, err, entity.ErrNotificationAlreadySent)
}

func TestCancel_AlreadyCancelled(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	notif := &entity.Notification{ID: id, Status: entity.StatusCancelled}
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(notif, nil)

	err := env.svc.Cancel(ctx, id)
	require.Error(t, err)
	assert.ErrorIs(t, err, entity.ErrNotificationCancelled)
}

func TestProcessQueue_NoNotifications(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	env.notifyRepo.EXPECT().GetForProcess(ctx, nil, uint64(10)).Return([]entity.Notification{}, nil)
	stats, err := env.svc.ProcessQueue(ctx)

	require.NoError(t, err)
	assert.Equal(t, 0, stats.Processed)
	assert.Equal(t, 0, stats.Failed)
}

func TestProcessQueue_RepositoryError(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	dbErr := errors.New("db error")
	env.notifyRepo.EXPECT().GetForProcess(ctx, nil, uint64(10)).Return(nil, dbErr)

	stats, err := env.svc.ProcessQueue(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, dbErr)
	assert.Equal(t, 0, stats.Processed)
}

func TestWorkerHandler_Success(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	id := uuid.New()
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess, Channel: entity.Email}

	body, err := json.Marshal(notif)
	require.NoError(t, err)

	delivery := amqp091.Delivery{Body: body}
	env.notifyRepo.EXPECT().GetByID(gomock.Any(), nil, id, true).Return(&notif, nil)
	env.sender.EXPECT().Send(gomock.Any(), notif).Return(nil)
	env.notifyRepo.EXPECT().UpdateStatus(gomock.Any(), nil, id, entity.StatusSent, nil).Return(nil)
	env.cacheRepo.EXPECT().InvalidateCache(gomock.Any(), id).Return(nil)
	handler := env.svc.GetWorkerHandler()

	err = handler(context.Background(), delivery)
	require.NoError(t, err)
}

func TestWorkerHandler_NotFound(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	notif := entity.Notification{ID: id}

	body, err := json.Marshal(notif)
	require.NoError(t, err)

	delivery := amqp091.Delivery{Body: body}
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(nil, entity.ErrDataNotFound)
	handler := env.svc.GetWorkerHandler()

	err = handler(ctx, delivery)
	require.NoError(t, err)
}

func TestWorkerHandler_StatusChanged(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess}
	current := &entity.Notification{ID: id, Status: entity.StatusCancelled}

	body, err := json.Marshal(notif)
	require.NoError(t, err)

	delivery := amqp091.Delivery{Body: body}
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(current, nil)
	handler := env.svc.GetWorkerHandler()

	err = handler(ctx, delivery)
	require.NoError(t, err)
}

func TestWorkerHandler_SendFailed_WithRetry(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	id := uuid.New()
	sendErr := errors.New("send failed")
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess, RetryCount: 0}

	body, err := json.Marshal(notif)
	require.NoError(t, err)

	delivery := amqp091.Delivery{Body: body}

	env.notifyRepo.EXPECT().GetByID(gomock.Any(), nil, id, true).Return(&notif, nil).Times(1)
	env.sender.EXPECT().Send(gomock.Any(), notif).Return(sendErr)
	env.notifyRepo.EXPECT().UpdateStatus(gomock.Any(), nil, id, entity.StatusFailed, gomock.Any()).Return(nil)
	env.notifyRepo.EXPECT().RescheduleNotification(gomock.Any(), nil, id, gomock.Any()).Return(nil)
	env.cacheRepo.EXPECT().InvalidateCache(gomock.Any(), id).Return(nil)
	handler := env.svc.GetWorkerHandler()

	err = handler(context.Background(), delivery)
	require.Error(t, err)
}

func TestWorkerHandler_MaxRetriesReached(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	id := uuid.New()
	sendErr := errors.New("send failed")
	notif := entity.Notification{ID: id, Status: entity.StatusInProcess, RetryCount: 3}

	body, err := json.Marshal(notif)
	require.NoError(t, err)

	delivery := amqp091.Delivery{Body: body}
	env.notifyRepo.EXPECT().GetByID(ctx, nil, id, true).Return(&notif, nil).Times(1)
	env.sender.EXPECT().Send(ctx, notif).Return(sendErr)
	env.notifyRepo.EXPECT().UpdateStatus(ctx, nil, id, entity.StatusFailed, gomock.Any()).Return(nil)
	env.cacheRepo.EXPECT().InvalidateCache(ctx, id).Return(nil)
	handler := env.svc.GetWorkerHandler()

	err = handler(ctx, delivery)
	require.Error(t, err)
}

func TestWorkerHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer env.ctrl.Finish()

	ctx := context.Background()
	delivery := amqp091.Delivery{Body: []byte("invalid")}
	handler := env.svc.GetWorkerHandler()

	err := handler(ctx, delivery)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestCalculateNextAttempt(t *testing.T) {
	t.Parallel()
	env := setup(t)
	defer t.Cleanup(func() {
		env.ctrl.Finish()
	})

	tests := []struct {
		retry int
		delay time.Duration
	}{
		{-1, 5 * time.Minute},
		{0, 5 * time.Minute},
		{1, 10 * time.Minute},
		{2, 20 * time.Minute},
		{10, 5 * time.Minute * 64},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("retry_%d", tt.retry), func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC()
			next := env.svc.CalculateNextAttempt(tt.retry)
			delta := next.Sub(now)

			assert.InDelta(t, tt.delay.Seconds(), delta.Seconds(), 2.0)
		})
	}
}
