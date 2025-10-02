package logger

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	loggerKey    contextKey = "zap_logger"

	_httpStatusClassDiv = 100
)

func (l *ZapLogger) WithRequestID(ctx context.Context, requestID string) context.Context {
	ctx = context.WithValue(ctx, requestIDKey, requestID)

	logger := l.logger.With(zap.String("request_id", requestID))
	ctx = context.WithValue(ctx, loggerKey, logger)

	return ctx
}

func (l *ZapLogger) GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

func (l *ZapLogger) NewContextLogger(ctx context.Context) *zap.Logger {
	requestID := l.GetRequestID(ctx)
	if requestID == "" {
		return l.logger
	}

	if cached, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return cached
	}

	logger := l.logger.With(zap.String("request_id", requestID))

	return logger
}

func (l *ZapLogger) LogRequest(
	ctx context.Context,
	method, path string,
	status int,
	duration time.Duration,
) {
	logger := l.NewContextLogger(ctx)

	logger.Info("request",
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status", status),
		zap.Duration("duration", duration),
		zap.Int("status_class", status/_httpStatusClassDiv),
	)
}

func (l *ZapLogger) GenerateRequestID() string {
	return uuid.New().String()
}
