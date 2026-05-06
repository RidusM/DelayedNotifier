// nolint:musttag
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	_defaultMaxRetries      = 3
	_defaultQueryLimit      = 10
	_defaultRetryDelay      = 5 * time.Minute
	_maxRetryDelay          = 30 * time.Minute
	_maxRetryExponentCap    = 4
	_maxPayloadSize         = 100_000
	_defaultTimeout         = 2 * time.Second
	_batchTimeout           = 20 * time.Second
	_itemTimeout            = 5 * time.Second
	_serviceTokenByteLength = 16

	_slowOperationThreshold = 200 * time.Millisecond
)

type NotifyRepository interface {
	Create(ctx context.Context, qe pgxdriver.QueryExecuter, notify entity.Notification) error
	GetByID(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID, forUpdate bool) (*entity.Notification, error)
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
}

type UserRepository interface {
	Create(ctx context.Context, qe pgxdriver.QueryExecuter, u entity.User) error
	GetByID(ctx context.Context, qe pgxdriver.QueryExecuter, id uuid.UUID) (*entity.User, error)
	GetByTelegramID(ctx context.Context, qe pgxdriver.QueryExecuter, chatID *int64) (*entity.User, error)
	UpdateTelegramID(ctx context.Context, qe pgxdriver.QueryExecuter, userID uuid.UUID, chatID *int64) error
	CreateLinkToken(
		ctx context.Context,
		qe pgxdriver.QueryExecuter,
		userID uuid.UUID,
		token string,
		expiresAt time.Time,
	) error
	GetUserByLinkToken(ctx context.Context, qe pgxdriver.QueryExecuter, token string) (uuid.UUID, error)
	DeleteLinkToken(ctx context.Context, qe pgxdriver.QueryExecuter, token string) error
}

type CacheRepository interface {
	Get(ctx context.Context, id uuid.UUID) (*entity.Notification, error)
	Save(ctx context.Context, notification *entity.Notification) error
	Invalidate(ctx context.Context, id uuid.UUID) error
}

type NotificationSender interface {
	Send(ctx context.Context, n entity.Notification, recipient string) error
}

type PublisherInterface interface {
	Publish(ctx context.Context, body []byte, routingKey string, opts ...rabbitmq.PublishOption) error
	GetExchangeName() string
}

type RegisterUserRequest struct {
	Name       string
	Email      string
	TelegramID *int64
}

type CreateNotificationRequest struct {
	UserID      uuid.UUID
	Channel     entity.Channel
	Payload     string
	ScheduledAt time.Time
}

type ProcessingStats struct {
	Processed int
	Failed    int
	Duration  time.Duration
}

type NotifyService struct {
	notifyRepo NotifyRepository
	userRepo   UserRepository
	cache      CacheRepository
	sender     NotificationSender
	tm         transaction.Manager
	publisher  PublisherInterface
	log        logger.Logger

	queryLimit uint64
	maxRetries int
	retryDelay time.Duration
}

