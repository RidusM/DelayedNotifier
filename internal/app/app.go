package app

import (
	"context"
	"delayednotifier/internal/config"
	"fmt"
	"net"
	"net/http"

	"github.com/golang-migrate/migrate/database/postgres"
	"github.com/ridusm/delayednotifier/pkg/metric"
	uuid "github.com/satori/go.uuid"
	"github.com/wb-go/wbf/dbpg/pgx-driver/transaction"
	"github.com/wb-go/wbf/logger"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg *config.Config, log logger.Logger) error {
	eg, ctx := errgroup.WithContext(ctx)

	metrics := initMetrics(eg, &cfg.Metrics, log)

	db, dbErr := initDatabase(&cfg.Postgres, log)
	if dbErr != nil {
		return dbErr
	}
	defer closeDB(db)

	txManager, txErr := initTransactionManager(
		db,
		log,
		metrics,
	)
	if txErr != nil {
		return txErr
	}

	orderCache, cacheErr := initCache(&cfg.Cache, log, metrics)
	if cacheErr != nil {
		return cacheErr
	}
	defer stopCache(orderCache)

	orderService := initOrderService(
		cfg,
		db,
		txManager,
		orderCache,
		log,
	)

	if err := orderService.RestoreCache(ctx); err != nil {
		log.Errorw("failed to restore cache from database", "error", err)
	}

	if serverErr := initHTTPServer(ctx, eg, &cfg.HTTP, orderService, log, metrics); serverErr != nil {
		return serverErr
	}

	if kafkaErr := initKafkaComponents(ctx, eg, cfg, orderService, log, metrics); kafkaErr != nil {
		return kafkaErr
	}

	return waitForShutdown(eg)
}

func initMetrics(
	eg *errgroup.Group,
	cfg *config.Metrics,
	log logger.Logger,
) metric.Factory {
	metrics := metric.NewFactory()

	hostPort := net.JoinHostPort(cfg.Host, cfg.Port)
	metricsServer := &http.Server{
		Addr:              hostPort,
		Handler:           metrics.Handler(),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	eg.Go(func() error {
		log.Infow("starting metrics server", "port", cfg.Port)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("app.initMetrics: %w", err)
		}
		return nil
	})

	return metrics
}

func initCache(
	cfg *config.Cache,
	log logger.Logger,
	metrics metric.Factory,
) (cache.Cache[uuid.UUID, *entity.Order], error) {
	orderCache, err := cache.NewLRUCache[uuid.UUID, *entity.Order](
		cfg.Capacity,
		log.With("component", "cache"),
		metrics.Cache(),
	)
	if err != nil {
		return nil, fmt.Errorf("app.initCache: %w", err)
	}
	orderCache.StartCleanup(cfg.CleanupInterval)
	return orderCache, nil
}

func stopCache(orderCache cache.Cache[uuid.UUID, *entity.Order]) {
	if orderCache != nil {
		orderCache.StopCleanup()
	}
}

func initOrderService(
	cfg *config.Config,
	db *postgres.Postgres,
	txManager transaction.Manager,
	orderCache cache.Cache[uuid.UUID, *entity.Order],
	log logger.Logger,
) *service.OrderService {
	orderRepo := repository.NewOrderRepository(db)
	deliveryRepo := repository.NewDeliveryRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	itemRepo := repository.NewItemRepository(db)

	orderService := service.NewOrderService(
		deliveryRepo,
		itemRepo,
		orderRepo,
		paymentRepo,
		txManager,
		log.With("component", "order service"),
		orderCache,
		cfg.Cache.TTL,
	)

	return orderService
}

func initHTTPServer(
	ctx context.Context,
	eg *errgroup.Group,
	cfg *config.HTTP,
	orderService *service.OrderService,
	log logger.Logger,
	metrics metric.Factory,
) error {
	httpServer, err := httpt.NewHTTPServer(
		httpt.NewOrderHandler(orderService, log, metrics.HTTP()),
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
