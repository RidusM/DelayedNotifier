package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
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

	_maxPayloadSize   = 100_000
	_defaultTimeout   = 2 * time.Second
	_maxRetryExponent = 6
)

type (
	NotifyRepository interface {
		Create(
			ctx context.Context,
			qe pgxdriver.QueryExecuter,
			notify entity.Notification,
		) error

		GetByID(
			ctx context.Context,
			qe pgxdriver.QueryExecuter,
			id uuid.UUID,
			forUpdate bool,
		) (*entity.Notification, error)

		GetForProcess(
			ctx context.Context,
			qe pgxdriver.QueryExecuter,
			limit uint64,
		) ([]entity.Notification, error)

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
	}

	TelegramRepository interface {
		SaveSubscriber(
			ctx context.Context,
			username string,
			chatID int64,
		) error
		GetChatID(
			ctx context.Context,
			username string,
		) (int64, error)
	}

	CacheRepository interface {
		GetCacheKey(
			id uuid.UUID,
		) string
		GetFromCache(
			ctx context.Context,
			key string,
		) (*entity.Notification, error)
		SaveToCache(
			ctx context.Context,
			key string,
			notification *entity.Notification,
		) error
		InvalidateCache(
			ctx context.Context,
			id uuid.UUID,
		) error
	}

	NotificationSender interface {
		Send(
			ctx context.Context,
			notification entity.Notification,
		) error
	}

	PublisherInterface interface {
		Publish(
			ctx context.Context,
			body []byte,
			routingKey string,
			opts ...rabbitmq.PublishOption,
		) error
		GetExchangeName() string
	}

	NotifyService struct {
		notifyRepo NotifyRepository
		teleRepo   TelegramRepository
		cache      CacheRepository
		sender     NotificationSender
		tm         transaction.Manager
		publisher  PublisherInterface
		log        logger.Logger

		queryLimit uint64
		maxRetries int
		retryDelay time.Duration
	}

	CreateNotificationRequest struct {
		ID          uuid.UUID
		UserID      uuid.UUID
		Channel     entity.Channel
		Payload     string
		Recipient   string
		ScheduledAt time.Time
	}

	ProcessingStats struct {
		Processed int
		Failed    int
		Duration  time.Duration
	}
)

func NewNotifyService(
	notifyRepo NotifyRepository,
	teleRepo TelegramRepository,
	cache CacheRepository,
	sender NotificationSender,
	tm transaction.Manager,
	publisher PublisherInterface,
	log logger.Logger,
	opts ...Option,
) (*NotifyService, error) {
	s := &NotifyService{
		notifyRepo: notifyRepo,
		teleRepo:   teleRepo,
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
		return nil, fmt.Errorf("service.notify.NewNotifyService: %w", err)
	}

	return s, nil
}

func (s *NotifyService) Create(ctx context.Context, req CreateNotificationRequest) (uuid.UUID, error) {
	const op = "service.notify.Create"

	log := s.log.Ctx(ctx).With("op", op)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]any{
		"user_id": req.UserID.String(),
		"channel": req.Channel,
	})

	log.LogAttrs(ctx, logger.InfoLevel, "create notification started",
		logger.String("user_id", req.UserID.String()),
		logger.String("channel", string(req.Channel)),
		logger.Time("scheduled_at", req.ScheduledAt),
	)

	var err error
	req.ID, err = uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s: v7 uuid: %w", op, err)
	}

	if validErr := s.validateCreateRequest(req); validErr != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "validation failed",
			logger.Any("error", validErr),
		)
		return uuid.Nil, fmt.Errorf("%s: %w", op, validErr)
	}

	scheduledAt := req.ScheduledAt
	if scheduledAt.Before(time.Now().UTC()) {
		return uuid.Nil, fmt.Errorf("scheduled time must be in future: %w", entity.ErrInvalidData)
	}

	notification := entity.Notification{
		ID:                  req.ID,
		Channel:             req.Channel,
		Payload:             req.Payload,
		RecipientIdentifier: req.Recipient,
		ScheduledAt:         scheduledAt,
		Status:              entity.StatusWaiting,
		CreatedAt:           time.Now().UTC(),
	}
	err = s.tm.ExecuteInTransaction(ctx, "create_notification", func(tx pgxdriver.QueryExecuter) error {
		txErr := s.notifyRepo.Create(ctx, tx, notification)
		if txErr != nil {
			return transaction.HandleError("create_notification", "create", txErr)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "creation failed",
			logger.Any("error", err),
		)
		return uuid.Nil, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notification created",
		logger.Duration("duration", time.Since(startTime)),
	)

	return req.ID, nil
}

