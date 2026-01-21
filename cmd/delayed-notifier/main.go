package main

import (
	"context"
	"delayednotifier/internal/app"
	"delayednotifier/internal/config"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	cleanenvport "github.com/wb-go/wbf/config/cleanenv-port"
	"github.com/wb-go/wbf/logger"
)

func main() {
	var cfg config.Config

	if err := cleanenvport.Load(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewZapAdapter(cfg.App.Name, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Infow("application starting", "version", cfg.App.Version)

	if err := app.Run(ctx, &cfg, log); err != nil {
		log.Errorw("application terminated with error", "error", err)
		os.Exit(1)
	}

	log.Infow("application stopped gracefully")
}
