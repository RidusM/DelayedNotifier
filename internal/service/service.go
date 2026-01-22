package service

import (
	"context"
	"delayednotifier/internal/entity"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/dbpg/pgx-driver/transaction"
	"github.com/wb-go/wbf/logger"
	"github.com/wb-go/wbf/rabbitmq"
	"github.com/wb-go/wbf/redis"
)

const (
	_cacheKeyPrefix         = "notify:"
	_cacheTTL               = 5 * time.Minute
	_slowOperationThreshold = 200 * time.Millisecond
	_defaultBatchSize       = 50
	_maxBatchSize           = 100
	_maxRetries             = 3
	_baseRetryDelay         = 5 * time.Minute
	_defaultCleanupAge      = 30 * 24 * time.Hour
)

var (
	ErrNotificationNotFound    = errors.New("notification not found")
	ErrNotificationAlreadySent = errors.New("notification already sent")
	ErrNotificationCancelled   = errors.New("notification already cancelled")
	ErrEmptyBatch              = errors.New("batch cannot be empty")
)

type (
	// NotifyRepository определяет интерфейс для работы с уведомлениями в БД
	NotifyRepository interface {
		Create(ctx context.Context, qe pgxdriver.QueryExecuter, notify entity.Notification) (*entity.Notification, error)
		GetByID(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID) (*entity.Notification, error)
		GetForProcess(ctx context.Context, qe pgxdriver.QueryExecuter, limit uint64) ([]entity.Notification, error)
		UpdateStatus(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID, status entity.Status, lastErr *string) error
		RescheduleNotification(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID, newScheduledAt time.Time) error
		GetTelegramChatIDByUserID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID) (int64, error)
		GetUserEmailByUserID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID) (string, error)
	}

	// NotificationSender определяет интерфейс для отправки уведомлений
	NotificationSender interface {
		Send(ctx context.Context, notification entity.Notification) error
	}

	// NotifyService предоставляет бизнес-логику для работы с уведомлениями
	NotifyService struct {
		repo      NotifyRepository
		tm        transaction.Manager
		db        *pgxdriver.Postgres
		publisher *rabbitmq.Publisher
		cache     redis.Client
		sender    NotificationSender
		log       logger.Logger

		batchSize      uint64
		maxRetries     int
		baseRetryDelay time.Duration
		cleanupAge     time.Duration
	}

	// CreateNotificationRequest представляет запрос на создание уведомления
	CreateNotificationRequest struct {
		UserID      uuid.UUID
		Channel     entity.Channel
		Payload     string
		ScheduledAt time.Time
	}

	// ProcessingStats содержит статистику обработки уведомлений
	ProcessingStats struct {
		Processed int
		Failed    int
		Duration  time.Duration
	}
)

