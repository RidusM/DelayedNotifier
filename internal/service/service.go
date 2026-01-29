package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"delayednotifier/internal/entity"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/dbpg/pgx-driver/transaction"
	"github.com/wb-go/wbf/logger"
	"github.com/wb-go/wbf/rabbitmq"
)

const (
	_slowOperationThreshold = 200 * time.Millisecond
	_defaultMaxRetries      = 3
	_defaultQueryLimit      = 10
	_defaultRetryDelay      = 5 * time.Minute
)

var (
	ErrNotificationNotFound    = errors.New("notification not found")
	ErrNotificationAlreadySent = errors.New("notification already sent")
	ErrNotificationCancelled   = errors.New("notification already cancelled")
)

type (
	NotifyRepository interface {
		Create(
			ctx context.Context,
			qe pgxdriver.QueryExecuter,
			notify entity.Notification,
		) (*entity.Notification, error)
		GetByID(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID) (*entity.Notification, error)
		GetForProcess(ctx context.Context, qe pgxdriver.QueryExecuter, limit uint64) ([]entity.Notification, error)
		UpdateStatus(
			ctx context.Context,
			qe pgxdriver.QueryExecuter,
			id uuid.UUID,
			status entity.Status,
			lastErr *string,
		) error
		RescheduleNotification(
			ctx context.Context,
			qe pgxdriver.QueryExecuter,
			id uuid.UUID,
			newScheduledAt time.Time,
		) error
		GetTelegramChatIDByUserID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID) (int64, error)
		GetUserEmailByUserID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID) (string, error)
	}

	CacheRepository interface {
		GetCacheKey(id uuid.UUID) string
		GetFromCache(ctx context.Context, key string) (*entity.Notification, error)
		SaveToCache(ctx context.Context, key string, notification *entity.Notification) error
		InvalidateCache(ctx context.Context, id uuid.UUID) error
	}

	NotificationSender interface {
		Send(ctx context.Context, notification entity.Notification) error
	}

	NotifyService struct {
		repo      NotifyRepository
		cache     CacheRepository
		sender    NotificationSender
		tm        transaction.Manager
		publisher *rabbitmq.Publisher
		log       logger.Logger

		queryLimit uint64
		maxRetries uint32
		retryDelay time.Duration
	}

	CreateNotificationRequest struct {
		UserID      uuid.UUID
		Channel     entity.Channel
		Payload     string
		ScheduledAt time.Time
	}

	ProcessingStats struct {
		Processed int
		Failed    int
		Duration  time.Duration
	}
)

func NewNotifyService(
	repo NotifyRepository,
	cache CacheRepository,
	sender NotificationSender,
	tm transaction.Manager,
	publisher *rabbitmq.Publisher,
	log logger.Logger,
	opts ...Option,
) (*NotifyService, error) {
	s := &NotifyService{
		repo:       repo,
		cache:      cache,
		sender:     sender,
		tm:         tm,
		publisher:  publisher,
		log:        log,
		maxRetries: _defaultMaxRetries,
		queryLimit: _defaultQueryLimit,
		retryDelay: _defaultRetryDelay,
	}

	for _, opt := range opts {
		opt(s)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("service.NewNotifyService: %w", err)
	}

	return s, nil
}

