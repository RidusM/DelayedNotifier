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
	_strategyAttempts = 3
	_strategyDelay    = 3 * time.Second
	_strategyBackoff  = 2.0
	_cacheTTL         = 5 * time.Minute
)

func Run(ctx context.Context, cfg *config.Config, log logger.Logger) error {
	var (
		db  *pgxdriver.Postgres
		rdb *redis.Client
		rmq *rabbitmq.Publisher
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

	rmq, err = initPublisher(&cfg.Publisher)
	if err != nil {
		return fmt.Errorf("init rabbitmq publisher: %w", err)
	}
	log.Info("RabbitMQ publisher initialized successfully")

	svc, err = initNotifyService(&cfg.Service, db, tm, rdb, multiSender, rmq, log)
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

func initPublisher(cfg *config.Publisher) (*rabbitmq.Publisher, error) {
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
		ReconnectStrat: strategy,
	}

	client, err := rabbitmq.NewClient(rmqCfg)
	if err != nil {
		return nil, fmt.Errorf("create rabbitmq client: %w", err)
	}

	publisher := rabbitmq.NewPublisher(client, cfg.Exchange, cfg.ContentType)
	return publisher, nil
}

func initQueueProcessor(
	ctx context.Context,
	log logger.Logger,
	service *service.NotifyService,
) {
	ticker := time.NewTicker(5 * time.Second)
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