func NewNotifyService(
	notifyRepo NotifyRepository,
	userRepo UserRepository,
	cache CacheRepository,
	sender NotificationSender,
	tm transaction.Manager,
	publisher PublisherInterface,
	log logger.Logger,
	opts ...Option,
) (*NotifyService, error) {
	const op = "service.notify.NewNotifyService"
	s := &NotifyService{
		notifyRepo: notifyRepo,
		userRepo:   userRepo,
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
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return s, nil
}

func (s *NotifyService) RegisterUser(ctx context.Context, req RegisterUserRequest) (*entity.User, error) {
	const op = "service.RegisterUser"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.String("name", req.Name),
		logger.String("email", req.Email),
	)

	log.LogAttrs(ctx, logger.InfoLevel, "register user requested",
		logger.String("name", req.Name),
		logger.String("email", req.Email),
	)

	if req.Email == "" && (req.TelegramID == nil || *req.TelegramID == 0) {
		return nil, fmt.Errorf("%s: email or telegram_id is required: %w", op, entity.ErrInvalidData)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("%s: generate id: %w", op, err)
	}

	var telegramID *int64
	if req.TelegramID != nil {
		telegramID = req.TelegramID
	}

	user := entity.User{
		ID:         id,
		Name:       req.Name,
		Email:      req.Email,
		TelegramID: telegramID,
		CreatedAt:  time.Now().UTC(),
	}

	err = s.tm.ExecuteInTransaction(ctx, "register_user", func(tx pgxdriver.QueryExecuter) error {
		if err = s.userRepo.Create(ctx, tx, user); err != nil {
			return transaction.HandleError("create", err)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "register failed", logger.Any("error", err))
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "user registered",
		logger.String("user_id", id.String()),
		logger.Duration("duration", time.Since(startTime)),
	)
	return &user, nil
}

func (s *NotifyService) GenerateLinkToken(ctx context.Context, userID uuid.UUID) (string, error) {
	const op = "service.GenerateLinkToken"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.String("user_id", userID.String()),
	)

	log.LogAttrs(ctx, logger.InfoLevel, "generate link token requested",
		logger.String("user_id", userID.String()),
	)

	bytes := make([]byte, _serviceTokenByteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("%s: generate random: %w", op, err)
	}
	token := hex.EncodeToString(bytes)

	expiresAt := time.Now().UTC().Add(1 * time.Hour)

	err := s.tm.ExecuteInTransaction(ctx, "create_link_token", func(tx pgxdriver.QueryExecuter) error {
		if err := s.userRepo.CreateLinkToken(ctx, tx, userID, token, expiresAt); err != nil {
			return transaction.HandleError("create", err)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "create link token failed", logger.Any("error", err))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "link token generated successfully",
		logger.String("user_id", userID.String()),
		logger.Duration("duration", time.Since(startTime)),
	)
	return token, nil
}

func (s *NotifyService) LinkTelegramByToken(ctx context.Context, token string, chatID *int64) error {
	const op = "service.LinkTelegramByToken"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.Int64("chat_id", *chatID),
	)

	log.LogAttrs(ctx, logger.InfoLevel, "link telegram by token requested",
		logger.Int64("chat_id", *chatID),
	)

	err := s.tm.ExecuteInTransaction(ctx, "link_telegram_by_token", func(tx pgxdriver.QueryExecuter) error {
		userID, err := s.userRepo.GetUserByLinkToken(ctx, tx, token)
		if err != nil {
			if errors.Is(err, entity.ErrDataNotFound) || errors.Is(err, entity.ErrInvalidData) {
				return fmt.Errorf("%s: invalid or expired token: %w", op, entity.ErrInvalidData)
			}
			return fmt.Errorf("%s: get user id by token: %w", op, err)
		}

		if err = s.userRepo.DeleteLinkToken(ctx, tx, token); err != nil {
			return fmt.Errorf("%s: delete token: %w", op, err)
		}

		user, err := s.userRepo.GetByID(ctx, tx, userID)
		if err != nil {
			return fmt.Errorf("%s: get user details: %w", op, err)
		}

		if err = s.userRepo.UpdateTelegramID(ctx, tx, user.ID, chatID); err != nil {
			return transaction.HandleError("update_telegram_id", err)
		}

		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "link telegram by token failed", logger.Any("error", err))
		return fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "telegram linked successfully",
		logger.String("user_id", "hidden"),
		logger.Int64("chat_id", *chatID),
		logger.Duration("duration", time.Since(startTime)),
	)
	return nil
}

func (s *NotifyService) GetUserByTelegramID(ctx context.Context, chatID *int64) (*entity.User, error) {
	const op = "service.GetUserByTelegramID"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.Int64("chat_id", *chatID),
	)

	log.LogAttrs(ctx, logger.DebugLevel, "get user by telegram id requested",
		logger.Int64("chat_id", *chatID),
	)

	user, err := s.userRepo.GetByTelegramID(ctx, nil, chatID)
	if err != nil {
		if errors.Is(err, entity.ErrDataNotFound) {
			log.LogAttrs(ctx, logger.DebugLevel, "user not found by telegram id")
		} else {
			log.LogAttrs(ctx, logger.ErrorLevel, "get user by telegram id failed", logger.Any("error", err))
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.DebugLevel, "user found by telegram id",
		logger.String("user_id", user.ID.String()),
		logger.Duration("duration", time.Since(startTime)),
	)
	return user, nil
}

