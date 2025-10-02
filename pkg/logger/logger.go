package logger

import (
	"context"
	"time"
)

type Level int

const (
	DebugLevel Level = iota - 4
	InfoLevel
	WarnLevel
	ErrorLevel
)

type (
	Attr struct {
		Key   string
		Value any
	}

	Logger interface {
		Debug(msg string, args ...any)
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
		Error(msg string, args ...any)

		Debugw(msg string, keysAndValues ...any)
		Infow(msg string, keysAndValues ...any)
		Warnw(msg string, keysAndValues ...any)
		Errorw(msg string, keysAndValues ...any)

		Ctx(ctx context.Context) Logger
		With(args ...any) Logger
		WithGroup(name string) Logger
		WithRequestID(ctx context.Context, requestID string) context.Context

		GenerateRequestID() string
		GetRequestID(ctx context.Context) string
		LogRequest(ctx context.Context, method, path string, status int, duration time.Duration)

		Log(level Level, msg string, attrs ...Attr)
		LogAttrs(ctx context.Context, level Level, msg string, attrs ...Attr)
	}
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func String(key string, value string) Attr {
	return Attr{Key: key, Value: value}
}

func Int(key string, value int) Attr {
	return Attr{Key: key, Value: value}
}

func Int64(key string, value int64) Attr {
	return Attr{Key: key, Value: value}
}

func Bool(key string, value bool) Attr {
	return Attr{Key: key, Value: value}
}

func Time(key string, value time.Time) Attr {
	return Attr{Key: key, Value: value}
}

func Any(key string, value any) Attr {
	return Attr{Key: key, Value: value}
}
