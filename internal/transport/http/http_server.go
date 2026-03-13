package httpt

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"delayednotifier/internal/config"

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
) *HTTPServer {
	return &HTTPServer{
		server: &http.Server{
			Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
			Handler:           handler.Engine(),
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			MaxHeaderBytes:    cfg.MaxHeaderBytes,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		log:             log,
	}
}

func (s *HTTPServer) Start(ctx context.Context) error {
	const op = "transport.http.HTTPServer.Start"

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		s.log.LogAttrs(ctx, logger.InfoLevel, "starting HTTP server",
			logger.String("addr", s.server.Addr),
		)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("%s: listen and serve: %w", op, err)
		}
		return nil
	})

	eg.Go(func() error {
		<-ctx.Done()
		return s.Stop(context.Background())
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *HTTPServer) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()

	s.log.LogAttrs(ctx, logger.InfoLevel, "shutting down HTTP server")
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		s.log.LogAttrs(ctx, logger.ErrorLevel, "HTTP server forced shutdown",
			logger.Any("error", err),
		)
		return fmt.Errorf("transport.http.HTTPServer.Stop: server shutdown: %w", err)
	}
	s.log.LogAttrs(ctx, logger.InfoLevel, "HTTP server stopped gracefully")
	return nil
}