func (s *NotifyService) CreateNotify(ctx context.Context, req CreateNotificationRequest) (uuid.UUID, error) {
	const op = "service.CreateNotify"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.String("user_id", req.UserID.String()),
		logger.String("channel", string(req.Channel)),
	)

	log.LogAttrs(ctx, logger.InfoLevel, "create notification requested",
		logger.String("user_id", req.UserID.String()),
		logger.String("channel", string(req.Channel)),
		logger.Time("scheduled_at", req.ScheduledAt),
	)

	if err := s.validateCreateRequest(req); err != nil {
		log.LogAttrs(ctx, logger.WarnLevel, "validation failed", logger.Any("error", err))
		return uuid.Nil, fmt.Errorf("%s: %w", op, err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "generate id failed", logger.Any("error", err))
		return uuid.Nil, fmt.Errorf("%s: generate id: %w", op, err)
	}

	notification := entity.Notification{
		ID:          id,
		Channel:     req.Channel,
		Payload:     req.Payload,
		UserID:      req.UserID,
		ScheduledAt: req.ScheduledAt.UTC(),
		Status:      entity.StatusWaiting,
		CreatedAt:   time.Now().UTC(),
	}

	err = s.tm.ExecuteInTransaction(ctx, "create_notification", func(tx pgxdriver.QueryExecuter) error {
		if err = s.notifyRepo.Create(ctx, tx, notification); err != nil {
			return transaction.HandleError("create", err)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "creation failed", logger.Any("error", err))
		return uuid.Nil, fmt.Errorf("%s: %w", op, err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notification created successfully",
		logger.String("id", id.String()),
		logger.Duration("duration", time.Since(startTime)),
	)
	return id, nil
}

func (s *NotifyService) GetStatus(ctx context.Context, id uuid.UUID) (*entity.Notification, error) {
	const op = "service.GetStatus"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.String("id", id.String()),
	)

	log.LogAttrs(ctx, logger.DebugLevel, "get status requested",
		logger.String("id", id.String()),
	)

	if cached, err := s.cache.Get(ctx, id); err == nil && cached != nil {
		log.LogAttrs(ctx, logger.DebugLevel, "served from cache",
			logger.Duration("duration", time.Since(startTime)),
		)
		return cached, nil
	}

	notification, err := s.notifyRepo.GetByID(ctx, nil, id, false)
	if err != nil {
		if errors.Is(err, entity.ErrDataNotFound) {
			log.LogAttrs(ctx, logger.WarnLevel, "notification not found")
			return nil, entity.ErrDataNotFound
		}
		log.LogAttrs(ctx, logger.ErrorLevel, "failed to get from database", logger.Any("error", err))
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	go func() {
		cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), _defaultTimeout)
		defer cancel()
		if err = s.cache.Save(cacheCtx, notification); err != nil {
			s.log.LogAttrs(cacheCtx, logger.WarnLevel, "failed to update cache",
				logger.String("id", id.String()),
				logger.Any("error", err),
			)
		}
	}()

	log.LogAttrs(ctx, logger.DebugLevel, "status retrieved from db",
		logger.String("status", string(notification.Status)),
		logger.Duration("duration", time.Since(startTime)),
	)
	return notification, nil
}

func (s *NotifyService) Cancel(ctx context.Context, id uuid.UUID) error {
	const op = "service.Cancel"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime,
		logger.String("id", id.String()),
	)

	log.LogAttrs(ctx, logger.InfoLevel, "cancel requested",
		logger.String("id", id.String()),
	)

	err := s.tm.ExecuteInTransaction(ctx, "cancel_notification", func(tx pgxdriver.QueryExecuter) error {
		notification, err := s.notifyRepo.GetByID(ctx, tx, id, true)
		if err != nil {
			if errors.Is(err, entity.ErrDataNotFound) {
				return entity.ErrDataNotFound
			}
			return fmt.Errorf("get notification: %w", err)
		}

		switch notification.Status {
		case entity.StatusSent, entity.StatusInProcess:
			return entity.ErrNotificationAlreadySent
		case entity.StatusCancelled:
			return entity.ErrNotificationCancelled
		case entity.StatusWaiting, entity.StatusFailed:
			// ok
		default:
			return fmt.Errorf("unknown status: %s", notification.Status)
		}

		cancelReason := "cancelled by user"
		if err = s.notifyRepo.UpdateStatus(ctx, tx, id, entity.StatusCancelled, &cancelReason); err != nil {
			return transaction.HandleError("update_status", err)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "cancel failed", logger.Any("error", err))
		return fmt.Errorf("%s: %w", op, err)
	}

	if err = s.cache.Invalidate(ctx, id); err != nil {
		log.LogAttrs(ctx, logger.WarnLevel, "cache invalidation failed", logger.Any("error", err))
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notification cancelled successfully",
		logger.String("id", id.String()),
		logger.Duration("duration", time.Since(startTime)),
	)
	return nil
}

