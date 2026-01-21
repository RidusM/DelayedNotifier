package httpt

import (
	"context"
	"delayednotifier/internal/config"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wb-go/wbf/logger"
	"golang.org/x/sync/errgroup"
)

type HTTPServer struct {
	server          *http.Server
	shutdownTimeout time.Duration
	log             logger.Logger
}

func NewHTTPServer(
	handler *NotifyHandler,
	cfg *config.HTTP,
	log logger.Logger,
) (*HTTPServer, error) {
	return &HTTPServer{
		server: &http.Server{
			Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
			Handler:           handler.Engine(),
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		log:             log,
	}, nil
}

func (s *HTTPServer) Start(ctx context.Context) error {
	const op = "transport.http.HTTPServer.Start"

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		s.log.Infow("starting HTTP server", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Errorw("HTTP server failed to start", "error", err)
			return fmt.Errorf("%s: server listen and serve: %w", op, err)
		}
		return nil
	})

	eg.Go(func() error {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-stop:
			s.log.Infow("shutdown signal received", "timeout", s.shutdownTimeout.String())
			return s.Stop(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("%s: error group wait: %w", op, err)
	}
	return nil
}

func (s *HTTPServer) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()

	s.log.Infow("shutting down HTTP server")
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		s.log.Errorw("HTTP server forced shutdown", "error", err)
		return fmt.Errorf("transport.http.HTTPServer.Stop: server shutdown: %w", err)
	}
	s.log.Infow("HTTP server stopped gracefully")
	return nil
}
