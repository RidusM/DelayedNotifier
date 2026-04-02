package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"delayednotifier/internal/config"
	"delayednotifier/internal/repository"
	"delayednotifier/internal/service"
	"delayednotifier/internal/transport/sender"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/suite"
	cleanenvport "github.com/wb-go/wbf/config/cleanenv-port"
	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/dbpg/pgx-driver/transaction"
	"github.com/wb-go/wbf/logger"
	"github.com/wb-go/wbf/rabbitmq"
	"github.com/wb-go/wbf/redis"
	"github.com/wb-go/wbf/retry"
)

const (
	_strategyAttempts = 3
	_strategyDelay    = 3 * time.Second
	_strategyBackoff  = 2.0
	_maxRetryAttempts = 10
	_ctxTimeout       = 60 * time.Second
	_baseTimeout      = 5 * time.Second
	_redisDB          = 0
)

type IntegrationTestSuite struct {
	suite.Suite

	db        *pgxdriver.Postgres
	rdb       *redis.Client
	rmq       *rabbitmq.RabbitClient
	notifySvc *service.NotifyService
	cfg       *config.Config
}

func (s *IntegrationTestSuite) SetupSuite() {
	var cfg config.Config
	err := cleanenvport.Load(&cfg)
	s.Require().NoError(err, "Failed to load configuration")
	s.cfg = &cfg

	testLogger, err := logger.NewZapAdapter(cfg.App.Name, cfg.Env)
	s.Require().NoError(err)

	var db *pgxdriver.Postgres
	for i := range _maxRetryAttempts {
		db, err = pgxdriver.New(cfg.Database.DSN, testLogger)
		if err == nil {
			break
		}
		testLogger.Info("Waiting for database...", "attempt", i+1)
		time.Sleep(_baseTimeout)
	}
	s.Require().NoError(err, "Failed to connect to DB")
	s.db = db

	rdb := redis.New(cfg.Cache.Addr, cfg.Cache.Password, _redisDB)
	s.rdb = rdb

	ctx, cancel := context.WithTimeout(context.Background(), _ctxTimeout)
	defer cancel()

	err = db.Pool.Ping(ctx)
	s.Require().NoError(err, "DB ping failed")

	strategy := retry.Strategy{
		Attempts: _strategyAttempts,
		Delay:    _strategyDelay,
		Backoff:  _strategyBackoff,
	}
	rmqCfg := rabbitmq.ClientConfig{
		URL:            cfg.Publisher.URL,
		ConnectionName: cfg.Publisher.ConnectionName,
		ConnectTimeout: cfg.Publisher.ConnectTimeout,
		Heartbeat:      cfg.Publisher.Heartbeat,
		ProducingStrat: strategy,
		ReconnectStrat: strategy,
	}

	rmq, err := rabbitmq.NewClient(rmqCfg)
	s.Require().NoError(err, "RabbitMQ init failed")
	s.rmq = rmq

	txManager, err := transaction.NewManager(db, testLogger)
	s.Require().NoError(err, "Transaction manager init failed")

	publisher := rabbitmq.NewPublisher(rmq, cfg.Publisher.Exchange, cfg.Publisher.ContentType)
	cacheRepo := repository.NewCacheRepository(rdb)
	userRepo := repository.NewTelegramRepository(db)
	notifyRepo := repository.NewNotifyRepository(db)
	notifySender := sender.NewMultiSender()

	s.notifySvc, err = service.NewNotifyService(
		notifyRepo,
		userRepo,
		cacheRepo,
		notifySender,
		txManager,
		publisher,
		testLogger,
		service.QueryLimit(cfg.Service.QueryLimit),
		service.MaxRetries(cfg.Service.MaxRetries),
		service.RetryDelay(cfg.Service.RetryDelay),
	)
	s.Require().NoError(err, "Notify service init failed")
}

func (s *IntegrationTestSuite) TearDownSuite() {
	if s.db != nil {
		s.db.Close()
	}
	if s.rmq != nil {
		s.rmq.Close()
	}
}

func (s *IntegrationTestSuite) TearDownTest() {
	ctx, cancel := context.WithTimeout(context.Background(), _ctxTimeout)
	defer cancel()

	_, err := s.db.Pool.Exec(ctx, `
		TRUNCATE TABLE notifications, telegram_subscribers RESTART IDENTITY;
	`)
	s.Require().NoError(err)
}

func TestIntegration(t *testing.T) {
	t.Parallel()
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Set INTEGRATION_TEST=1 to run integration tests")
	}
	suite.Run(t, new(IntegrationTestSuite))
}