func (s *NotifyService) GetStatus(ctx context.Context, id uuid.UUID) (*entity.Notification, error) {
	const op = "service.notify.GetStatus"

	log := s.log.Ctx(ctx).With("op", op)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]any{
		"id": id.String(),
	})

	log.LogAttrs(ctx, logger.DebugLevel, "get status requested",
		logger.String("id", id.String()),
	)

	cacheKey := s.cache.GetCacheKey(id)
	if cached, err := s.cache.GetFromCache(ctx, cacheKey); err == nil && cached != nil {
		log.LogAttrs(ctx, logger.DebugLevel, "served from cache",
			logger.String("id", id.String()),
			logger.Duration("duration", time.Since(startTime)),
		)
		return cached, nil
	}

	notification, err := s.notifyRepo.GetByID(ctx, nil, id, false)
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get from database",
			logger.Any("error", err),
		)
		if errors.Is(err, entity.ErrDataNotFound) {
			return nil, entity.ErrNotificationNotFound
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	_ = s.cache.SaveToCache(ctx, cacheKey, notification)

	log.LogAttrs(ctx, logger.InfoLevel, "served from database",
		logger.String("id", id.String()),
		logger.String("status", notification.Status.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return notification, nil
}

func (s *NotifyService) Cancel(ctx context.Context, id uuid.UUID) error {
	const op = "service.notify.Cancel"

	log := s.log.Ctx(ctx).With("op", op)
	startTime := time.Now()

	defer s.logSlowOperation(ctx, op, startTime, map[string]any{
		"id": id.String(),
	})

	log.LogAttrs(ctx, logger.InfoLevel, "cancel requested",
		logger.String("id", id.String()),
	)

	err := s.tm.ExecuteInTransaction(ctx, "cancel_notification", func(tx pgxdriver.QueryExecuter) error {
		notification, err := s.notifyRepo.GetByID(ctx, tx, id, true)
		if err != nil {
			if errors.Is(err, entity.ErrDataNotFound) {
				return entity.ErrNotificationNotFound
			}
			return fmt.Errorf("%s: %w", op, err)
		}

		if notification.Status == entity.StatusSent {
			return entity.ErrNotificationAlreadySent
		}
		if notification.Status == entity.StatusCancelled {
			return entity.ErrNotificationCancelled
		}

		cancelReason := "cancelled by user"
		if upErr := s.notifyRepo.UpdateStatus(ctx, tx, id, entity.StatusCancelled, &cancelReason); upErr != nil {
			return transaction.HandleError("cancel_notification", "update_status", upErr)
		}

		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "cancel failed",
			logger.Any("error", err),
		)
		return fmt.Errorf("%s: %w", op, err)
	}

	_ = s.cache.InvalidateCache(ctx, id)

	log.LogAttrs(ctx, logger.InfoLevel, "notification cancelled",
		logger.String("id", id.String()),
		logger.Duration("duration", time.Since(startTime)),
	)

	return nil
}

func (s *NotifyService) ProcessQueue(ctx context.Context) (*ProcessingStats, error) {
	const op = "service.notify.ProcessQueue"

	log := s.log.Ctx(ctx).With("op", op)
	startTime := time.Now()
	stats := &ProcessingStats{}

	notifications, err := s.notifyRepo.GetForProcess(ctx, nil, s.queryLimit)
	if err != nil {
		return stats, fmt.Errorf("%s: get for process: %w", op, err)
	}

	if len(notifications) == 0 {
		log.LogAttrs(ctx, logger.DebugLevel, "no notifications to process")
		return stats, nil
	}

	log.LogAttrs(ctx, logger.InfoLevel, "processing notifications",
		logger.Int("count", len(notifications)),
	)

	for _, n := range notifications {
		err = s.tm.ExecuteInTransaction(ctx, "mark_in_process", func(tx pgxdriver.QueryExecuter) error {
			return s.notifyRepo.UpdateStatus(ctx, tx, n.ID, entity.StatusInProcess, nil)
		})
		if err != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "failed to mark notification as in_process",
				logger.String("id", n.ID.String()),
				logger.Any("error", err),
			)
			stats.Failed++
			continue
		}

		if resolveErr := s.resolveRecipient(ctx, &n); resolveErr != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "failed to resolve recipient",
				logger.String("id", n.ID.String()),
				logger.Any("error", resolveErr),
			)
			stats.Failed++
			continue
		}

		if pubErr := s.publishToQueue(ctx, n); pubErr != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "failed to publish to queue",
				logger.String("id", n.ID.String()),
				logger.Any("error", pubErr),
			)
			_ = s.tm.ExecuteInTransaction(ctx, "rollback_to_waiting", func(tx pgxdriver.QueryExecuter) error {
				return s.notifyRepo.UpdateStatus(ctx, tx, n.ID, entity.StatusWaiting, nil)
			})
			stats.Failed++
			continue
		}

		stats.Processed++
	}

	stats.Duration = time.Since(startTime)

	log.LogAttrs(ctx, logger.InfoLevel, "queue processing completed",
		logger.Int("processed", stats.Processed),
		logger.Int("failed", stats.Failed),
		logger.Duration("duration", stats.Duration),
	)

	return stats, nil
}

