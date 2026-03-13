package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
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

	defer func() {
		if r := recover(); r != nil {
			log.Error("PANIC RECOVERED", "panic", r)
		}
	}()

	log.Info("starting application...")

	if appErr := app.Run(ctx, &cfg, log); appErr != nil {
		if errors.Is(appErr, context.Canceled) {
			log.Info("application stopped gracefully")
			return nil
		}
		return fmt.Errorf("app run: %w", appErr)
	}

	log.Info("shutdown complete")
	return nil
}