func (s *NotifyService) ProcessQueue(ctx context.Context) (*ProcessingStats, error) {
	const op = "service.ProcessQueue"

	log := s.log.With("op", op)
	startTime := time.Now()
	defer s.logSlowOperation(ctx, op, startTime)

	log.LogAttrs(ctx, logger.DebugLevel, "process queue started")

	procCtx, cancel := context.WithTimeout(ctx, _batchTimeout)
	defer cancel()

	stats := &ProcessingStats{}

	var notifications []entity.Notification
	err := s.tm.ExecuteInTransaction(procCtx, "get_for_process", func(tx pgxdriver.QueryExecuter) error {
		var err error
		notifications, err = s.notifyRepo.GetForProcess(procCtx, tx, s.queryLimit)
		if err != nil {
			return transaction.HandleError("get", err)
		}
		return nil
	})
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "get for process failed", logger.Any("error", err))
		return stats, fmt.Errorf("%s: get for process: %w", op, err)
	}

	log.LogAttrs(ctx, logger.DebugLevel, "processing batch",
		logger.Int("count", len(notifications)),
	)

	for _, n := range notifications {
		itemCtx, itemCancel := context.WithTimeout(procCtx, _itemTimeout)
		if err = s.processSingle(itemCtx, n); err != nil {
			stats.Failed++
			log.LogAttrs(ctx, logger.WarnLevel, "notification processing failed",
				logger.String("id", n.ID.String()),
				logger.Any("error", err),
			)
		} else {
			stats.Processed++
		}
		itemCancel()
	}

	stats.Duration = time.Since(startTime)
	log.LogAttrs(ctx, logger.DebugLevel, "queue processing completed",
		logger.Int("processed", stats.Processed),
		logger.Int("failed", stats.Failed),
		logger.Duration("duration", stats.Duration),
	)
	return stats, nil
}

func (s *NotifyService) processSingle(ctx context.Context, n entity.Notification) error {
	if err := s.tm.ExecuteInTransaction(ctx, "mark_in_process", func(tx pgxdriver.QueryExecuter) error {
		return s.notifyRepo.UpdateStatus(ctx, tx, n.ID, entity.StatusInProcess, nil)
	}); err != nil {
		return fmt.Errorf("mark_in_process: %w", err)
	}

	if err := s.publishToQueue(ctx, n); err != nil {
		_ = s.tm.ExecuteInTransaction(ctx, "rollback_to_waiting", func(tx pgxdriver.QueryExecuter) error {
			return s.notifyRepo.UpdateStatus(ctx, tx, n.ID, entity.StatusWaiting, nil)
		})
		return fmt.Errorf("publish_to_queue: %w", err)
	}
	return nil
}

func (s *NotifyService) publishToQueue(ctx context.Context, notification entity.Notification) error {
	const op = "service.publishToQueue"

	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", op, err)
	}

	routingKey := string(notification.Channel)
	if err = s.publisher.Publish(ctx, payload, routingKey); err != nil {
		s.log.Ctx(ctx).LogAttrs(ctx, logger.ErrorLevel, "publish failed",
			logger.String("id", notification.ID.String()),
			logger.String("routing_key", routingKey),
			logger.Any("error", err),
		)
		return fmt.Errorf("%s: publish to %s: %w", op, routingKey, err)
	}

	s.log.Ctx(ctx).LogAttrs(ctx, logger.DebugLevel, "published to queue",
		logger.String("id", notification.ID.String()),
		logger.String("routing_key", routingKey),
	)
	return nil
}

func (s *NotifyService) GetWorkerHandler() rabbitmq.MessageHandler {
	return func(ctx context.Context, msg amqp091.Delivery) error {
		const op = "service.WorkerHandler"

		var notification entity.Notification
		if err := json.Unmarshal(msg.Body, &notification); err != nil {
			s.log.LogAttrs(ctx, logger.ErrorLevel, "unmarshal failed", logger.Any("error", err))
			return msg.Ack(false)
		}

		log := s.log.With("op", op, "id", notification.ID.String())
		startTime := time.Now()

		log.LogAttrs(ctx, logger.DebugLevel, "processing message from queue")

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
				log.LogAttrs(ctx, logger.WarnLevel, "status changed, skipping",
					logger.String("current_status", string(current.Status)),
				)
				return nil
			}

			shouldInvalidate = true
			sendErr = s.sendNotification(ctx, notification)
			return s.updateAfterSend(ctx, tx, notification.ID, current.RetryCount, sendErr)
		})
		if err != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "worker transaction failed", logger.Any("error", err))
			return fmt.Errorf("%s: %w", op, err)
		}

		if shouldInvalidate {
			_ = s.cache.Invalidate(ctx, notification.ID)
		}

		if sendErr != nil {
			log.LogAttrs(ctx, logger.ErrorLevel, "send failed",
				logger.Any("error", sendErr),
				logger.Duration("duration", time.Since(startTime)),
			)
			return sendErr
		}

		log.LogAttrs(ctx, logger.InfoLevel, "notification sent successfully",
			logger.Duration("duration", time.Since(startTime)),
		)
		return msg.Ack(false)
	}
}