func (s *NotifyService) GetWorkerHandler() rabbitmq.MessageHandler {
	return func(ctx context.Context, msg amqp091.Delivery) error {
		const op = "service.notify.WorkerHandler"

		log := s.log.Ctx(ctx).With("op", op)
		startTime := time.Now()

		var notification entity.Notification
		if err := json.Unmarshal(msg.Body, &notification); err != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "unmarshal failed",
				logger.Any("error", err),
			)
			return fmt.Errorf("%s: unmarshal: %w", op, err)
		}

		log.LogAttrs(ctx, logger.InfoLevel, "processing notification",
			logger.String("id", notification.ID.String()),
			logger.String("channel", string(notification.Channel)),
			logger.String("recipient", notification.RecipientIdentifier),
		)

		var sendErr error
		var shouldInvalidate bool
		err := s.tm.ExecuteInTransaction(ctx, "worker_process", func(tx pgxdriver.QueryExecuter) error {
			current, err := s.notifyRepo.GetByID(ctx, tx, notification.ID, true)
			if err != nil {
				if errors.Is(err, entity.ErrDataNotFound) {
					return nil
				}
				return fmt.Errorf("get current status: %w", err)
			}

			if current.Status != entity.StatusInProcess {
				return nil
			}

			shouldInvalidate = true
			sendErr = s.sendNotification(ctx, notification)
			return s.updateAfterSend(ctx, tx, notification.ID, current.RetryCount, sendErr)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		if shouldInvalidate {
			_ = s.cache.InvalidateCache(ctx, notification.ID)
		}

		if sendErr != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "send failed",
				logger.String("id", notification.ID.String()),
				logger.Any("error", sendErr),
				logger.Duration("duration", time.Since(startTime)),
			)
			return sendErr
		}

		log.LogAttrs(ctx, logger.InfoLevel, "notification sent successfully",
			logger.String("id", notification.ID.String()),
			logger.Duration("duration", time.Since(startTime)),
		)

		return nil
	}
}