func (s *NotifyService) Create(ctx context.Context, req CreateNotificationRequest) (*entity.Notification, error) {
	const op = "service.NotifyService.Create"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]interface{}{
		"user_id": req.UserID.String(),
		"channel": req.Channel,
	})

	log.LogAttrs(ctx, logger.InfoLevel, "create notification started",
		logger.String("op", op),
		logger.String("user_id", req.UserID.String()),
		logger.String("channel", string(req.Channel)),
		logger.Time("scheduled_at", req.ScheduledAt),
	)

	if err := s.validateCreateRequest(req); err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "validation failed",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	scheduledAt := req.ScheduledAt
	if scheduledAt.Before(time.Now().UTC()) {
		scheduledAt = time.Now().UTC().Add(time.Minute)
	}

	recipientIdentifier, err := s.getRecipientIdentifier(ctx, req.UserID, req.Channel)
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get recipient identifier",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	notification := entity.Notification{
		UserID:              req.UserID,
		Channel:             req.Channel,
		Payload:             req.Payload,
		RecipientIdentifier: recipientIdentifier,
		ScheduledAt:         scheduledAt,
		Status:              entity.StatusWaiting,
	}

	var result *entity.Notification
	err = s.tm.ExecuteInTransaction(ctx, "create_notification", func(tx pgxdriver.QueryExecuter) error {
		created, txErr := s.repo.Create(ctx, tx, notification)
		if txErr != nil {
			return transaction.HandleError("create_notification", "create", txErr)
		}
		result = created
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "creation failed",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notification created",
		logger.String("op", op),
		logger.String("id", result.ID.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return result, nil
}

func (s *NotifyService) GetStatus(ctx context.Context, id uuid.UUID) (*entity.Notification, error) {
	const op = "service.NotifyService.GetStatus"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]any{
		"id": id.String(),
	})

	log.LogAttrs(ctx, logger.InfoLevel, "get status requested",
		logger.String("op", op),
		logger.String("id", id.String()),
	)

	cacheKey := s.cache.GetCacheKey(id)
	if cached, err := s.cache.GetFromCache(ctx, cacheKey); err == nil && cached != nil {
		log.LogAttrs(ctx, logger.InfoLevel, "served from cache",
			logger.String("op", op),
			logger.String("id", id.String()),
			logger.Duration("duration", time.Since(startTime)),
		)
		return cached, nil
	}

	var notification *entity.Notification
	err := s.tm.ExecuteInTransaction(ctx, "get_status", func(tx pgxdriver.QueryExecuter) error {
		var err error
		notification, err = s.repo.GetByID(ctx, tx, id)
		if err != nil {
			if errors.Is(err, entity.ErrDataNotFound) {
				return ErrNotificationNotFound
			}
			return fmt.Errorf("%s: %w", op, err)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get from database",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	_ = s.cache.SaveToCache(ctx, cacheKey, notification)

	log.LogAttrs(ctx, logger.InfoLevel, "served from database",
		logger.String("op", op),
		logger.String("id", id.String()),
		logger.String("status", notification.Status.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return notification, nil
}

func (s *NotifyService) Cancel(ctx context.Context, id uuid.UUID) error {
	const op = "service.NotifyService.Cancel"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]any{
		"id": id.String(),
	})

	log.LogAttrs(ctx, logger.InfoLevel, "cancel requested",
		logger.String("op", op),
		logger.String("id", id.String()),
	)

	err := s.tm.ExecuteInTransaction(ctx, "cancel_notification", func(tx pgxdriver.QueryExecuter) error {
		notification, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			if errors.Is(err, entity.ErrDataNotFound) {
				return ErrNotificationNotFound
			}
			return fmt.Errorf("%s: %w", op, err)
		}

		if notification.Status == entity.StatusSent {
			return ErrNotificationAlreadySent
		}
		if notification.Status == entity.StatusCancelled {
			return ErrNotificationCancelled
		}

		cancelReason := "cancelled by user"
		if upErr := s.repo.UpdateStatus(ctx, tx, id, entity.StatusCancelled, &cancelReason); upErr != nil {
			return transaction.HandleError("cancel_notification", "update_status", upErr)
		}

		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "cancel failed",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return fmt.Errorf("%s: %w", op, err)
	}

	_ = s.cache.InvalidateCache(ctx, id)

	log.LogAttrs(ctx, logger.InfoLevel, "notification cancelled",
		logger.String("op", op),
		logger.String("id", id.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return nil
}

func (s *NotifyService) ProcessQueue(ctx context.Context) (*ProcessingStats, error) {
	const op = "service.NotifyService.ProcessQueue"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	log.LogAttrs(ctx, logger.InfoLevel, "queue processing started",
		logger.String("op", op),
		logger.Uint64("query_limit", s.queryLimit),
	)

	stats := &ProcessingStats{}

	err := s.tm.ExecuteInTransaction(ctx, "process_queue", func(tx pgxdriver.QueryExecuter) error {
		notifications, err := s.repo.GetForProcess(ctx, tx, s.queryLimit)
		if err != nil {
			return transaction.HandleError("process_queue", "get_for_process", err)
		}

		if len(notifications) == 0 {
			log.LogAttrs(ctx, logger.DebugLevel, "no notifications to process",
				logger.String("op", op),
			)
			return nil
		}

		log.LogAttrs(ctx, logger.InfoLevel, "publish to queue",
			logger.String("op", op),
			logger.Int("count", len(notifications)),
		)

		for _, notification := range notifications {
			if pubErr := s.publishToQueue(ctx, tx, notification); pubErr != nil {
				log.LogAttrs(ctx, logger.ErrorLevel, "failed to publish",
					logger.String("op", op),
					logger.String("id", notification.ID.String()),
					logger.Any("error", pubErr),
				)
				stats.Failed++
				continue
			}
			stats.Processed++
		}

		return nil
	})

	stats.Duration = time.Since(startTime)

	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "queue processing failed",
			logger.String("op", op),
			logger.Any("error", err),
			logger.Duration("duration", stats.Duration),
		)
		return stats, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "queue processing completed",
		logger.String("op", op),
		logger.Int("processed", stats.Processed),
		logger.Int("failed", stats.Failed),
		logger.Duration("duration", stats.Duration),
	)

	return stats, nil
}

func (s *NotifyService) GetWorkerHandler() rabbitmq.MessageHandler {
	return func(ctx context.Context, msg amqp091.Delivery) error {
		const op = "service.NotifyService.WorkerHandler"

		log := s.log.Ctx(ctx)
		startTime := time.Now()

		var notification entity.Notification
		if err := json.Unmarshal(msg.Body, &notification); err != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "unmarshal failed",
				logger.String("op", op),
				logger.Any("error", err),
			)
			return fmt.Errorf("%s: unmarshal: %w", op, err)
		}

		log.LogAttrs(ctx, logger.InfoLevel, "processing notification",
			logger.String("op", op),
			logger.String("id", notification.ID.String()),
			logger.String("channel", string(notification.Channel)),
			logger.String("recipient", notification.RecipientIdentifier),
		)

		sendErr := s.sendNotification(ctx, notification)

		if err := s.updateAfterSend(ctx, notification.ID, sendErr); err != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "failed to update status",
				logger.String("op", op),
				logger.String("id", notification.ID.String()),
				logger.Any("error", err),
			)
			return err
		}

		if sendErr != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "send failed",
				logger.String("op", op),
				logger.String("id", notification.ID.String()),
				logger.Any("error", sendErr),
				logger.Duration("duration", time.Since(startTime)),
			)
			return sendErr
		}

		_ = s.cache.InvalidateCache(ctx, notification.ID)

		log.LogAttrs(ctx, logger.InfoLevel, "notification sent successfully",
			logger.String("op", op),
			logger.String("id", notification.ID.String()),
			logger.Duration("duration", time.Since(startTime)),
		)

		return nil
	}
}