func (s *NotifyService) sendNotification(ctx context.Context, n entity.Notification) error {
	const op = "service.sendNotification"

	log := s.log.With("op", op, "id", n.ID.String())

	recipient, err := s.resolveRecipient(ctx, n)
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "resolve recipient failed", logger.Any("error", err))
		return fmt.Errorf("%s: resolve recipient: %w", op, err)
	}

	log.LogAttrs(ctx, logger.DebugLevel, "sending notification",
		logger.String("recipient", recipient),
		logger.String("channel", string(n.Channel)),
	)

	if err = s.sender.Send(ctx, n, recipient); err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "sender failed", logger.Any("error", err))
		return fmt.Errorf("%s: sender failed: %w", op, err)
	}

	log.LogAttrs(ctx, logger.DebugLevel, "sent via sender")
	return nil
}

func (s *NotifyService) resolveRecipient(ctx context.Context, n entity.Notification) (string, error) {
	user, err := s.userRepo.GetByID(ctx, nil, n.UserID)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}

	switch n.Channel {
	case entity.Email:
		if user.Email == "" {
			return "", fmt.Errorf("user has no email: %w", entity.ErrRecipientNotFound)
		}
		return user.Email, nil

	case entity.Telegram:
		if user.TelegramID == nil {
			return "", fmt.Errorf("user has no telegram_id: %w", entity.ErrRecipientNotFound)
		}
		return strconv.FormatInt(*user.TelegramID, 10), nil

	default:
		return "", fmt.Errorf("unsupported channel: %s", n.Channel)
	}
}

func (s *NotifyService) updateAfterSend(
	ctx context.Context,
	tx pgxdriver.QueryExecuter,
	id uuid.UUID,
	retryCount int,
	sendErr error,
) error {
	const op = "service.updateAfterSend"

	if sendErr != nil {
		return s.handleSendFailure(ctx, tx, id, retryCount, sendErr)
	}

	err := s.notifyRepo.UpdateStatus(ctx, tx, id, entity.StatusSent, nil)
	if err != nil {
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
	if err := s.notifyRepo.UpdateStatus(ctx, tx, id, entity.StatusFailed, &errMsg); err != nil {
		return fmt.Errorf("update status to failed: %w", err)
	}

	if retryCount >= s.maxRetries {
		s.log.LogAttrs(ctx, logger.WarnLevel, "max retries exceeded",
			logger.String("id", id.String()),
			logger.Int("retry_count", retryCount),
		)
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
	nextAttempt := s.calculateNextAttempt(retryCount)
	if nextAttempt.IsZero() {
		return nil
	}
	if err := s.notifyRepo.RescheduleNotification(ctx, tx, id, nextAttempt); err != nil {
		return fmt.Errorf("reschedule notification: %w", err)
	}

	s.log.Ctx(ctx).LogAttrs(ctx, logger.InfoLevel, "notification rescheduled",
		logger.String("id", id.String()),
		logger.Int("retry_count", retryCount+1),
		logger.Time("next_attempt", nextAttempt),
	)
	return nil
}

func (s *NotifyService) calculateNextAttempt(retryCount int) time.Time {
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount >= s.maxRetries {
		return time.Time{}
	}
	exp := min(retryCount, _maxRetryExponentCap)
	delay := min(s.retryDelay*time.Duration(1<<exp), _maxRetryDelay)
	return time.Now().UTC().Add(delay)
}

func (s *NotifyService) validateCreateRequest(req CreateNotificationRequest) error {
	if req.ScheduledAt.Before(time.Now().UTC()) {
		return fmt.Errorf("scheduled time must be in future: %w", entity.ErrInvalidData)
	}
	if len(req.Payload) > _maxPayloadSize {
		return fmt.Errorf("payload too large: %w", entity.ErrInvalidData)
	}
	if req.UserID == uuid.Nil {
		return fmt.Errorf("userID is required: %w", entity.ErrInvalidData)
	}
	return nil
}

func (s *NotifyService) logSlowOperation(
	ctx context.Context,
	op string,
	startTime time.Time,
	attrs ...logger.Attr,
) {
	duration := time.Since(startTime)
	if duration > _slowOperationThreshold {
		allAttrs := append([]logger.Attr{
			logger.String("op", op),
			logger.Duration("duration", duration),
		}, attrs...)
		s.log.LogAttrs(ctx, logger.WarnLevel, "slow operation detected", allAttrs...)
	}
}
