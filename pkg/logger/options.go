package logger

import (
	"errors"

	"go.uber.org/zap/zapcore"
)

type Option func(*ZapLogger)

func MaxSize(size int) Option {
	return func(cfg *ZapLogger) {
		cfg.maxSize = size
	}
}

func MaxBackups(backups int) Option {
	return func(cfg *ZapLogger) {
		cfg.maxBackups = backups
	}
}

func MaxAge(age int) Option {
	return func(cfg *ZapLogger) {
		cfg.maxAge = age
	}
}

func SetLevel(level zapcore.Level) Option {
	return func(cfg *ZapLogger) {
		cfg.level = level
	}
}

func (cfg *ZapLogger) validate() error {
	if cfg.maxSize <= 0 {
		return errors.New("invalid maxSize: must be > 0")
	}

	if cfg.maxBackups <= 0 {
		return errors.New("invalid maxBackups: must be > 0")
	}

	if cfg.maxAge <= 0 {
		return errors.New("invalid maxAge: must be > 0")
	}
	return nil
}
