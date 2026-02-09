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
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
			os.Exit(2)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var cfg config.Config
	if err := cleanenvport.Load(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "critical: config load failed: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewZapAdapter(cfg.App.Name, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "critical: logger init failed: %v\n", err)
		os.Exit(1)
	}

	log.Infow("application starting",
		"version", cfg.App.Version,
		"env", cfg.Env,
		"pid", os.Getpid(),
	)

	if err := app.Run(ctx, &cfg, log); err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Errorw("application crashed", "error", err)
			os.Exit(1)
		}
	}

	log.Infow("shutdown complete")
}
