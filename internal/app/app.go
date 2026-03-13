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
	_rabbitMQWorkers       = 2
	_rabbitMQPrefetchCount = 10

	_queueProcessorInterval = 5 * time.Second
)

func Run(ctx context.Context, cfg *config.Config, log logger.Logger) error {
	var (
		db  *pgxdriver.Postgres
		rdb *redis.Client
		rmq *rabbitmq.RabbitClient
		tm  transaction.Manager
		err error
	)

	defer func() {
		if rmq != nil {
			if closeErr := rmq.Close(); closeErr != nil {
				log.LogAttrs(ctx, logger.ErrorLevel, "failed to close RabbitMQ",
					logger.Any("error", err),
				)
			}
		}
		if rdb != nil {
			_ = rdb.Close()
			log.Warn("rabbitMQ closed")
		}
		if db != nil {
			db.Close()
			log.Warn("database closed")
		}
		log.Info("all resources cleaned up")
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
		return fmt.Errorf("init rabbitmq: %w", err)
	}

	if declareErr := declareRabbitMQQueues(rmq, cfg.Publisher.Exchange); declareErr != nil {
		return fmt.Errorf("declare queues: %w", declareErr)
	}

	publisher := rabbitmq.NewPublisher(rmq, cfg.Publisher.Exchange, cfg.Publisher.ContentType)
	svc, err := initNotifyService(&cfg.Service, db, tm, rdb, multiSender, publisher, log)
	if err != nil {
		return fmt.Errorf("init service: %w", err)
	}
	eg, ctx := errgroup.WithContext(ctx)

	handler := httpt.NewNotifyHandler(svc, log)
	httpServer := httpt.NewHTTPServer(handler, &cfg.HTTP, log)
	eg.Go(func() error {
		return httpServer.Start(ctx)
	})

	eg.Go(func() error {
		startQueueProcessor(ctx, log, svc)
		return nil
	})

	for _, ch := range entity.ListChannels() {
		eg.Go(func() error {
			return runConsumer(ctx, svc, rmq, log, ch.String())
		})
	}

	if egErr := eg.Wait(); egErr != nil && !errors.Is(egErr, context.Canceled) {
		return fmt.Errorf("app execution failed: %w", egErr)
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

// initTelegramSender returns (*sender.TelegramSender, error) if found, (nil, nil) if token is not set
// and (nil, error) in case access/telegram errors
// nolint: nilnil
func initTelegramSender(cfg *config.TG, log logger.Logger) (*sender.TelegramSender, error) {
	if cfg.Token == "" {
		log.Warn("telegram sender disabled: token not configured")
		return nil, nil
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

func runConsumer(
	ctx context.Context,
	svc *service.NotifyService,
	client *rabbitmq.RabbitClient,
	log logger.Logger,
	ch string,
) error {
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
	log.Info("starting consumer", "channel", ch)

	err := consumer.Start(ctx)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, rabbitmq.ErrClientClosed) {
		return fmt.Errorf("consumer %s error: %w", ch, err)
	}
	return nil
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

	for _, ch := range entity.ListChannels() {
		name := ch.String()
		if err := client.DeclareQueue(
			name,
			exchangeName,
			name,
			true,
			false,
			true,
			nil,
		); err != nil {
			return fmt.Errorf("declare queue %s: %w", name, err)
		}
	}

	return nil
}

func startQueueProcessor(ctx context.Context, log logger.Logger, svc *service.NotifyService) {
	ticker := time.NewTicker(_queueProcessorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := svc.ProcessQueue(ctx)
			if err != nil {
				log.Error("ProcessQueue failed", "error", err)
				continue
			}
			if stats.Processed > 0 {
				log.Info("Queue processed", "stats", stats)
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
	telegramRepo := repository.NewTelegramRepository(db)
	cacheRepo := repository.NewCacheRepository(rdb)

	svc, err := service.NewNotifyService(
		notifyRepo,
		telegramRepo,
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
