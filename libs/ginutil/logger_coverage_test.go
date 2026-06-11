package ginutil

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { //nolint:gochecknoinits // gin test mode must be set before tests run
	gin.SetMode(gin.TestMode)
}

func newTestEngine() (*gin.Engine, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewEngineWithLogger(logger), &buf
}

func TestLoggerMiddleware_400Status(t *testing.T) {
	r, buf := newTestEngine()
	r.GET("/notfound", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected log output for 4xx response")
	}
}

func TestLoggerMiddleware_500Status(t *testing.T) {
	r, buf := newTestEngine()
	r.GET("/error", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected log output for 5xx response")
	}
}

func TestLoggerMiddleware_WithQueryString(t *testing.T) {
	r, buf := newTestEngine()
	r.GET("/search", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"q": c.Query("q")})
	})

	req, _ := http.NewRequest(http.MethodGet, "/search?q=hello&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	output := buf.String()
	if !strings.Contains(output, "q=hello") {
		t.Errorf("expected query string in log output, got: %s", output)
	}
}

func TestLoggerMiddleware_WithGinError(t *testing.T) {
	r, buf := newTestEngine()
	r.GET("/err", func(c *gin.Context) {
		_ = c.Error(gin.Error{Err: http.ErrBodyNotAllowed, Type: gin.ErrorTypePrivate})
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/err", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	output := buf.String()
	if output == "" {
		t.Error("expected log output for request with errors")
	}

	// Verify error is included in the log output
	if !strings.Contains(output, "error") {
		t.Errorf("expected 'error' field in log output, got: %s", output)
	}

	// Verify no double-logging: exactly one "request completed" entry
	count := strings.Count(output, "request completed")
	if count != 1 {
		t.Errorf("expected exactly 1 'request completed' log entry, got %d", count)
	}
}

func TestRecoveryMiddleware_PanicWithError(t *testing.T) {
	r, buf := newTestEngine()
	r.GET("/panic-err", func(_ *gin.Context) {
		panic(errors.New("database connection lost"))
	})

	req, _ := http.NewRequest(http.MethodGet, "/panic-err", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	output := buf.String()
	if !strings.Contains(output, "database connection lost") {
		t.Errorf("expected error message in log, got: %s", output)
	}
}
