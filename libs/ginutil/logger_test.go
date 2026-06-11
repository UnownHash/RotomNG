package ginutil

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoggerMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test logger that writes to a buffer
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Create a Gin engine with our custom logger middleware
	r := NewEngineWithLogger(logger)

	// Add a test endpoint
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create a test request
	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a response recorder
	w := httptest.NewRecorder()

	// Perform the request
	r.ServeHTTP(w, req)

	// Check that the request was successful
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check that the response contains the expected message
	if !strings.Contains(w.Body.String(), "success") {
		t.Errorf("Expected response to contain 'success', got %s", w.Body.String())
	}

	// Check that the log contains request details
	logOutput := buf.String()
	if !strings.Contains(logOutput, "GET") {
		t.Errorf("Expected log output to contain HTTP method 'GET', got: %s", logOutput)
	}

	if !strings.Contains(logOutput, "/test") {
		t.Errorf("Expected log output to contain path '/test', got: %s", logOutput)
	}

	if !strings.Contains(logOutput, "200") {
		t.Errorf("Expected log output to contain status '200', got: %s", logOutput)
	}

	if !strings.Contains(logOutput, "latency") {
		t.Errorf("Expected log output to contain 'latency' field, got: %s", logOutput)
	}

	// Verify no double-logging: exactly one "request completed" entry
	count := strings.Count(logOutput, "request completed")
	if count != 1 {
		t.Errorf("Expected exactly 1 'request completed' log entry, got %d", count)
	}
}

func TestNewEngineWithLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test logger
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Create engine with logger
	r := NewEngineWithLogger(logger)

	// Verify that the engine is not nil
	if r == nil {
		t.Fatal("Expected engine to be created, got nil")
	}

	// Verify that we can add routes
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Test the route
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRecoveryMiddleware_Panic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	r := NewEngineWithLogger(logger)
	r.GET("/panic", func(_ *gin.Context) {
		panic("test panic")
	})

	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	output := buf.String()
	if !strings.Contains(output, "panic recovered in HTTP handler") {
		t.Errorf("expected panic log message, got: %s", output)
	}
	if !strings.Contains(output, "test panic") {
		t.Errorf("expected panic value in log output, got: %s", output)
	}
	if !strings.Contains(output, "stack") {
		t.Errorf("expected stack trace in log output, got: %s", output)
	}
}

func TestRecoveryMiddleware_500Status(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	r := NewEngineWithLogger(logger)
	r.GET("/panic", func(_ *gin.Context) {
		panic("test panic")
	})

	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	r := NewEngineWithLogger(logger)
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	output := buf.String()
	if strings.Contains(output, "panic") {
		t.Errorf("expected no panic log for normal request, got: %s", output)
	}
}
