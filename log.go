package jonson

import (
	"io"
	"log/slog"
	"reflect"
)

func NewNoOpLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

type loggerProvider struct {
	logger *slog.Logger
}

func newLoggerProvider(logger *slog.Logger) *loggerProvider {
	return &loggerProvider{
		logger: logger,
	}
}

func (l *loggerProvider) NewLogger(ctx *Context) *slog.Logger {
	return l.logger
}

var TypeLogger = reflect.TypeOf((**slog.Logger)(nil)).Elem()

// RequireLogger allows you to require the logger provided
// during initialization.
// In case no logger was provided, a NoOpLogger will be returned.
// The logger will be available by default.
func RequireLogger(ctx *Context) *slog.Logger {
	if v := ctx.Require(TypeLogger); v != nil {
		return v.(*slog.Logger)
	}
	return nil
}