func (s *NotifyService) validateCreateRequest(req CreateNotificationRequest) error {
	if req.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required: %w", entity.ErrInvalidData)
	}
	if req.Channel == "" {
		return fmt.Errorf("channel is required: %w", entity.ErrInvalidData)
	}
	if !req.Channel.IsValid() {
		return fmt.Errorf("invalid channel: %w", entity.ErrInvalidData)
	}
	if req.Payload == "" {
		return fmt.Errorf("payload is required: %w", entity.ErrInvalidData)
	}
	if req.ScheduledAt.IsZero() {
		return fmt.Errorf("scheduled_at is required: %w", entity.ErrInvalidData)
	}
	return nil
}

func (s *NotifyService) publishToQueue(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	notification entity.Notification,
) error {
	payload, err := json.Marshal(notification)
	if err != nil {
		errMsg := fmt.Sprintf("marshal error: %v", err)
		_ = s.repo.UpdateStatus(ctx, tx, notification.ID, entity.StatusFailed, &errMsg)
		return fmt.Errorf("marshal: %w", err)
	}

	routingKey := string(notification.Channel)
	if pubErr := s.publisher.Publish(ctx, payload, routingKey); pubErr != nil {
		errMsg := fmt.Sprintf("publish error: %v", pubErr)
		_ = s.repo.UpdateStatus(ctx, tx, notification.ID, entity.StatusFailed, &errMsg)
		return fmt.Errorf("publish: %w", pubErr)
	}

	if upErr := s.repo.UpdateStatus(ctx, tx, notification.ID, entity.StatusInProcess, nil); upErr != nil {
		return fmt.Errorf("update status: %w", upErr)
	}

	return nil
}

