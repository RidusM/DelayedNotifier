package logger

import (
	"fmt"
	"os"

	"github.com/ridusm/delayednotifier/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	_defaultMaxSize    = 100
	_defaultMaxBackups = 7
	_defaultMaxAge     = 30
)

type ZapLogger struct {
	logger *zap.Logger
	level  zapcore.Level

	maxSize    int
	maxBackups int
	maxAge     int
}

func NewZapLogger(cfg *config.Config, opts ...Option) (*ZapLogger, error) {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "ts",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		FunctionKey:   zapcore.OmitKey,
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.LowercaseLevelEncoder,
		EncodeTime:    zapcore.ISO8601TimeEncoder,
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}

	logger := &ZapLogger{
		maxSize:    _defaultMaxSize,
		maxBackups: _defaultMaxBackups,
		maxAge:     _defaultMaxAge,
		level:      zapcore.InfoLevel,
	}

	for _, opt := range opts {
		opt(logger)
	}

	lumberSync := &lumberjack.Logger{
		Filename:   cfg.Logger.Filename,
		MaxSize:    logger.maxSize,
		MaxBackups: logger.maxBackups,
		MaxAge:     logger.maxAge,
		Compress:   true,
	}

	if err := logger.validate(); err != nil {
		return nil, fmt.Errorf("logger.newZapLogger: validation: %w", err)
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(
			zapcore.AddSync(lumberSync),
			zapcore.AddSync(os.Stdout),
		),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= logger.level
		}),
	)

	logger = &ZapLogger{
		logger: zap.New(core,
			zap.Fields(
				zap.String("service", cfg.App.Name),
				zap.String("env", cfg.Env),
			),
			zap.AddCaller(),
			zap.AddStacktrace(zap.ErrorLevel),
		),
	}

	return logger, nil
}

func (l *ZapLogger) Zap() *zap.Logger {
	return l.logger
}
