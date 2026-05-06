package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"delayednotifier/internal/app"
	"delayednotifier/internal/config"

	cleanenvport "github.com/wb-go/wbf/config/cleanenv-port"
	"github.com/wb-go/wbf/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var log logger.Logger
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			if log != nil {
				log.Error("PANIC RECOVERED",
					"panic", r,
					"stack", stack,
				)
			} else {
				fmt.Fprintf(os.Stderr, "PANIC RECOVERED: %v\n%s\n", r, stack)
			}
			os.Exit(1)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var cfg config.Config
	if err := cleanenvport.Load(&cfg); err != nil {
		return fmt.Errorf("config load: %w", err)
	}

	log, err := logger.NewZapAdapter(cfg.App.Name, cfg.Env)
	if err != nil {
		return fmt.Errorf("logger init: %w", err)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "starting application",
		logger.String("name", cfg.App.Name),
		logger.String("version", cfg.App.Version),
		logger.String("env", cfg.Env),
		logger.String("http_addr", cfg.HTTP.Host+":"+cfg.HTTP.Port),
	)

	if appErr := app.Run(ctx, &cfg, log); appErr != nil {
		if errors.Is(appErr, context.Canceled) {
			log.LogAttrs(ctx, logger.InfoLevel, "application stopped gracefully")
			return nil
		}
		return fmt.Errorf("app run: %w", appErr)
	}

	log.LogAttrs(ctx, logger.InfoLevel, "shutdown complete")
	return nil
}
