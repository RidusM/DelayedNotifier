package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"delayednotifier/internal/config"
	"delayednotifier/internal/entity"
	"delayednotifier/internal/repository"
	"delayednotifier/internal/service"
	httpt "delayednotifier/internal/transport/http"
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
	_strategyAttempts = 3
	_strategyDelay    = 3 * time.Second
	_strategyBackoff  = 2.0

	_rabbitMQWorkers       = 2
	_rabbitMQPrefetchCount = 10

	_cacheTTL = 5 * time.Minute

	_queueProcessorInterval = 5 * time.Second
)

func Run(ctx context.Context, cfg *config.Config, log logger.Logger) error {
	var (
		db  *pgxdriver.Postgres
		rdb *redis.Client
		rmq *rabbitmq.RabbitClient
		tm  transaction.Manager
		svc *service.NotifyService
		err error
	)

	defer func() {
		if rdb != nil {
			if closeErr := rdb.Close(); closeErr != nil {
				log.Error("failed to close Redis client", logger.Any("error", closeErr))
			} else {
				log.Info("Redis client closed")
			}
		}
		if db != nil {
			db.Close()
			log.Info("database connection pool closed")
		}
	}()

	db, err = initDatabase(&cfg.Database, log)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	log.Info("database initialized successfully")

	tm, err = initTransactionManager(db, log)
	if err != nil {
		return fmt.Errorf("init transaction manager: %w", err)
	}

	rdb = initCache(&cfg.Cache)
	log.Info("cache initialized successfully")

	teleSender, err := initTelegramSender(&cfg.TG, log)
	if err != nil {
		return fmt.Errorf("init telegram sender: %w", err)
	}

	emailSender := initEmailSender(&cfg.SMTP, log)

	multiSender := initMultiSender(log, teleSender, emailSender)
	log.Info("multi-sender initialized successfully")

	rmq, err = initRabbitMQ(&cfg.Publisher)
	if err != nil {
		return fmt.Errorf("init rabbitmq client: %w", err)
	}
	log.Info("RabbitMQ client initialized successfully")

	if declareErr := declareRabbitMQQueues(rmq, cfg.Publisher.Exchange); declareErr != nil {
		return fmt.Errorf("declare rabbitmq queues: %w", declareErr)
	}

	publisher := rabbitmq.NewPublisher(rmq, cfg.Publisher.Exchange, cfg.Publisher.ContentType)

	svc, err = initNotifyService(&cfg.Service, db, tm, rdb, multiSender, publisher, log)
	if err != nil {
		return fmt.Errorf("init notify service: %w", err)
	}
	log.Info("notification service initialized successfully")

	eg, ctx := errgroup.WithContext(ctx)

	if serverErr := initHTTPServer(ctx, eg, &cfg.HTTP, svc, log); serverErr != nil {
		return fmt.Errorf("init http server: %w", serverErr)
	}
	log.Info("HTTP server initialized")

	eg.Go(func() error {
		initQueueProcessor(ctx, log, svc)
		return nil
	})

	eg.Go(func() error {
		return startConsumers(ctx, svc, rmq, log)
	})

	log.Info("application started",
		logger.String("env", cfg.Env),
		logger.String("version", cfg.App.Version),
	)

	if egErr := eg.Wait(); egErr != nil {
		if !errors.Is(egErr, context.Canceled) {
			log.Error("application shutdown with error", logger.Any("error", egErr))
			return fmt.Errorf("application shutdown error: %w", egErr)
		}
	}

	if rmqErr := rmq.Close(); rmqErr != nil {
		log.Error("Failed to close RabbitMQ client", logger.Any("error", rmqErr))
	}

	log.Info("application shutdown complete")
	return nil
}

