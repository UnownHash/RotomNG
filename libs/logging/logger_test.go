package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestConfig_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected Config
	}{
		{
			name:   "empty config",
			config: Config{},
			expected: Config{
				Level:  levelInfo,
				Format: formatPlain,
			},
		},
		{
			name: "partial config with level",
			config: Config{
				Level: levelDebug,
			},
			expected: Config{
				Level:  levelDebug,
				Format: formatPlain,
			},
		},
		{
			name: "partial config with format",
			config: Config{
				Format: formatJSON,
			},
			expected: Config{
				Level:  levelInfo,
				Format: formatJSON,
			},
		},
		{
			name: "full config",
			config: Config{
				Level:  levelError,
				Format: formatJSON,
			},
			expected: Config{
				Level:  levelError,
				Format: formatJSON,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config
			cfg.SetDefaults()

			if cfg.Level != tt.expected.Level {
				t.Errorf("Expected Level %s, got %s", tt.expected.Level, cfg.Level)
			}
			if cfg.Format != tt.expected.Format {
				t.Errorf("Expected Format %s, got %s", tt.expected.Format, cfg.Format)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		expectErr bool
		errMsg    string
	}{
		{
			name:      "empty config",
			config:    Config{},
			expectErr: false,
		},
		{
			name: "valid log level - info",
			config: Config{
				Level:  levelInfo,
				Format: formatPlain,
			},
			expectErr: false,
		},
		{
			name: "valid log level - debug",
			config: Config{
				Level:  levelDebug,
				Format: formatJSON,
			},
			expectErr: false,
		},
		{
			name: "valid log level - warning",
			config: Config{
				Level:  levelWarning,
				Format: formatPlain,
			},
			expectErr: false,
		},
		{
			name: "invalid log level",
			config: Config{
				Level:  "invalid",
				Format: formatPlain,
			},
			expectErr: true,
			errMsg:    "invalid log level 'invalid'",
		},
		{
			name: "invalid format",
			config: Config{
				Level:  levelInfo,
				Format: "xml",
			},
			expectErr: true,
			errMsg:    "invalid log format 'xml'",
		},
		{
			name: "case insensitive log level",
			config: Config{
				Level:  "INFO",
				Format: formatPlain,
			},
			expectErr: false,
		},
		{
			name: "case insensitive format",
			config: Config{
				Level:  levelInfo,
				Format: "JSON",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			err := tt.config.Validate()

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidLevels(t *testing.T) {
	expectedLevels := []string{
		levelPanic, levelFatal, levelError, levelWarn, levelWarning, levelInfo, levelDebug, levelTrace,
	}

	for _, level := range expectedLevels {
		if !validLevels[level] {
			t.Errorf("Expected level '%s' to be valid", level)
		}
	}

	// Test invalid level
	if validLevels["invalid"] {
		t.Error("Expected 'invalid' to not be a valid level")
	}
}

// Tests for context methods (updated to use *slog.Logger).
func TestContextWithLogger(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	ctx := context.Background()

	// Test storing logger in context
	ctxWithLogger := ContextWithLogger(ctx, logger)

	// Verify the context is different
	if ctx == ctxWithLogger {
		t.Error("Expected new context to be different from original")
	}

	// Verify we can retrieve the logger
	retrievedLogger := LoggerFromContext(ctxWithLogger)
	if retrievedLogger != logger {
		t.Error("Expected retrieved logger to be the same as stored logger")
	}
}

func TestLoggerFromContext(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	tests := []struct {
		name         string
		setupContext func() context.Context
		expectLogger bool
	}{
		{
			name: "context with logger",
			setupContext: func() context.Context {
				return ContextWithLogger(context.Background(), logger)
			},
			expectLogger: true,
		},
		{
			name:         "context without logger",
			setupContext: context.Background,
			expectLogger: false,
		},
		{
			name: "context with nil logger",
			setupContext: func() context.Context {
				return ContextWithLogger(context.Background(), nil)
			},
			expectLogger: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupContext()
			retrievedLogger := LoggerFromContext(ctx)

			if tt.expectLogger {
				if retrievedLogger == nil {
					t.Error("Expected logger to be found in context")
				}
			} else {
				if retrievedLogger != nil {
					t.Error("Expected no logger to be found in context")
				}
			}
		})
	}
}

func TestLoggerFromContextOrDefault(t *testing.T) {
	defaultLogger := slog.New(slog.DiscardHandler)
	contextLogger := slog.New(slog.DiscardHandler)

	tests := []struct {
		name          string
		setupContext  func() context.Context
		expectDefault bool
	}{
		{
			name: "context with logger",
			setupContext: func() context.Context {
				return ContextWithLogger(context.Background(), contextLogger)
			},
			expectDefault: false,
		},
		{
			name:          "context without logger",
			setupContext:  context.Background,
			expectDefault: true,
		},
		{
			name: "context with nil logger",
			setupContext: func() context.Context {
				return ContextWithLogger(context.Background(), nil)
			},
			expectDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupContext()
			logger := LoggerFromContextOrDefault(ctx, defaultLogger)

			if logger == nil {
				t.Fatal("Expected logger to never be nil")
			}

			if tt.expectDefault {
				if logger != defaultLogger {
					t.Error("Expected default logger to be returned")
				}
			} else {
				if logger == defaultLogger {
					t.Error("Expected context logger to be returned, not default")
				}
			}
		})
	}
}

// Benchmark tests.
func BenchmarkContextWithLogger(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		ContextWithLogger(ctx, logger)
	}
}

func BenchmarkLoggerFromContext(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)
	ctx := ContextWithLogger(context.Background(), logger)

	b.ResetTimer()
	for range b.N {
		LoggerFromContext(ctx)
	}
}

func BenchmarkLoggerFromContextOrDefault(b *testing.B) {
	logger := slog.New(slog.DiscardHandler)
	defaultLogger := slog.New(slog.DiscardHandler)
	ctx := ContextWithLogger(context.Background(), logger)

	b.ResetTimer()
	for range b.N {
		LoggerFromContextOrDefault(ctx, defaultLogger)
	}
}

func BenchmarkConfig_Validate(b *testing.B) {
	config := Config{
		Level:  levelInfo,
		Format: formatJSON,
	}
	config.SetDefaults()

	b.ResetTimer()
	for range b.N {
		err := config.Validate()
		if err != nil {
			b.Fatalf("Expected no error, got: %v", err)
		}
	}
}

// --- slog-based context helper and LogRecovery tests ---

func TestContextWithLogger_SlogRoundTrip(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	ctx := ContextWithLogger(context.Background(), logger)
	got := LoggerFromContext(ctx)
	if got != logger {
		t.Errorf("Expected same *slog.Logger pointer, got different")
	}
}

func TestLoggerFromContext_NilWhenMissing(t *testing.T) {
	got := LoggerFromContext(context.Background())
	if got != nil {
		t.Errorf("Expected nil when no logger in context, got %v", got)
	}
}

func TestLoggerFromContextOrDefault_ReturnsSlogDefault(t *testing.T) {
	defaultLogger := slog.New(slog.DiscardHandler)
	got := LoggerFromContextOrDefault(context.Background(), defaultLogger)
	if got != defaultLogger {
		t.Errorf("Expected default logger when context has no logger")
	}
}

func TestLoggerFromContextOrDefault_ReturnsContextLogger(t *testing.T) {
	ctxLogger := slog.New(slog.DiscardHandler)
	defaultLogger := slog.New(slog.DiscardHandler)
	ctx := ContextWithLogger(context.Background(), ctxLogger)
	got := LoggerFromContextOrDefault(ctx, defaultLogger)
	if got != ctxLogger {
		t.Errorf("Expected context logger, got default")
	}
	if got == defaultLogger {
		t.Errorf("Should not have returned the default logger")
	}
}

func TestLogRecovery_WithSlogLogger(t *testing.T) {
	var buf bytes.Buffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelDebug)
	handler := NewSlogHandler(&buf, "plain", &SlogHandlerOptions{
		Level: levelVar,
	})
	logger := slog.New(handler)

	LogRecovery(logger, "test panic", errors.New("something went wrong"))

	output := buf.String()
	if !strings.Contains(output, "something went wrong") {
		t.Errorf("Expected output to contain error message, got: %s", output)
	}
	if !strings.Contains(output, "stack") {
		t.Errorf("Expected output to contain 'stack' attribute, got: %s", output)
	}
}

func TestLogRecovery_NilRecovery_Slog(t *testing.T) {
	var buf bytes.Buffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelDebug)
	handler := NewSlogHandler(&buf, "plain", &SlogHandlerOptions{
		Level: levelVar,
	})
	logger := slog.New(handler)

	LogRecovery(logger, "should not appear", nil)

	output := buf.String()
	if output != "" {
		t.Errorf("Expected no output for nil recovery, got: %s", output)
	}
}

func TestNewDiscardLogger_ReturnsSlogLogger(t *testing.T) {
	logger := NewDiscardLogger()
	if logger == nil {
		t.Fatal("Expected non-nil *slog.Logger from NewDiscardLogger")
	}
	// Should be able to log without error (output is discarded)
	logger.Info("this should be discarded")
}
