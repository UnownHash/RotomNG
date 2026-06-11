package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
)

func TestSlogHandler_PlainBasic(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)
	logger := slog.New(h)
	logger.Info("hello")
	line := buf.String()

	if !strings.Contains(line, "INFO") {
		t.Errorf("expected INFO level, got: %s", line)
	}
	if !strings.Contains(line, "[system]") {
		t.Errorf("expected [system] component, got: %s", line)
	}
	if !strings.HasSuffix(line, "hello\n") {
		t.Errorf("expected line to end with 'hello\\n', got: %q", line)
	}
	// Should NOT have "-- " when no extra attrs
	if strings.Contains(line, "-- ") {
		t.Errorf("expected no attrs separator, got: %s", line)
	}
}

func TestSlogHandler_PlainWithComponent(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)
	logger := slog.New(h)
	logger.With(slog.String("component", "web")).Info("req")
	line := buf.String()

	if !strings.Contains(line, "[web]") {
		t.Errorf("expected [web] component, got: %s", line)
	}
	if strings.Contains(line, "[system]") {
		t.Errorf("should not contain [system], got: %s", line)
	}
}

func TestSlogHandler_PlainWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)
	logger := slog.New(h)
	logger.Info("msg", slog.String("key", "val"))
	line := buf.String()

	if !strings.Contains(line, "-- key=val -- ") {
		t.Errorf("expected '-- key=val -- ', got: %s", line)
	}
}

func TestSlogHandler_PlainMultipleAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)
	logger := slog.New(h)
	logger.Info("msg", slog.String("beta", "2"), slog.String("alpha", "1"))
	line := buf.String()

	// attrs should be sorted alphabetically
	if !strings.Contains(line, "-- alpha=1, beta=2 -- ") {
		t.Errorf("expected sorted attrs '-- alpha=1, beta=2 -- ', got: %s", line)
	}
}

func TestSlogHandler_JSONBasic(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "json", nil)
	logger := slog.New(h)
	logger.Info("hello")
	line := buf.String()

	var result map[string]any
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v, line: %s", err, line)
	}

	if _, ok := result["timestamp"]; !ok {
		t.Error("missing 'timestamp' key")
	}
	if result["level"] != "INFO" {
		t.Errorf("expected level INFO, got: %v", result["level"])
	}
	if result["message"] != "hello" {
		t.Errorf("expected message 'hello', got: %v", result["message"])
	}
	fields, ok := result["fields"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'fields' key")
	}
	if fields["component"] != "system" {
		t.Errorf("expected fields.component 'system', got: %v", fields["component"])
	}
}

func TestSlogHandler_JSONWithComponent(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "json", nil)
	logger := slog.New(h)
	logger.With(slog.String("component", "api")).Info("test")
	line := buf.String()

	var result map[string]any
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	fields := result["fields"].(map[string]any)
	if fields["component"] != "api" {
		t.Errorf("expected component 'api', got: %v", fields["component"])
	}
}

func TestSlogHandler_JSONWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "json", nil)
	logger := slog.New(h)
	logger.Info("msg", slog.String("beta", "2"), slog.String("alpha", "1"))
	line := buf.String()

	var result map[string]any
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	fields := result["fields"].(map[string]any)
	if fields["alpha"] != "1" {
		t.Errorf("expected alpha=1, got: %v", fields["alpha"])
	}
	if fields["beta"] != "2" {
		t.Errorf("expected beta=2, got: %v", fields["beta"])
	}
}

func TestSlogHandler_JSONCompliance(t *testing.T) {
	var buf bytes.Buffer
	newHandler := func(_ *testing.T) slog.Handler {
		buf.Reset()
		return NewSlogHandler(&buf, "json", nil)
	}
	result := func(t *testing.T) map[string]any { //nolint:thelper // slogtest callback signature requires *testing.T
		line := buf.String()
		if line == "" {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("failed to parse JSON: %v\nline: %s", err, line)
		}
		// slogtest expects top-level keys; flatten "fields" into top-level
		if fields, ok := m["fields"].(map[string]any); ok {
			maps.Copy(m, fields)
			delete(m, "fields")
		}
		// slogtest expects "msg" key, but we use "message"
		if msg, ok := m["message"]; ok {
			m["msg"] = msg
			delete(m, "message")
		}
		// slogtest expects "level" as string
		// slogtest expects "time" key, but we use "timestamp"
		if ts, ok := m["timestamp"]; ok {
			if ts == "" {
				// Zero time: don't include "time" key so slogtest sees it as absent
				delete(m, "timestamp")
			} else {
				m["time"] = ts
				delete(m, "timestamp")
			}
		}
		return m
	}
	slogtest.Run(t, newHandler, result)
}