// NewNotifyService создает новый экземпляр сервиса уведомлений
func NewNotifyService(
	repo NotifyRepository,
	tm transaction.Manager,
	db *pgxdriver.Postgres,
	publisher *rabbitmq.Publisher,
	cache redis.Client,
	log logger.Logger,
	opts ...Option,
) *NotifyService {
	s := &NotifyService{
		repo:           repo,
		tm:             tm,
		db:             db,
		publisher:      publisher,
		cache:          cache,
		log:            log,
		batchSize:      _defaultBatchSize,
		maxRetries:     _maxRetries,
		baseRetryDelay: _baseRetryDelay,
		cleanupAge:     _defaultCleanupAge,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Create создает новое уведомление с автоматическим получением recipient_identifier
func (s *NotifyService) Create(ctx context.Context, req CreateNotificationRequest) (*entity.Notification, error) {
	const op = "service.NotifyService.Create"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]interface{}{
		"user_id": req.UserID.String(),
		"channel": string(req.Channel),
	})

	log.LogAttrs(ctx, logger.InfoLevel, "create notification started",
		logger.String("op", op),
		logger.String("user_id", req.UserID.String()),
		logger.String("channel", string(req.Channel)),
		logger.Time("scheduled_at", req.ScheduledAt),
	)

	// Валидация
	if err := s.validateCreateRequest(req); err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "validation failed",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Автоматическая корректировка времени
	scheduledAt := req.ScheduledAt
	if scheduledAt.Before(time.Now().UTC()) {
		scheduledAt = time.Now().UTC().Add(time.Minute)
		log.LogAttrs(ctx, logger.DebugLevel, "scheduled_at adjusted to future",
			logger.Time("original", req.ScheduledAt),
			logger.Time("adjusted", scheduledAt),
		)
	}

	// Получаем recipient_identifier в зависимости от канала
	var recipientIdentifier string
	var err error

	err = s.tm.ExecuteInTransaction(ctx, "get_recipient_identifier", func(tx pgxdriver.QueryExecuter) error {
		switch req.Channel {
		case entity.Telegram:
			chatID, err := s.repo.GetTelegramChatIDByUserID(ctx, tx, req.UserID)
			if err != nil {
				if errors.Is(err, entity.ErrDataNotFound) {
					return fmt.Errorf("telegram chat_id not found for user: %w", entity.ErrRecipientNotFound)
				}
				return fmt.Errorf("get telegram chat_id: %w", err)
			}
			recipientIdentifier = fmt.Sprintf("%d", chatID)

		case entity.Email:
			email, err := s.repo.GetUserEmailByUserID(ctx, tx, req.UserID)
			if err != nil {
				if errors.Is(err, entity.ErrDataNotFound) {
					return fmt.Errorf("email not found for user: %w", entity.ErrRecipientNotFound)
				}
				return fmt.Errorf("get email: %w", err)
			}
			recipientIdentifier = email

		default:
			return fmt.Errorf("unknown channel %s: %w", req.Channel, entity.ErrInvalidData)
		}
		return nil
	})

	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get recipient identifier",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.DebugLevel, "recipient identifier resolved",
		logger.String("channel", string(req.Channel)),
		logger.String("recipient", recipientIdentifier),
	)

	// Создаем уведомление
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
		created, err := s.repo.Create(ctx, tx, notification)
		if err != nil {
			return transaction.HandleError("create_notification", "create", err)
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

// GetStatus возвращает статус уведомления с кешированием
func (s *NotifyService) GetStatus(ctx context.Context, id uuid.UUID) (*entity.Notification, error) {
	const op = "service.NotifyService.GetStatus"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]interface{}{
		"id": id.String(),
	})

	log.LogAttrs(ctx, logger.InfoLevel, "get status requested",
		logger.String("op", op),
		logger.String("id", id.String()),
	)

	// Проверяем кеш
	cacheKey := s.getCacheKey(id)
	if cached, err := s.getFromCache(ctx, cacheKey); err == nil && cached != nil {
		log.LogAttrs(ctx, logger.InfoLevel, "served from cache",
			logger.String("op", op),
			logger.String("id", id.String()),
			logger.Duration("duration", time.Since(startTime)),
		)
		return cached, nil
	}

	// Получаем из БД
	notification, err := s.repo.GetByID(ctx, s.db, id)
	if err != nil {
		if errors.Is(err, entity.ErrDataNotFound) {
			return nil, fmt.Errorf("%s: %w", op, ErrNotificationNotFound)
		}
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get from database",
			logger.String("op", op),
			logger.Any("error", err),
		)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Сохраняем в кеш
	_ = s.saveToCache(ctx, cacheKey, notification)

	log.LogAttrs(ctx, logger.InfoLevel, "served from database",
		logger.String("op", op),
		logger.String("id", id.String()),
		logger.String("status", notification.Status.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return notification, nil
}

// Cancel отменяет уведомление
func (s *NotifyService) Cancel(ctx context.Context, id uuid.UUID) error {
	const op = "service.NotifyService.Cancel"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]interface{}{
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
			return err
		}

		if notification.Status == entity.StatusSent {
			return ErrNotificationAlreadySent
		}
		if notification.Status == entity.StatusCancelled {
			return ErrNotificationCancelled
		}

		cancelReason := "cancelled by user"
		if err := s.repo.UpdateStatus(ctx, tx, id, entity.StatusCancelled, &cancelReason); err != nil {
			return transaction.HandleError("cancel_notification", "update_status", err)
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

	// Инвалидируем кеш
	_ = s.invalidateCache(ctx, id)

	log.LogAttrs(ctx, logger.InfoLevel, "notification cancelled",
		logger.String("op", op),
		logger.String("id", id.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return nil
}

// ProcessQueue обрабатывает очередь готовых к отправке уведомлений
func (s *NotifyService) ProcessQueue(ctx context.Context) (*ProcessingStats, error) {
	const op = "service.NotifyService.ProcessQueue"

	log := s.log.Ctx(ctx)
	startTime := time.Now()

	log.LogAttrs(ctx, logger.InfoLevel, "queue processing started",
		logger.String("op", op),
		logger.Uint64("batch_size", s.batchSize),
	)

	stats := &ProcessingStats{}

	err := s.tm.ExecuteInTransaction(ctx, "process_queue", func(tx pgxdriver.QueryExecuter) error {
		notifications, err := s.repo.GetForProcess(ctx, tx, s.batchSize)
		if err != nil {
			return transaction.HandleError("process_queue", "get_for_process", err)
		}

		if len(notifications) == 0 {
			log.LogAttrs(ctx, logger.DebugLevel, "no notifications to process",
				logger.String("op", op),
			)
			return nil
		}

		log.LogAttrs(ctx, logger.InfoLevel, "processing batch",
			logger.String("op", op),
			logger.Int("count", len(notifications)),
		)

		for _, notification := range notifications {
			if err := s.publishToQueue(ctx, tx, notification); err != nil {
				log.LogAttrs(ctx, logger.ErrorLevel, "failed to publish",
					logger.String("op", op),
					logger.String("id", notification.ID.String()),
					logger.Any("error", err),
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

// GetWorkerHandler возвращает обработчик сообщений для RabbitMQ воркера
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

		// Отправляем уведомление через соответствующий sender
		sendErr := s.sendNotification(ctx, notification)

		// Обновляем статус
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

		// Инвалидируем кеш
		_ = s.invalidateCache(ctx, notification.ID)

		log.LogAttrs(ctx, logger.InfoLevel, "notification sent successfully",
			logger.String("op", op),
			logger.String("id", notification.ID.String()),
			logger.Duration("duration", time.Since(startTime)),
		)

		return nil
	}
}

// Приватные helper методы

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

func (s *NotifyService) publishToQueue(ctx context.Context, tx pgxdriver.QueryExecuter, notification entity.Notification) error {
	payload, err := json.Marshal(notification)
	if err != nil {
		errMsg := fmt.Sprintf("marshal error: %v", err)
		_ = s.repo.UpdateStatus(ctx, tx, notification.ID, entity.StatusFailed, &errMsg)
		return fmt.Errorf("marshal: %w", err)
	}

	routingKey := string(notification.Channel)
	if err := s.publisher.Publish(ctx, payload, routingKey); err != nil {
		errMsg := fmt.Sprintf("publish error: %v", err)
		_ = s.repo.UpdateStatus(ctx, tx, notification.ID, entity.StatusFailed, &errMsg)
		return fmt.Errorf("publish: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, tx, notification.ID, entity.StatusInProcess, nil); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	return nil
}

func (s *NotifyService) sendNotification(ctx context.Context, notification entity.Notification) error {
	if s.sender != nil {
		return s.sender.Send(ctx, notification)
	}

	// Fallback: заглушка для тестирования без sender
	select {
	case <-time.After(500 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *NotifyService) updateAfterSend(ctx context.Context, id uuid.UUID, sendErr error) error {
	return s.tm.ExecuteInTransaction(ctx, "update_after_send", func(tx pgxdriver.QueryExecuter) error {
		if sendErr != nil {
			notification, err := s.repo.GetByID(ctx, tx, id)
			if err != nil {
				return err
			}

			errMsg := sendErr.Error()
			if err := s.repo.UpdateStatus(ctx, tx, id, entity.StatusFailed, &errMsg); err != nil {
				return err
			}

			// Retry logic с exponential backoff
			if notification.RetryCount < s.maxRetries {
				nextAttempt := s.calculateNextAttempt(notification.RetryCount)
				if err := s.repo.RescheduleNotification(ctx, tx, id, nextAttempt); err != nil {
					return err
				}

				s.log.LogAttrs(ctx, logger.InfoLevel, "notification rescheduled",
					logger.String("id", id.String()),
					logger.Int("retry_count", notification.RetryCount+1),
					logger.Time("next_attempt", nextAttempt),
				)
			}
			return nil
		}

		return s.repo.UpdateStatus(ctx, tx, id, entity.StatusSent, nil)
	})
}

func (s *NotifyService) calculateNextAttempt(retryCount int) time.Time {
	delay := s.baseRetryDelay * time.Duration(1<<retryCount) // 5m, 10m, 20m, ...
	return time.Now().UTC().Add(delay)
}

// Cache helpers

func (s *NotifyService) getCacheKey(id uuid.UUID) string {
	return _cacheKeyPrefix + id.String()
}

func (s *NotifyService) getFromCache(ctx context.Context, key string) (*entity.Notification, error) {
	cached, err := s.cache.Get(ctx, key)
	if err != nil || cached == "" {
		return nil, err
	}

	var notification entity.Notification
	if err := json.Unmarshal([]byte(cached), &notification); err != nil {
		return nil, err
	}

	return &notification, nil
}

func (s *NotifyService) saveToCache(ctx context.Context, key string, notification *entity.Notification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	return s.cache.SetWithExpiration(ctx, key, data, _cacheTTL)
}

func (s *NotifyService) invalidateCache(ctx context.Context, id uuid.UUID) error {
	return s.cache.Del(ctx, s.getCacheKey(id))
}

// Logging helpers

func (s *NotifyService) logSlowOperation(ctx context.Context, op string, startTime time.Time, fields map[string]interface{}) {
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
