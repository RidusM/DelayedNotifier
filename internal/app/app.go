package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"delayednotifier/internal/config"
	"delayednotifier/internal/entity"
	"delayednotifier/internal/repository"
	"delayednotifier/internal/service"
	handler "delayednotifier/internal/transport/http"
	"delayednotifier/internal/transport/sender"

	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/dbpg/pgx-driver/transaction"
	"github.com/wb-go/wbf/logger"
	"github.com/wb-go/wbf/rabbitmq"
	"github.com/wb-go/wbf/redis"
	"github.com/wb-go/wbf/retry"
	"golang.org/x/sync/errgroup"
)

const (
	_defaultPrefetchCount = 10
	_tokenByteLength      = 16
)

func Run(ctx context.Context, cfg *config.Config, log logger.Logger) error {
	var (
		db  *pgxdriver.Postgres
		rdb *redis.Client
		rmq *rabbitmq.RabbitClient
		err error
	)

	defer func() {
		closeResources(ctx, db, rdb, rmq, log)
	}()

	db, rdb, rmq, err = initInfrastructure(ctx, cfg, log)
	if err != nil {
		return err
	}

	tm, err := transaction.NewManager(db, log)
	if err != nil {
		return fmt.Errorf("init transaction manager: %w", err)
	}

	svc, handler, teleSender, err := initServices(ctx, cfg, db, tm, rdb, rmq, log)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	startWorkers(ctx, eg, svc, handler, teleSender, rmq, cfg, log)

	if egErr := eg.Wait(); egErr != nil && !errors.Is(egErr, context.Canceled) {
		return fmt.Errorf("app execution failed: %w", egErr)
	}

	return nil
}

func closeResources(
	ctx context.Context,
	db *pgxdriver.Postgres,
	rdb *redis.Client,
	rmq *rabbitmq.RabbitClient,
	log logger.Logger,
) {
	if db != nil {
		db.Close()
		log.LogAttrs(ctx, logger.InfoLevel, "database connection closed")
	}
	if rdb != nil {
		if closeErr := rdb.Close(); closeErr != nil {
			log.LogAttrs(ctx, logger.WarnLevel, "failed to close cache",
				logger.Any("error", closeErr),
			)
		}
	}
	if rmq != nil {
		if closeErr := rmq.Close(); closeErr != nil {
			log.Error("failed to close RabbitMQ", "error", closeErr)
		} else {
			log.LogAttrs(ctx, logger.InfoLevel, "rabbitmq connection closed")
		}
	}
	log.LogAttrs(ctx, logger.InfoLevel, "all resources cleaned up")
}

func initInfrastructure(
	ctx context.Context,
	cfg *config.Config,
	log logger.Logger,
) (*pgxdriver.Postgres, *redis.Client, *rabbitmq.RabbitClient, error) {
	db, err := initDatabase(&cfg.Database, log)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init database: %w", err)
	}
	log.LogAttrs(ctx, logger.InfoLevel, "database initialized successfully")

	rdb, err := initCache(ctx, &cfg.Cache)
	if err != nil {
		db.Close()
		return nil, nil, nil, fmt.Errorf("init cache: %w", err)
	}
	log.LogAttrs(ctx, logger.InfoLevel, "cache initialized successfully")

	rmq, err := initRabbitMQ(&cfg.Publisher)
	if err != nil {
		db.Close()
		_ = rdb.Close()
		return nil, nil, nil, fmt.Errorf("init rabbitmq: %w", err)
	}

	if declareErr := declareRabbitMQQueues(rmq, cfg.Publisher.Exchange); declareErr != nil {
		db.Close()
		_ = rdb.Close()
		_ = rmq.Close()
		return nil, nil, nil, fmt.Errorf("declare queues: %w", declareErr)
	}

	return db, rdb, rmq, nil
}

func initServices(
	ctx context.Context,
	cfg *config.Config,
	db *pgxdriver.Postgres,
	tm transaction.Manager,
	rdb *redis.Client,
	rmq *rabbitmq.RabbitClient,
	log logger.Logger,
) (*service.NotifyService, *handler.NotifyHandler, *sender.TelegramSender, error) {
	userRepo := repository.NewUserRepository(db)
	notifyRepo := repository.NewNotifyRepository(db)
	cacheRepo := repository.NewCacheRepository(rdb)

	teleSender, err := sender.NewTelegramSender(cfg.TG.Token, log)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init telegram sender: %w", err)
	}

	emailSender := sender.NewEmailSender(
		cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From, log,
	)

	multiSender := sender.NewMultiSender()
	multiSender.Register(entity.Telegram, teleSender)
	multiSender.Register(entity.Email, emailSender)
	log.LogAttrs(ctx, logger.InfoLevel, "multi-sender initialized with telegram and email")

	publisher := rabbitmq.NewPublisher(rmq, cfg.Publisher.Exchange, cfg.Publisher.ContentType)

	svc, err := service.NewNotifyService(
		notifyRepo,
		userRepo,
		cacheRepo,
		multiSender,
		tm,
		publisher,
		log,
		service.QueryLimit(cfg.Service.QueryLimit),
		service.MaxRetries(cfg.Service.MaxRetries),
		service.RetryDelay(cfg.Service.RetryDelay),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init service: %w", err)
	}

	handler := handler.NewNotifyHandler(svc, log, cfg.TG)
	return svc, handler, teleSender, nil
}

