package app

import (
	"context"
	"fmt"
	"time"

	"delayednotifier/internal/config"
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
	_strategyBackoff  = 2
)

func Run(ctx context.Context, cfg *config.Config, log logger.Logger) error {
	eg, ctx := errgroup.WithContext(ctx)

	db, dbErr := initDatabase(&cfg.Database, log)
	if dbErr != nil {
		return dbErr
	}
	defer closeDB(db)

	tm, tmErr := initTransactionManager(
		db,
		log,
	)
	if tmErr != nil {
		return tmErr
	}

	rdb := initCache(&cfg.Cache)
	if rdbErr := closeCache(rdb); rdbErr != nil {
		return rdbErr
	}

	teleSender, teleErr := initTelegramSender(&cfg.TG, log)
	if teleErr != nil {
		return teleErr
	}
	emailSender := initEmailSender(&cfg.SMTP, log)

	multiSender := initMultiSender(teleSender, emailSender)

	rmq, rmqErr := initPublisher(&cfg.Publisher)
	if rmqErr != nil {
		return rmqErr
	}

	notifyService, svcErr := initNotifyService(
		&cfg.Service,
		db,
		tm,
		rdb,
		multiSender,
		rmq,
		log,
	)
	if svcErr != nil {
		return svcErr
	}

	if serverErr := initHTTPServer(ctx, eg, &cfg.HTTP, notifyService, log); serverErr != nil {
		return serverErr
	}

	return waitForShutdown(eg)
}

func initDatabase(cfg *config.Database, log logger.Logger) (*pgxdriver.Postgres, error) {
	db, err := pgxdriver.New(
		cfg.DSN,
		log.With("component", "database"),
		pgxdriver.MaxPoolSize(cfg.PoolMax),
		pgxdriver.MaxConnAttempts(cfg.ConnAttempts),
		pgxdriver.BaseRetryDelay(cfg.BaseRetryDelay),
		pgxdriver.MaxRetryDelay(cfg.MaxRetryDelay),
	)
	if err != nil {
		return nil, fmt.Errorf("app.initDatabase: %w", err)
	}
	return db, nil
}

func closeDB(db *pgxdriver.Postgres) {
	if db != nil {
		db.Close()
	}
}

func initTransactionManager(db *pgxdriver.Postgres, log logger.Logger) (transaction.Manager, error) {
	tm, err := transaction.NewManager(
		db,
		log,
	)
	if err != nil {
		return nil, fmt.Errorf("app.initTransactionManager: %w", err)
	}
	return tm, nil
}

func initCache(
	cfg *config.Cache,
) *redis.Client {
	client := redis.New(cfg.Addr, cfg.Password, 0)
	return client
}

func closeCache(rdb *redis.Client) error {
	if err := rdb.Close(); err != nil {
		return fmt.Errorf("app.closeCache: %w", err)
	}
	return nil
}

func initTelegramSender(cfg *config.TG, log logger.Logger) (*sender.TelegramSender, error) {
	tSender, err := sender.NewTelegramSender(cfg.Token, log)
	if err != nil {
		return nil, fmt.Errorf("app.initTelegramSender: %w", err)
	}
	return tSender, nil
}

func initEmailSender(cfg *config.SMTP, log logger.Logger) *sender.EmailSender {
	return sender.NewEmailSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From, log)
}

func initMultiSender(ts sender.NotificationSender, es sender.NotificationSender) *sender.MultiSender {
	return sender.NewMultiSender(ts, es)
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
		return nil, fmt.Errorf("app.initPublisher: init new RabbitMQ client: %w", err)
	}
	defer client.Close()

	publisher := rabbitmq.NewPublisher(client, cfg.Exchange, cfg.ContentType)
	return publisher, nil
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
	cacheRepo := repository.NewCacheRepository(rdb)
	notifyService, err := service.NewNotifyService(
		notifyRepo,
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
		return nil, fmt.Errorf("app.initNotifyService: %w", err)
	}
	return notifyService, nil
}

func initHTTPServer(
	ctx context.Context,
	eg *errgroup.Group,
	cfg *config.HTTP,
	svc *service.NotifyService,
	log logger.Logger,
) error {
	httpServer, err := httpt.NewHTTPServer(
		httpt.NewNotifyHandler(svc, log),
		cfg,
		log.With("component", "http server"),
	)
	if err != nil {
		return fmt.Errorf("app.initHTTPServer: %w", err)
	}

	eg.Go(func() error {
		return httpServer.Start(ctx)
	})
	return nil
}

func waitForShutdown(eg *errgroup.Group) error {
	if err := eg.Wait(); err != nil && !isShutdownSignal(err) {
		return fmt.Errorf("app.waitForShutdown: application failed: %w", err)
	}
	return nil
}

func isShutdownSignal(err error) bool {
	return err != nil && err.Error() == "shutdown signal"
}
