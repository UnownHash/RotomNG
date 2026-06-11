package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/logging"
)

func init() { //nolint:gochecknoinits // gin test mode must be set before tests run
	gin.SetMode(gin.TestMode)
}

// --- NoOpStatsCollector ---

func TestNoOpStatsCollector_AllMethods(_ *testing.T) {
	sc := NewNoOpStatsCollector()
	// Verify all methods execute without panic
	sc.IncrWorkerAccepts()
	sc.IncrWorkerAcceptFails()
	sc.IncrWorkerRegistrationFails()
	sc.IncrWorkersConnected("origin1")
	sc.DecrWorkersConnected("origin1")
}

func TestNoOpStatsCollector_ImplementsInterface(_ *testing.T) {
	var _ WorkerStatsCollector = NewNoOpStatsCollector()
}

// --- HTTPAPIHandlerSettings ---

func TestHTTPAPIHandlerSettings_Validate(t *testing.T) {
	s := HTTPAPIHandlerSettings{}
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// --- HTTPAPIHandlerConfig.Init ---

func TestHTTPAPIHandlerConfig_Init(t *testing.T) {
	cfg := &HTTPAPIHandlerConfig{}
	err := cfg.Init(HTTPAPIHandlerSettings{
		CurrentConfig: config.Config{Instance: "test"},
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	s := cfg.GetSettings()
	if s.CurrentConfig.Instance != "test" {
		t.Errorf("expected instance 'test', got '%s'", s.CurrentConfig.Instance)
	}
}

// --- NewHTTPAPIHandler ---

func newTestHTTPAPIHandler(reloadFn func() error) *HTTPAPIHandler {
	cfg := &HTTPAPIHandlerConfig{
		Logger:     logging.NewDiscardLogger(),
		AppVersion: "1.0.0",
		GitSHA:     "abc123",
		ReloadFn:   reloadFn,
	}
	cfg.Init(HTTPAPIHandlerSettings{
		CurrentConfig: config.Config{
			Instance: "test-instance",
			Tuning:   &config.Tuning{Profiling: true},
			RateLimit: &config.RateLimitConfig{
				Enable:                   true,
				MaxSelectionsPerDuration: 10,
				Duration:                 5 * time.Minute,
			},
			Prometheus: &config.PrometheusConfig{Enable: true},
		},
	})
	return NewHTTPAPIHandler(*cfg)
}

func TestNewHTTPAPIHandler(t *testing.T) {
	handler := newTestHTTPAPIHandler(nil)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.appVersion != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", handler.appVersion)
	}
	if handler.gitSHA != "abc123" {
		t.Errorf("expected sha 'abc123', got '%s'", handler.gitSHA)
	}
}

// --- GetConfig ---

func TestGetConfig(t *testing.T) {
	handler := newTestHTTPAPIHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/config", nil)

	handler.GetConfig(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%v'", resp["status"])
	}

	cfg, ok := resp["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config object in response")
	}
	if cfg["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%v'", cfg["version"])
	}
	if cfg["instance"] != "test-instance" {
		t.Errorf("expected instance 'test-instance', got '%v'", cfg["instance"])
	}
	if cfg["rate_limit"] == nil {
		t.Error("expected rate_limit in response")
	}
}

func TestGetConfig_NoInstance(t *testing.T) {
	cfg := &HTTPAPIHandlerConfig{
		Logger:     logging.NewDiscardLogger(),
		AppVersion: "1.0.0",
		GitSHA:     "abc123",
	}
	cfg.Init(HTTPAPIHandlerSettings{
		CurrentConfig: config.Config{
			Tuning:     &config.Tuning{},
			Prometheus: &config.PrometheusConfig{},
		},
	})
	handler := NewHTTPAPIHandler(*cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/config", nil)

	handler.GetConfig(c)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	cfgMap := resp["config"].(map[string]any)
	if _, exists := cfgMap["instance"]; exists {
		t.Error("expected no instance field when empty")
	}
	if _, exists := cfgMap["rate_limit"]; exists {
		t.Error("expected no rate_limit field when nil")
	}
}

// --- GetPrometheusEnabled ---

func TestGetPrometheusEnabled_True(t *testing.T) {
	handler := newTestHTTPAPIHandler(nil)
	if !handler.GetPrometheusEnabled() {
		t.Error("expected prometheus enabled to be true")
	}
}

func TestGetPrometheusEnabled_False(t *testing.T) {
	cfg := &HTTPAPIHandlerConfig{
		Logger: logging.NewDiscardLogger(),
	}
	cfg.Init(HTTPAPIHandlerSettings{
		CurrentConfig: config.Config{
			Prometheus: &config.PrometheusConfig{Enable: false},
		},
	})
	handler := NewHTTPAPIHandler(*cfg)

	if handler.GetPrometheusEnabled() {
		t.Error("expected prometheus enabled to be false")
	}
}

// --- ConfigReload ---

func TestConfigReload_Success(t *testing.T) {
	handler := newTestHTTPAPIHandler(func() error { return nil })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/config/reload", nil)

	handler.ConfigReload(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestConfigReload_Error(t *testing.T) {
	handler := newTestHTTPAPIHandler(func() error { return errors.New("reload failed") })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/config/reload", nil)

	handler.ConfigReload(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if !contains(string(body), "reload failed") {
		t.Errorf("expected error message in response, got: %s", string(body))
	}
}

// --- NewWorkerHandler ---

func TestNewWorkerHandler(t *testing.T) {
	handler := NewWorkerHandler(context.TODO(), WorkerHandlerConfig{
		Logger: logging.NewDiscardLogger(),
	})
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	// Should use NoOpStatsCollector when nil
	if handler.statsCollector == nil {
		t.Error("expected non-nil statsCollector")
	}
}

func TestNewWorkerHandler_WithStatsCollector(t *testing.T) {
	sc := NewNoOpStatsCollector()
	handler := NewWorkerHandler(context.TODO(), WorkerHandlerConfig{
		Logger:         logging.NewDiscardLogger(),
		StatsCollector: sc,
	})
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