func startWorkers(
	ctx context.Context,
	eg *errgroup.Group,
	svc *service.NotifyService,
	h *handler.NotifyHandler,
	teleSender *sender.TelegramSender,
	rmq *rabbitmq.RabbitClient,
	cfg *config.Config,
	log logger.Logger,
) {
	eg.Go(func() error {
		return startHTTPServer(ctx, h, &cfg.HTTP, log)
	})

	if teleSender != nil {
		eg.Go(func() error {
			log.LogAttrs(ctx, logger.InfoLevel, "starting telegram polling for subscribers")
			tgHandler := svc.GetTelegramStartHandler()
			teleSender.StartPolling(
				ctx,
				func(ctx context.Context, username string, chatID *int64, startPayload string) error {
					return tgHandler(ctx, username, chatID, startPayload)
				},
			)
			return nil
		})
	}

	eg.Go(func() error {
		return startQueueProcessor(ctx, svc, cfg.Publisher.QueueProcessorInterval, log)
	})

	for _, ch := range entity.ListChannels() {
		queueName := string(ch)
		eg.Go(func() error {
			return runConsumer(ctx, svc, rmq, queueName, cfg.Publisher.RabbitMQWorkers, log)
		})
	}
}

func startHTTPServer(ctx context.Context, h *handler.NotifyHandler, cfg *config.HTTP, log logger.Logger) error {
	server := handler.NewHTTPServer(h, cfg, log)
	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("start http server: %w", err)
	}
	return nil
}

func initDatabase(cfg *config.Database, log logger.Logger) (*pgxdriver.Postgres, error) {
	db, err := pgxdriver.New(
		cfg.DSN,
		log,
		pgxdriver.MaxPoolSize(cfg.PoolMax),
		pgxdriver.MaxConnAttempts(cfg.ConnAttempts),
		pgxdriver.BaseRetryDelay(cfg.BaseRetryDelay),
		pgxdriver.MaxRetryDelay(cfg.MaxRetryDelay),
	)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	return db, nil
}

func initCache(ctx context.Context, cfg *config.Cache) (*redis.Client, error) {
	initCtx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	rdb := redis.New(cfg.Addr, cfg.Password, cfg.DB)

	if err := rdb.Ping(initCtx); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("cache ping failed: %w", err)
	}
	return rdb, nil
}

func initRabbitMQ(cfg *config.Publisher) (*rabbitmq.RabbitClient, error) {
	strategy := retry.Strategy{
		Attempts: cfg.Attempts,
		Delay:    cfg.Delay,
		Backoff:  cfg.Backoff,
	}
	rmqCfg := rabbitmq.ClientConfig{
		URL:            cfg.URL,
		ConnectionName: cfg.ConnectionName,
		ConnectTimeout: cfg.ConnectTimeout,
		Heartbeat:      cfg.Heartbeat,
		ProducingStrat: strategy,
		ConsumingStrat: strategy,
		ReconnectStrat: strategy,
	}

	client, err := rabbitmq.NewClient(rmqCfg)
	if err != nil {
		return nil, fmt.Errorf("create rabbitmq client: %w", err)
	}
	return client, nil
}

func declareRabbitMQQueues(client *rabbitmq.RabbitClient, exchangeName string) error {
	if err := client.DeclareExchange(exchangeName, "direct", true, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", exchangeName, err)
	}

	for _, ch := range entity.ListChannels() {
		queueName := string(ch)
		if err := client.DeclareQueue(queueName, exchangeName, queueName, true, false, true, nil); err != nil {
			return fmt.Errorf("declare queue %s: %w", queueName, err)
		}
	}
	return nil
}

func startQueueProcessor(
	ctx context.Context,
	svc *service.NotifyService,
	interval time.Duration,
	log logger.Logger,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := svc.ProcessQueue(ctx)
			if err != nil {
				log.Error("queue processing failed", "error", err)
				continue
			}
			if stats.Processed > 0 {
				log.LogAttrs(ctx, logger.InfoLevel, "queue processed",
					logger.Int("processed", stats.Processed),
					logger.Int("failed", stats.Failed),
					logger.Duration("duration", stats.Duration),
				)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func runConsumer(
	ctx context.Context,
	svc *service.NotifyService,
	client *rabbitmq.RabbitClient,
	queueName string,
	workers int,
	log logger.Logger,
) error {
	consumerCfg := rabbitmq.ConsumerConfig{
		Queue:         queueName,
		ConsumerTag:   fmt.Sprintf("delayed-notifier-%s", queueName),
		AutoAck:       false,
		Workers:       workers,
		PrefetchCount: _defaultPrefetchCount,
		Ask:           rabbitmq.AskConfig{Multiple: false},
		Nack:          rabbitmq.NackConfig{Multiple: false, Requeue: true},
	}

	handler := svc.GetWorkerHandler()
	consumer := rabbitmq.NewConsumer(client, consumerCfg, handler)

	log.LogAttrs(ctx, logger.InfoLevel, "starting consumer",
		logger.String("queue", queueName),
		logger.Int("workers", workers),
	)

	err := consumer.Start(ctx)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, rabbitmq.ErrClientClosed) {
		return fmt.Errorf("consumer %s error: %w", queueName, err)
	}
	return nil
}
