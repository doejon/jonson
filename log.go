package jonson

import "reflect"

// Logger is used within jonson to create log outputs
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// NoOpLogger is a logger which does not output any
// logging information
type NoOpLogger struct {
}

func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

var _ Logger = (&NoOpLogger{})

func (n *NoOpLogger) Info(msg string, args ...any) {
}

func (n *NoOpLogger) Warn(msg string, args ...any) {
}

func (n *NoOpLogger) Error(msg string, args ...any) {
}

type loggerProvider struct {
	logger Logger
}

func newLoggerProvider(logger Logger) *loggerProvider {
	return &loggerProvider{
		logger: logger,
	}
}

func (l *loggerProvider) NewLogger(ctx *Context) Logger {
	return l.logger
}

var TypeLogger = reflect.TypeOf((*Logger)(nil)).Elem()

// RequireLogger allows you to require the logger provided
// during initialization.
// In case no logger was provided, a NoOpLogger will be returned.
// The logger will be available by default.
func RequireLogger(ctx *Context) Logger {
	if v := ctx.Require(TypeLogger); v != nil {
		return v.(Logger)
	}
	return nil
}