func (s *NotifyService) validateCreateRequest(req CreateNotificationRequest) error {
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
	if req.Recipient == "" {
		return fmt.Errorf("recipient is required: %w", entity.ErrInvalidData)
	}

	switch req.Channel {
	case entity.Email:
		if !isValidEmail(req.Recipient) {
			return fmt.Errorf("invalid email format: %w", entity.ErrInvalidData)
		}
		if len(req.Payload) > _maxPayloadSize {
			return fmt.Errorf("email payload too large (max 100KB): %w", entity.ErrInvalidData)
		}
	case entity.Telegram:
		if !isValidTelegramID(req.Recipient) {
			return fmt.Errorf("invalid telegram ID format: %w", entity.ErrInvalidData)
		}
	}

	return nil
}

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func isValidTelegramID(id string) bool {
	if strings.HasPrefix(id, "@") {
		return len(id) > 1
	}
	_, err := strconv.ParseInt(id, 10, 64)
	return err == nil
}

func (s *NotifyService) resolveRecipient(ctx context.Context, n *entity.Notification) error {
	if n.Channel != entity.Telegram {
		return nil
	}
	if !strings.HasPrefix(n.RecipientIdentifier, "@") {
		return nil
	}

	chatID, err := s.teleRepo.GetChatID(ctx, n.RecipientIdentifier)
	if err != nil {
		if errors.Is(err, entity.ErrDataNotFound) {
			return fmt.Errorf("user %s has not subscribed: %w",
				n.RecipientIdentifier, entity.ErrRecipientNotFound)
		}
		return fmt.Errorf("resolve recipient: %w", err)
	}

	n.RecipientIdentifier = strconv.FormatInt(chatID, 10)
	return nil
}

func (s *NotifyService) publishToQueue(
	ctx context.Context,
	notification entity.Notification,
) error {
	const op = "service.notify.publishToQueue"

	pubCtx, cancel := context.WithTimeout(ctx, _defaultTimeout)
	defer cancel()

	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", op, err)
	}

	routingKey := string(notification.Channel)
	if pubErr := s.publisher.Publish(pubCtx, payload, routingKey); pubErr != nil {
		return fmt.Errorf("%s: publish to %s: %w", op, routingKey, pubErr)
	}

	s.log.LogAttrs(ctx, logger.DebugLevel, "published to queue",
		logger.String("op", op),
		logger.String("id", notification.ID.String()),
		logger.String("routing_key", routingKey),
	)

	return nil
}

func (s *NotifyService) sendNotification(ctx context.Context, notification entity.Notification) error {
	if err := s.sender.Send(ctx, notification); err != nil {
		return fmt.Errorf("service.notify.sendNotification: %w", err)
	}
	return nil
}

func (s *NotifyService) updateAfterSend(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	id uuid.UUID,
	retryCount int,
	sendErr error,
) error {
	const op = "service.notify.updateAfterSend"

	if sendErr == nil {
		if updErr := s.notifyRepo.UpdateStatus(ctx, tx, id, entity.StatusSent, nil); updErr != nil {
			return fmt.Errorf("service.notify.updateAfterSend: %w", updErr)
		}
	}

	if err := s.handleSendFailure(ctx, tx, id, retryCount, sendErr); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *NotifyService) handleSendFailure(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	id uuid.UUID,
	retryCount int,
	sendErr error,
) error {
	errMsg := sendErr.Error()
	if statusErr := s.notifyRepo.UpdateStatus(ctx, tx, id, entity.StatusFailed, &errMsg); statusErr != nil {
		return fmt.Errorf("update status to failed: %w", statusErr)
	}

	if retryCount >= s.maxRetries {
		return nil
	}

	return s.scheduleRetry(ctx, tx, id, retryCount)
}

func (s *NotifyService) scheduleRetry(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	id uuid.UUID,
	retryCount int,
) error {
	nextAttempt := s.CalculateNextAttempt(retryCount)

	if err := s.notifyRepo.RescheduleNotification(ctx, tx, id, nextAttempt); err != nil {
		return fmt.Errorf("reschedule notification: %w", err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "notification rescheduled",
		logger.String("id", id.String()),
		logger.Int("retry_count", retryCount+1),
		logger.Time("next_attempt", nextAttempt),
	)

	return nil
}

func (s *NotifyService) CalculateNextAttempt(retryCount int) time.Time {
	if retryCount < 0 {
		retryCount = 0
	}
	multiplier := 1 << min(retryCount, _maxRetryExponent)
	delay := s.retryDelay * time.Duration(multiplier)

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