func (s *NotifyService) getRecipientIdentifier(
	ctx context.Context,
	userID uuid.UUID,
	channel entity.Channel,
) (string, error) {
	var recipientIdentifier string

	err := s.tm.ExecuteInTransaction(ctx, "get_recipient_identifier", func(tx pgxdriver.QueryExecuter) error {
		switch channel {
		case entity.Telegram:
			chatID, txErr := s.repo.GetTelegramChatIDByUserID(ctx, tx, userID)
			if txErr != nil {
				if errors.Is(txErr, entity.ErrDataNotFound) {
					return fmt.Errorf("telegram chat_id not found for user: %w", entity.ErrRecipientNotFound)
				}
				return fmt.Errorf("get telegram chat_id: %w", txErr)
			}
			recipientIdentifier = strconv.FormatInt(chatID, 10)

		case entity.Email:
			email, txErr := s.repo.GetUserEmailByUserID(ctx, tx, userID)
			if txErr != nil {
				if errors.Is(txErr, entity.ErrDataNotFound) {
					return fmt.Errorf("email not found for user: %w", entity.ErrRecipientNotFound)
				}
				return fmt.Errorf("get email: %w", txErr)
			}
			recipientIdentifier = email

		default:
			return fmt.Errorf("unknown channel %s: %w", channel, entity.ErrInvalidData)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("service.NotifyService.getRecipientIdentifier: %w", err)
	}
	return recipientIdentifier, nil
}

func (s *NotifyService) sendNotification(ctx context.Context, notification entity.Notification) error {
	if s.sender != nil {
		if err := s.sender.Send(ctx, notification); err != nil {
			return fmt.Errorf("service.NotifyService.sendNotification: %w", err)
		}
	}
	return nil
}

func (s *NotifyService) updateAfterSend(ctx context.Context, id uuid.UUID, sendErr error) error {
	const op = "service.NotifyService.updateAfterSend"

	log := s.log.Ctx(ctx)
	err := s.tm.ExecuteInTransaction(ctx, "update_after_send", func(tx pgxdriver.QueryExecuter) error {
		if sendErr == nil {
			return s.repo.UpdateStatus(ctx, tx, id, entity.StatusSent, nil)
		}

		return s.handleSendFailure(ctx, tx, id, sendErr)
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get recipient identifier",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *NotifyService) handleSendFailure(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	id uuid.UUID,
	sendErr error,
) error {
	notification, err := s.repo.GetByID(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("get notification: %w", err)
	}

	errMsg := sendErr.Error()
	if statusErr := s.repo.UpdateStatus(ctx, tx, id, entity.StatusFailed, &errMsg); statusErr != nil {
		return fmt.Errorf("update status to failed: %w", statusErr)
	}

	if notification.RetryCount >= s.maxRetries {
		return nil
	}

	return s.scheduleRetry(ctx, tx, id, notification.RetryCount)
}

func (s *NotifyService) scheduleRetry(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	id uuid.UUID,
	retryCount uint32,
) error {
	nextAttempt := s.calculateNextAttempt(retryCount)

	if err := s.repo.RescheduleNotification(ctx, tx, id, nextAttempt); err != nil {
		return fmt.Errorf("reschedule notification: %w", err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "notification rescheduled",
		logger.String("id", id.String()),
		logger.Uint32("retry_count", retryCount+1),
		logger.Time("next_attempt", nextAttempt),
	)

	return nil
}

func (s *NotifyService) calculateNextAttempt(retryCount uint32) time.Time {
	delay := s.retryDelay * time.Duration(1<<retryCount)
	return time.Now().UTC().Add(delay)
}

func (s *NotifyService) logSlowOperation(
	ctx context.Context,
	op string,
	startTime time.Time,
	fields map[string]any,
) {
	duration := time.Since(startTime)
	if duration > _slowOperationThreshold {
		attrs := []logger.Attr{
			logger.String("op", op),
			logger.Duration("duration", duration),
		}
		for k, v := range fields {
			attrs = append(attrs, logger.Any(k, v))
		}
		s.log.Ctx(ctx).LogAttrs(ctx, logger.WarnLevel, "slow operation detected", attrs...)
	}
}
