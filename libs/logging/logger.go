package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"

	"gopkg.in/natefinch/lumberjack.v2"
)

// NewDiscardLogger returns a *slog.Logger that discards all output.
func NewDiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// LogRecovery logs a recovered panic with stack trace using the provided *slog.Logger.
func LogRecovery(logger *slog.Logger, msg string, r any) {
	if r == nil {
		return
	}
	err, ok := r.(error)
	if !ok {
		err = fmt.Errorf("%v", r)
	}
	stack := debug.Stack()
	if msg == "" {
		msg = "panic caught"
	}
	logger.LogAttrs(context.Background(), slog.LevelError, msg, slog.String("error", err.Error()), slog.String("stack", string(stack)))
}

// contextKey is a private type used for context keys to avoid collisions.
type contextKey int

const loggerContextKey contextKey = 1

// ContextWithLogger stores a *slog.Logger in the context and returns a new context.
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// LoggerFromContext extracts a *slog.Logger from the context.
// If no logger is found, it returns nil.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	logger, _ := ctx.Value(loggerContextKey).(*slog.Logger)
	return logger
}

// LoggerFromContextOrDefault extracts a *slog.Logger from the context.
// If no logger is found, it returns the provided default logger.
func LoggerFromContextOrDefault(ctx context.Context, defaultLogger *slog.Logger) *slog.Logger {
	if logger := LoggerFromContext(ctx); logger != nil {
		return logger
	}
	return defaultLogger
}

// CreateSlogLogger creates a *slog.Logger from the given slog.Handler.
// It returns the logger and an io.Closer for the file output (nil if no file logging).
func CreateSlogLogger(handler slog.Handler) *slog.Logger {
	return slog.New(handler)
}

// nopWriteCloser wraps an io.Writer with a no-op Close method.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// teeWriteCloser wraps an io.Writer and closes an underlying io.Closer on Close.
type teeWriteCloser struct {
	io.Writer

	closer io.Closer
}

func (t teeWriteCloser) Close() error {
	if err := t.closer.Close(); err != nil {
		return fmt.Errorf("closing tee writer: %w", err)
	}
	return nil
}

// GetLoggingWriter assembles an io.WriteCloser from the Config fields.
// It combines console and file writers as configured. Close is a no-op
// when file logging is not enabled.
func GetLoggingWriter(cfg *Config, consoleWriter io.Writer) (io.WriteCloser, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var consoleOutput io.Writer
	if !cfg.NoConsoleLog && consoleWriter != nil {
		consoleOutput = consoleWriter
	}

	var lumberjackLogger *lumberjack.Logger

	if fileConfig := cfg.File; fileConfig != nil && !fileConfig.Disable && fileConfig.Path != "" {
		lumberjackLogger = NewLumberjackLogger(fileConfig)
	}

	switch {
	case consoleOutput != nil && lumberjackLogger != nil:
		return teeWriteCloser{
			Writer: io.MultiWriter(consoleOutput, lumberjackLogger),
			closer: lumberjackLogger,
		}, nil
	case lumberjackLogger != nil:
		return lumberjackLogger, nil
	case consoleOutput != nil:
		return nopWriteCloser{consoleOutput}, nil
	default:
		return nil, errors.New("GetLoggingWriter: consoleWriter is required when file logging is disabled")
	}
}

// NewLumberjackLogger creates a *lumberjack.Logger from a FileConfig and
// optionally rotates the log file on start.
func NewLumberjackLogger(cfg *FileConfig) *lumberjack.Logger {
	l := &lumberjack.Logger{
		Filename:   cfg.Path,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	if cfg.RotateOnStart {
		_ = l.Rotate()
	}

	return l
}