func initDatabase(cfg *config.Database, log logger.Logger) (*pgxdriver.Postgres, error) {
	db, err := pgxdriver.New(
		cfg.DSN,
		log.With(logger.String("component", "database")),
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

func initTransactionManager(db *pgxdriver.Postgres, log logger.Logger) (transaction.Manager, error) {
	tm, err := transaction.NewManager(db, log)
	if err != nil {
		return nil, fmt.Errorf("create transaction manager: %w", err)
	}
	return tm, nil
}

func initCache(cfg *config.Cache) *redis.Client {
	return redis.New(cfg.Addr, cfg.Password, 0)
}

func initTelegramSender(cfg *config.TG, log logger.Logger) (*sender.TelegramSender, error) {
	if cfg.Token == "" {
		log.Warn("telegram sender disabled: token not configured")
		return nil, errors.New("token not configured")
	}

	sender, err := sender.NewTelegramSender(cfg.Token, log)
	if err != nil {
		return nil, fmt.Errorf("init telegram sender: %w", err)
	}
	return sender, nil
}

func initEmailSender(cfg *config.SMTP, log logger.Logger) *sender.EmailSender {
	if cfg.Host == "" {
		log.Warn("email sender disabled: host not configured")
		return nil
	}
	return sender.NewEmailSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From, log)
}

func initMultiSender(
	log logger.Logger,
	tg *sender.TelegramSender,
	email *sender.EmailSender,
) *sender.MultiSender {
	multi := sender.NewMultiSender()

	if tg != nil {
		multi.Register(entity.Telegram, tg)
		log.Info("registered telegram sender")
	}

	if email != nil {
		multi.Register(entity.Email, email)
		log.Info("registered email sender")
	}

	return multi
}

func initRabbitMQ(cfg *config.Publisher) (*rabbitmq.RabbitClient, error) {
	strategy := retry.Strategy{
		Attempts: _strategyAttempts,
		Delay:    _strategyDelay,
		Backoff:  _strategyBackoff,
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

func startConsumers(
	ctx context.Context,
	svc *service.NotifyService,
	client *rabbitmq.RabbitClient,
	log logger.Logger,
) error {
	channels := []string{"telegram", "email"}

	var wg sync.WaitGroup

	for _, channel := range channels {
		wg.Add(1)
		go func(ch string) {
			defer wg.Done()

			consumerCfg := rabbitmq.ConsumerConfig{
				Queue:         ch,
				ConsumerTag:   fmt.Sprintf("delayed-notifier-%s", ch),
				AutoAck:       false,
				Workers:       _rabbitMQWorkers,
				PrefetchCount: _rabbitMQPrefetchCount,
				Ask: rabbitmq.AskConfig{
					Multiple: false,
				},
				Nack: rabbitmq.NackConfig{
					Multiple: false,
					Requeue:  true,
				},
			}

			consumer := rabbitmq.NewConsumer(client, consumerCfg, svc.GetWorkerHandler())

			log.Info("Starting consumer", "channel", ch)

			if err := consumer.Start(ctx); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, rabbitmq.ErrClientClosed) {
					log.Error("Consumer failed", "channel", ch, "error", err)
				}
			}

			log.Info("Consumer stopped", "channel", ch)
		}(channel)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("consumer context cancelled: %w", ctx.Err())
	case <-done:
		return nil
	}
}

func declareRabbitMQQueues(client *rabbitmq.RabbitClient, exchangeName string) error {
	if err := client.DeclareExchange(
		exchangeName,
		"direct",
		true,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", exchangeName, err)
	}
	queues := []struct {
		name       string
		routingKey string
		durable    bool
		autoDelete bool
	}{
		{"telegram", "telegram", true, false},
		{"email", "email", true, false},
	}

	for _, q := range queues {
		if err := client.DeclareQueue(
			q.name,
			exchangeName,
			q.routingKey,
			q.durable,
			q.autoDelete,
			true,
			nil,
		); err != nil {
			return fmt.Errorf("declare queue %s: %w", q.name, err)
		}
	}

	return nil
}

func initQueueProcessor(
	ctx context.Context,
	log logger.Logger,
	service *service.NotifyService,
) {
	ticker := time.NewTicker(_queueProcessorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := service.ProcessQueue(ctx)
			if err != nil {
				log.Error("ProcessQueue failed", "error", err)
				continue
			}
			if stats.Processed > 0 {
				log.Info("Queue processed",
					"processed", stats.Processed,
					"failed", stats.Failed,
					"duration", stats.Duration)
			}
		case <-ctx.Done():
			return
		}
	}
}

func initNotifyService(
	cfg *config.Service,
	db *pgxdriver.Postgres,
	tm transaction.Manager,
	rdb *redis.Client,
	ms *sender.MultiSender,
	rmq *rabbitmq.Publisher,
	log logger.Logger,
) (*service.NotifyService, error) {
	notifyRepo := repository.NewNotifyRepository(db)
	userRepo := repository.NewUserRepository(db)
	cacheRepo := repository.NewCacheRepository(rdb, _cacheTTL)

	svc, err := service.NewNotifyService(
		notifyRepo,
		userRepo,
		cacheRepo,
		ms,
		tm,
		rmq,
		log,
		service.QueryLimit(cfg.QueryLimit),
		service.RetryDelay(cfg.RetryDelay),
		service.MaxRetries(cfg.MaxRetries),
	)
	if err != nil {
		return nil, fmt.Errorf("create notify service: %w", err)
	}
	return svc, nil
}

func initHTTPServer(
	ctx context.Context,
	eg *errgroup.Group,
	cfg *config.HTTP,
	svc *service.NotifyService,
	log logger.Logger,
) error {
	handler := httpt.NewNotifyHandler(svc, log)
	httpServer, err := httpt.NewHTTPServer(handler, cfg, log.With(logger.String("component", "http")))
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	eg.Go(func() error {
		if serverErr := httpServer.Start(ctx); serverErr != nil && !errors.Is(serverErr, context.Canceled) {
			return fmt.Errorf("http server error: %w", serverErr)
		}
		return nil
	})

	return nil
}
