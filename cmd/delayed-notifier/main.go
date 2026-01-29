package main

import (
	"context"
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
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	var cfg config.Config

	if err := cleanenvport.Load(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		cancel()
		os.Exit(1)
	}

	log, err := logger.NewZapAdapter(cfg.App.Name, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		cancel()
		os.Exit(1)
	}

	log.Infow("application starting", "version", cfg.App.Version)

	if appErr := app.Run(ctx, &cfg, log); err != nil {
		log.Errorw("application terminated with error", "error", appErr)
		cancel()
		os.Exit(1)
	}

	log.Infow("application stopped gracefully")
	cancel()
}
