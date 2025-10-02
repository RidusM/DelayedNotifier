package logger

import (
	"context"
	"fmt"
	"time"

	"github.com/ridusm/delayednotifier/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	_argPairs = 2
)

type Adapter struct {
	zapLogger *ZapLogger
}

func NewAdapter(cfg *config.Config, opts ...Option) (*Adapter, error) {
	logger, err := NewZapLogger(cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("logger.adapter.NewAdapter: %w", err)
	}
	return &Adapter{
		zapLogger: logger,
	}, nil
}

func (a *Adapter) Debug(msg string, args ...any) {
	a.zapLogger.Zap().Sugar().Debugw(msg, args...)
}

func (a *Adapter) Info(msg string, args ...any) {
	a.zapLogger.Zap().Sugar().Infow(msg, args...)
}

func (a *Adapter) Warn(msg string, args ...any) {
	a.zapLogger.Zap().Sugar().Warnw(msg, args...)
}

func (a *Adapter) Error(msg string, args ...any) {
	a.zapLogger.Zap().Sugar().Errorw(msg, args...)
}

func (a *Adapter) Debugw(msg string, keysAndValues ...any) {
	a.zapLogger.Zap().Sugar().Debugw(msg, keysAndValues...)
}

func (a *Adapter) Infow(msg string, keysAndValues ...any) {
	a.zapLogger.Zap().Sugar().Infow(msg, keysAndValues...)
}

func (a *Adapter) Warnw(msg string, keysAndValues ...any) {
	a.zapLogger.Zap().Sugar().Warnw(msg, keysAndValues...)
}

func (a *Adapter) Errorw(msg string, keysAndValues ...any) {
	a.zapLogger.Zap().Sugar().Errorw(msg, keysAndValues...)
}

func (a *Adapter) Ctx(ctx context.Context) Logger {
	return &Adapter{
		zapLogger: &ZapLogger{
			logger: a.zapLogger.NewContextLogger(ctx),
		},
	}
}

func (a *Adapter) With(args ...any) Logger {
	newAdapter := &Adapter{zapLogger: &ZapLogger{}}
	newAdapter.zapLogger.logger = a.zapLogger.Zap().With(toZapFields(args)...)
	return newAdapter
}

func (a *Adapter) WithGroup(name string) Logger {
	newAdapter := &Adapter{zapLogger: &ZapLogger{}}
	newAdapter.zapLogger.logger = a.zapLogger.Zap().With(zap.Namespace(name))
	return newAdapter
}

func (a *Adapter) Log(level Level, msg string, attrs ...Attr) {
	zapLevel := toZapLevel(level)
	if !a.zapLogger.Zap().Core().Enabled(zapLevel) {
		return
	}
	a.zapLogger.Zap().Log(zapLevel, msg, toZapFieldsFromAttrs(attrs)...)
}

func (a *Adapter) LogAttrs(ctx context.Context, level Level, msg string, attrs ...Attr) {
	logger := a.zapLogger.NewContextLogger(ctx)
	zapLevel := toZapLevel(level)

	if !logger.Core().Enabled(zapLevel) {
		return
	}

	logger.Log(zapLevel, msg, toZapFieldsFromAttrs(attrs)...)
}

func (a *Adapter) Level() Level {
	return InfoLevel
}

func (a *Adapter) GenerateRequestID() string {
	return a.zapLogger.GenerateRequestID()
}

func (a *Adapter) GetRequestID(ctx context.Context) string {
	return a.zapLogger.GetRequestID(ctx)
}

func (a *Adapter) WithRequestID(ctx context.Context, requestID string) context.Context {
	return a.zapLogger.WithRequestID(ctx, requestID)
}

func (a *Adapter) LogRequest(
	ctx context.Context,
	method, path string,
	status int,
	duration time.Duration,
) {
	a.zapLogger.LogRequest(ctx, method, path, status, duration)
}

func toZapLevel(level Level) zapcore.Level {
	switch level {
	case DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func toZapFields(args []any) []zap.Field {
	if len(args)%2 != 0 {
		args = append(args, "<missing>")
	}
	fields := make([]zap.Field, 0, len(args)/_argPairs)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			key = "UNKNOWN"
		}
		fields = append(fields, zap.Any(key, args[i+1]))
	}
	return fields
}

func toZapFieldsFromAttrs(attrs []Attr) []zap.Field {
	fields := make([]zap.Field, 0, len(attrs))
	for _, a := range attrs {
		fields = append(fields, zap.Any(a.Key, a.Value))
	}
	return fields
}
