package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// --- NewDiscardLogger ---

func TestNewDiscardLogger(t *testing.T) {
	logger := NewDiscardLogger()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	// Should not panic when logging
	logger.Info("test message")
	logger.Debug("debug message")
}

// --- LogRecovery ---

func TestLogRecovery_WithError(t *testing.T) {
	var buf bytes.Buffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelDebug)
	handler := NewSlogHandler(&buf, "plain", &SlogHandlerOptions{Level: levelVar})
	logger := slog.New(handler)

	LogRecovery(logger, "something failed", errors.New("test error"))

	output := buf.String()
	if !strings.Contains(output, "something failed") {
		t.Errorf("expected 'something failed' in output, got: %s", output)
	}
	if !strings.Contains(output, "test error") {
		t.Errorf("expected 'test error' in output, got: %s", output)
	}
}

func TestLogRecovery_WithString(t *testing.T) {
	var buf bytes.Buffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelDebug)
	handler := NewSlogHandler(&buf, "plain", &SlogHandlerOptions{Level: levelVar})
	logger := slog.New(handler)

	LogRecovery(logger, "", "panic string")

	output := buf.String()
	if !strings.Contains(output, "panic caught") {
		t.Errorf("expected default 'panic caught' message, got: %s", output)
	}
}

func TestLogRecovery_NilRecovery(t *testing.T) {
	var buf bytes.Buffer
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelDebug)
	handler := NewSlogHandler(&buf, "plain", &SlogHandlerOptions{Level: levelVar})
	logger := slog.New(handler)

	LogRecovery(logger, "msg", nil)

	if buf.Len() != 0 {
		t.Error("expected no output for nil recovery")
	}
}

// --- FileConfig.Validate ---

func TestFileConfig_Validate_Disabled(t *testing.T) {
	cfg := &FileConfig{Disable: true}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected nil error for disabled config, got %v", err)
	}
}

func TestFileConfig_Validate_MissingPath(t *testing.T) {
	cfg := &FileConfig{Disable: false, Path: ""}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestFileConfig_Validate_WithPath(t *testing.T) {
	cfg := &FileConfig{Disable: false, Path: "/tmp/test.log"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// --- Config.Validate edge cases ---

func TestConfig_Validate_InvalidFormat(t *testing.T) {
	cfg := &Config{Level: "info", Format: "xml"}
	cfg.SetDefaults() // won't override since already set
	cfg.defaultsSet = true
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestConfig_Validate_DefaultsNotSet(t *testing.T) {
	cfg := &Config{Level: "info", Format: "plain"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when defaults not set")
	}
}

// --- Context helpers ---

func TestContextWithLogger_Coverage(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	ctx := ContextWithLogger(context.Background(), logger)
	got := LoggerFromContext(ctx)
	if got != logger {
		t.Error("expected same logger from context")
	}
}
