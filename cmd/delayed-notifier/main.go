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
			log, _ := logger.NewZapAdapter("delayed-notifier", "production")
			if log != nil {
				log.Errorw("PANIC RECOVERED", "panic", r)
			} else {
				fmt.Fprintf(os.Stderr, "PANIC (no logger): %v\n", r)
			}
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var cfg config.Config
	if err := cleanenvport.Load(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "critical: config load failed: %v\n", err)
	}

	log, err := logger.NewZapAdapter(cfg.App.Name, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "critical: logger init failed: %v\n", err)
	}

	if err = app.Run(ctx, &cfg, log); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "critical: application crashed: %v\n", err)
	}

	log.Infow("shutdown complete")
}