func TestSlogHandler_LevelVar(t *testing.T) {
	var lv slog.LevelVar
	lv.Set(slog.LevelInfo)

	h := NewSlogHandler(io.Discard, "plain", &SlogHandlerOptions{Level: &lv})

	if h.Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("Debug should be disabled when level is Info")
	}
	if !h.Enabled(context.TODO(), slog.LevelInfo) {
		t.Error("Info should be enabled when level is Info")
	}

	lv.Set(slog.LevelDebug)
	if !h.Enabled(context.TODO(), slog.LevelDebug) {
		t.Error("Debug should be enabled after changing LevelVar to Debug")
	}
}

func TestSlogHandler_LevelVarFiltering(t *testing.T) {
	var buf bytes.Buffer
	var lv slog.LevelVar
	lv.Set(slog.LevelWarn)

	h := NewSlogHandler(&buf, "plain", &SlogHandlerOptions{Level: &lv})
	logger := slog.New(h)
	logger.Info("should not appear")

	if buf.Len() > 0 {
		t.Errorf("Info message should not be written when level is Warn, got: %s", buf.String())
	}

	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("Warn message should be written when level is Warn")
	}
}

func TestSlogHandler_TeeMode(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	w := io.MultiWriter(&buf1, &buf2)
	h := NewSlogHandler(w, "plain", nil)
	logger := slog.New(h)
	logger.Info("tee test")

	if buf1.String() != buf2.String() {
		t.Errorf("tee mode: buf1 and buf2 differ:\nbuf1: %s\nbuf2: %s", buf1.String(), buf2.String())
	}
	if !strings.Contains(buf1.String(), "tee test") {
		t.Errorf("expected 'tee test' in output, got: %s", buf1.String())
	}
}

func TestSlogHandler_WithGroup(t *testing.T) {
	// Plain format: group prefix
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)
	logger := slog.New(h)
	logger.WithGroup("req").Info("msg", slog.String("method", "GET"))
	line := buf.String()
	if !strings.Contains(line, "req.method=GET") {
		t.Errorf("expected 'req.method=GET' in plain group output, got: %s", line)
	}

	// JSON format: nested object
	buf.Reset()
	h = NewSlogHandler(&buf, "json", nil)
	logger = slog.New(h)
	logger.WithGroup("req").Info("msg", slog.String("method", "GET"))
	line = buf.String()
	var result map[string]any
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	fields := result["fields"].(map[string]any)
	reqGroup, ok := fields["req"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested 'req' group in fields, got: %v", fields)
	}
	if reqGroup["method"] != "GET" {
		t.Errorf("expected req.method=GET, got: %v", reqGroup["method"])
	}
}

func TestSlogHandler_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)
	logger := slog.New(h)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Info("concurrent", slog.Int("n", n))
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}
}

func TestSlogHandler_EmptyGroup(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "plain", nil)

	// Empty group name should return same-behaving handler
	h2 := h.WithGroup("")
	if h2 == nil {
		t.Fatal("WithGroup(\"\") should not return nil")
	}

	logger := slog.New(h2)
	logger.Info("test")
	if !strings.Contains(buf.String(), "test") {
		t.Errorf("expected output after empty group, got: %s", buf.String())
	}
}

func TestSlogHandler_ErrorAttr(t *testing.T) {
	var buf bytes.Buffer
	h := NewSlogHandler(&buf, "json", nil)
	logger := slog.New(h)

	type testError struct{}
	logger.Info("oops", slog.Any("err", &testError{}))
	// Just verify it's valid JSON (error values should be handled gracefully)
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v, line: %s", err, buf.String())
	}
}

// --- Plan 02 Tests: ParseSlogLevel and CreateSlogLogger ---

func TestParseSlogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
		wantErr  bool
	}{
		{"trace", LevelTrace, false},
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"fatal", LevelFatal, false},
		{"panic", LevelPanic, false},
		// Case insensitivity
		{"INFO", slog.LevelInfo, false},
		{"Debug", slog.LevelDebug, false},
		{"WARN", slog.LevelWarn, false},
		{"WARNING", slog.LevelWarn, false},
		{"TRACE", LevelTrace, false},
		{"FATAL", LevelFatal, false},
		{"PANIC", LevelPanic, false},
		{"Error", slog.LevelError, false},
		// Invalid
		{"invalid", 0, true},
		{"", 0, true},
		{"verbose", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseSlogLevel(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseSlogLevel(%q) expected error, got level %d", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSlogLevel(%q) unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.expected {
				t.Errorf("ParseSlogLevel(%q) = %d, want %d", tc.input, got, tc.expected)
			}
		})
	}
}

