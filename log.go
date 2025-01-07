package jonson

import (
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
)

func NewNoOpLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

type LoggerOptions struct {
	// Initializer will be called directly after the logger has been provided
	// You can either provide your own initializer or use for example LoggerOptions.WithCallingFunction
	// which will add the current function the logger has been required to the log output
	Initializer []func(ctx *Context, logger *slog.Logger) *slog.Logger
}

// WithCallerFunction logs the current function to the output where the logger has been requred.
// In case you provide a key, the default key "function" in the log output will replaced with your provided key
func (l *LoggerOptions) WithCallerFunction(key ...string) *LoggerOptions {
	k := "function"
	for _, v := range key {
		k = v
	}
	type log struct {
		Name   string `json:"name"`
		Struct string `json:"struct"`
	}

	l.Initializer = append(l.Initializer, func(ctx *Context, logger *slog.Logger) *slog.Logger {
		pc := make([]uintptr, 1)
		runtime.Callers(3, pc)
		frames := runtime.CallersFrames(pc)
		frame, _ := frames.Next()

		// extract func name
		parts := strings.Split(frame.Function, ".")
		funcName := parts[len(parts)-1]

		// extract struct func name
		parts = strings.Split(frame.Function, "/")
		structFunc := parts[len(parts)-1]

		return logger.With(k, log{Name: funcName, Struct: structFunc})
	})
	return l
}

// WithCallerRpcMeta allows you to log the rpc meta by default to the log output
// Specify a key in case you do not want to use the default key "endpoint" for the log output
func (l *LoggerOptions) WithCallerRpcMeta(key ...string) *LoggerOptions {
	k := "rpcMeta"
	for _, v := range key {
		k = v
	}

	l.Initializer = append(l.Initializer, func(ctx *Context, logger *slog.Logger) *slog.Logger {
		meta := RequireRpcMeta(ctx)
		return logger.With(k, struct {
			Source     string `json:"source"`
			Method     string `json:"method"`
			HttpMethod string `json:"httpMethod"`
		}{
			Source:     string(meta.Source),
			Method:     string(meta.Method),
			HttpMethod: string(meta.HttpMethod),
		})
	})
	return l
}

type loggerProvider struct {
	logger  *slog.Logger
	options *LoggerOptions
}

func newLoggerProvider(logger *slog.Logger, options *LoggerOptions) *loggerProvider {
	return &loggerProvider{
		logger:  logger,
		options: options,
	}
}

func (l *loggerProvider) NewLogger(ctx *Context) *slog.Logger {
	return l.logger
}

func (l *loggerProvider) NewLoggerOptions(ctx *Context) *LoggerOptions {
	return l.options
}

var TypeLogger = reflect.TypeOf((**slog.Logger)(nil)).Elem()

var TypeLoggerOptions = reflect.TypeOf((**LoggerOptions)(nil)).Elem()

// RequireLogger allows you to require the logger provided
// during initialization.
// In case no logger was provided, a NoOpLogger will be returned.
// The logger will be available by default.
func RequireLogger(ctx *Context) *slog.Logger {
	if v := ctx.Require(TypeLogger); v != nil {
		log := v.(*slog.Logger)
		opts := requireLoggerOptions(ctx)
		for _, v := range opts.Initializer {
			log = v(ctx, log)
		}
		return log
	}
	return nil
}

// requireLoggerOptions returns available logger options.
// this function is only used internally and hence not exposed
func requireLoggerOptions(ctx *Context) *LoggerOptions {
	if v := ctx.Require(TypeLoggerOptions); v != nil {
		return v.(*LoggerOptions)
	}
	return nil
}