func TestGetLoggingWriter_ConsoleOnly(t *testing.T) {
	var buf bytes.Buffer
	cfg := &Config{
		Level:  "info",
		Format: "plain",
	}
	cfg.SetDefaults()

	var levelVar slog.LevelVar
	levelVar.Set(slog.LevelInfo)

	writer, err := cfg.GetLoggingWriter(&buf)
	if err != nil {
		t.Fatalf("GetLoggingWriter failed: %v", err)
	}
	defer writer.Close()

	handler := NewSlogHandler(writer, cfg.Format, &SlogHandlerOptions{Level: &levelVar})
	logger := CreateSlogLogger(handler)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	logger.Info("hello console")
	got := buf.String()
	if !strings.Contains(got, "hello console") {
		t.Errorf("expected output to contain 'hello console', got: %s", got)
	}
	if !strings.Contains(got, "INFO") {
		t.Errorf("expected output to contain 'INFO', got: %s", got)
	}
}

func TestGetLoggingWriter_FileConfig(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Level:        "debug",
		Format:       "json",
		NoConsoleLog: true,
		File: &FileConfig{
			Path:       logPath,
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAgeDays: 7,
			Compress:   true,
		},
	}
	cfg.SetDefaults()

	var levelVar slog.LevelVar
	levelVar.Set(slog.LevelDebug)

	writer, err := cfg.GetLoggingWriter(nil)
	if err != nil {
		t.Fatalf("GetLoggingWriter failed: %v", err)
	}
	defer writer.Close()

	handler := NewSlogHandler(writer, cfg.Format, &SlogHandlerOptions{Level: &levelVar})
	logger := CreateSlogLogger(handler)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	logger.Info("file test message")

	// Read the file to verify it was written
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "file test message") {
		t.Errorf("expected log file to contain 'file test message', got: %s", string(data))
	}
}

func TestGetLoggingWriter_TeeMode(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tee.log")

	var consoleBuf bytes.Buffer
	cfg := &Config{
		Level:  "info",
		Format: "plain",
		File: &FileConfig{
			Path: logPath,
		},
	}
	cfg.SetDefaults()

	var levelVar slog.LevelVar
	levelVar.Set(slog.LevelInfo)

	writer, err := cfg.GetLoggingWriter(&consoleBuf)
	if err != nil {
		t.Fatalf("GetLoggingWriter failed: %v", err)
	}
	defer writer.Close()

	handler := NewSlogHandler(writer, cfg.Format, &SlogHandlerOptions{Level: &levelVar})
	logger := CreateSlogLogger(handler)

	logger.Info("tee message")

	// Check console output
	consoleOutput := consoleBuf.String()
	if !strings.Contains(consoleOutput, "tee message") {
		t.Errorf("expected console to contain 'tee message', got: %s", consoleOutput)
	}

	// Check file output
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "tee message") {
		t.Errorf("expected log file to contain 'tee message', got: %s", string(data))
	}
}

func TestCreateSlogLogger_LevelVarReload(t *testing.T) {
	var buf bytes.Buffer
	cfg := &Config{
		Level:  "info",
		Format: "plain",
	}
	cfg.SetDefaults()

	var levelVar slog.LevelVar
	levelVar.Set(slog.LevelInfo)

	writer, err := cfg.GetLoggingWriter(&buf)
	if err != nil {
		t.Fatalf("GetLoggingWriter failed: %v", err)
	}
	defer writer.Close()

	handler := NewSlogHandler(writer, cfg.Format, &SlogHandlerOptions{Level: &levelVar})
	logger := CreateSlogLogger(handler)

	// Debug should be filtered at Info level
	logger.Debug("should not appear")
	if strings.Contains(buf.String(), "should not appear") {
		t.Error("Debug message should be filtered at Info level")
	}

	// Change level to Debug dynamically
	levelVar.Set(slog.LevelDebug)

	// Now Debug should be written
	logger.Debug("should appear now")
	if !strings.Contains(buf.String(), "should appear now") {
		t.Errorf("Debug message should appear after level change, got: %s", buf.String())
	}
}

func TestGetLoggingWriter_NoWriterError(t *testing.T) {
	cfg := &Config{
		Level:        "info",
		Format:       "plain",
		NoConsoleLog: true,
	}
	cfg.SetDefaults()

	_, err := cfg.GetLoggingWriter(nil)
	if err == nil {
		t.Error("expected error when no console writer and no file config")
	}
	if !strings.Contains(err.Error(), "consoleWriter is required") {
		t.Errorf("expected error about consoleWriter, got: %v", err)
	}
}
