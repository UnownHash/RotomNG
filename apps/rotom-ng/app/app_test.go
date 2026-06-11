package app

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/logging"
)

// newTestConfig returns a *config.Config with defaults set, suitable for tests.
// Listener addresses use :0 to avoid port conflicts.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		DeviceListener: &config.DeviceListener{
			Address: "127.0.0.1:0",
		},
		ControllerListener: &config.ControllerListener{
			Address: "127.0.0.1:0",
		},
		HTTPListener: &config.HTTPListener{
			Address: "127.0.0.1:0",
		},
		Logging: &logging.Config{
			Level:  "info",
			Format: "plain",
			File: &logging.FileConfig{
				Disable: true,
			},
		},
		Jobs: &config.JobsConfig{
			Enable: true,
			Path:   tmpDir,
		},
	}
	cfg.SetDefaults()
	return cfg
}

func newTestFlagConfig() FlagConfig {
	return FlagConfig{
		UIDev: true, // skip UI file checks
		ReloadConfig: func() (*config.Config, error) {
			return nil, nil
		},
	}
}

// newTestApp creates a new App and registers cleanup to close the logger.
func newTestApp(t *testing.T, cfg *config.Config, flagCfg FlagConfig) (*App, error) {
	t.Helper()
	app, err := NewApp(cfg, flagCfg)
	if err == nil && app != nil {
		t.Cleanup(func() {
			if app.closer != nil {
				app.closer.Close()
			}
		})
	}
	return app, err
}

// TestNewApp_DefaultConfig tests NewApp with a default valid config.
func TestNewApp_DefaultConfig(t *testing.T) {
	cfg := newTestConfig(t)

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil App")
	}
	if app.cfg != cfg {
		t.Error("expected app.cfg to match provided config")
	}
	if app.logger == nil {
		t.Error("expected app.logger to be set")
	}
}

// TestNewApp_NilLoggingConfig tests NewApp when Logging is nil.
func TestNewApp_NilLoggingConfig(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Logging = nil

	_, err := newTestApp(t, cfg, newTestFlagConfig())
	if err == nil {
		t.Fatal("expected error when Logging config is nil")
	}
}

// TestNewApp_InvalidLogLevel tests NewApp with an invalid log level.
func TestNewApp_InvalidLogLevel(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Logging.Level = "invalid-level"

	_, err := newTestApp(t, cfg, newTestFlagConfig())
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

// TestNewApp_WithSecrets tests NewApp with listener secrets set.
func TestNewApp_WithSecrets(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.DeviceListener.Secret = "device-secret"
	cfg.ControllerListener.Secret = "ctrl-secret"
	cfg.HTTPListener.Secret = "http-secret"

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil App")
	}
}

// TestNewApp_FlagConfigPreserved tests that FlagConfig is stored correctly.
func TestNewApp_FlagConfigPreserved(t *testing.T) {
	cfg := newTestConfig(t)
	flagCfg := FlagConfig{
		DebugMode: true,
		UIPath:    "/some/path",
		UIDev:     true,
	}

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	if app.flagCfg.DebugMode != true {
		t.Error("expected DebugMode to be true")
	}
	if app.flagCfg.UIPath != "/some/path" {
		t.Errorf("expected UIPath '/some/path', got '%s'", app.flagCfg.UIPath)
	}
	if app.flagCfg.UIDev != true {
		t.Error("expected UIDev to be true")
	}
}

// TestInit_DebugMode tests that Init sets debug level and gin mode.
func TestInit_DebugMode(t *testing.T) {
	cfg := newTestConfig(t)
	flagCfg := newTestFlagConfig()
	flagCfg.DebugMode = true

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	if app.levelVar.Level() != slog.LevelDebug {
		t.Error("expected debug level to be enabled in debug mode")
	}

	if app.ctx == nil {
		t.Error("expected ctx to be set after Init")
	}
	if app.cancel == nil {
		t.Error("expected cancel to be set after Init")
	}
}

// TestInit_ReleaseMode tests Init without debug mode.
func TestInit_ReleaseMode(t *testing.T) {
	cfg := newTestConfig(t)
	flagCfg := newTestFlagConfig()
	flagCfg.DebugMode = false

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// In release mode, debug should not be enabled (config level is "info")
	if app.levelVar.Level() == slog.LevelDebug {
		t.Error("expected logger not to be at debug level in release mode")
	}

	if app.connectionManager == nil {
		t.Error("expected connectionManager to be initialized")
	}
	if app.jobsManager == nil {
		t.Error("expected jobsManager to be initialized")
	}
}

// TestInit_UIPathValidation tests Init UI path validation logic.
func TestInit_UIPathValidation(t *testing.T) {
	tests := []struct {
		name    string
		uiDev   bool
		uiPath  string
		uifs    *embed.FS
		wantErr bool
		errMsg  string
	}{
		{
			name:    "UIDev skips validation",
			uiDev:   true,
			uiPath:  "",
			uifs:    nil,
			wantErr: false,
		},
		{
			name:    "embedded UI with no path skips validation",
			uiDev:   false,
			uiPath:  "",
			uifs:    &embed.FS{},
			wantErr: false,
		},
		{
			name:    "no embedded UI and missing path fails",
			uiDev:   false,
			uiPath:  "/nonexistent/path",
			uifs:    nil,
			wantErr: true,
			errMsg:  "UI index.html file does not exist",
		},
		{
			name:    "no embedded UI and no path fails",
			uiDev:   false,
			uiPath:  "",
			uifs:    nil,
			wantErr: true,
			errMsg:  "UI index.html file does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			flagCfg := newTestFlagConfig()
			flagCfg.UIDev = tt.uiDev
			flagCfg.UIPath = tt.uiPath
			flagCfg.UIFS = tt.uifs

			app, err := newTestApp(t, cfg, flagCfg)
			if err != nil {
				t.Fatalf("NewApp returned error: %v", err)
			}

			err = app.Init()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errMsg != "" {
					if got := err.Error(); !strings.Contains(got, tt.errMsg) {
						t.Errorf("expected error to contain %q, got %q", tt.errMsg, got)
					}
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestInit_UIPathWithIndexHTML tests Init succeeds when index.html exists at UIPath.
func TestInit_UIPathWithIndexHTML(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	cfg := newTestConfig(t)
	flagCfg := newTestFlagConfig()
	flagCfg.UIDev = false
	flagCfg.UIPath = tmpDir
	flagCfg.UIFS = nil

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
}

// TestInit_WithRateLimiting tests Init with rate limiting enabled and verifies
// the selector config settings are stored correctly.
func TestInit_WithRateLimiting(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.RateLimit = &config.RateLimitConfig{
		Enable:                   true,
		MaxSelectionsPerDuration: 10,
		Duration:                 time.Minute,
	}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	selectorSettings := app.selectorConfig.GetSettings()
	if !selectorSettings.DeviceRateLimit.Enabled {
		t.Error("expected DeviceRateLimit.Enabled to be true")
	}
	if selectorSettings.DeviceRateLimit.MaxSelections != 10 {
		t.Errorf("expected MaxSelections=10, got %d", selectorSettings.DeviceRateLimit.MaxSelections)
	}
	if selectorSettings.DeviceRateLimit.Duration != time.Minute {
		t.Errorf("expected Duration=1m, got %v", selectorSettings.DeviceRateLimit.Duration)
	}
}

// TestInit_WithRateLimitingDisabled tests Init with rate limiting explicitly disabled
// and verifies the selector config reflects that.
func TestInit_WithRateLimitingDisabled(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.RateLimit = &config.RateLimitConfig{
		Enable: false,
	}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	selectorSettings := app.selectorConfig.GetSettings()
	if selectorSettings.DeviceRateLimit.Enabled {
		t.Error("expected DeviceRateLimit.Enabled to be false")
	}
}

// TestInit_WithPrometheus tests Init with Prometheus enabled and verifies
// the connection manager config receives the DisableWorkerStats flag.
func TestInit_WithPrometheus(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Prometheus = &config.PrometheusConfig{
		Enable:    true,
		Namespace: "test_ns",
	}
	cfg.Tuning.DisableWorkerStats = true
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	if app.statsCollector == nil {
		t.Fatal("expected statsCollector to be initialized")
	}

	connSettings := app.connectionManagerConfig.GetSettings()
	if !connSettings.DisableWorkerStats {
		t.Error("expected DisableWorkerStats=true when tuning has it enabled")
	}
}

// TestInit_WithPrometheusDisabled tests Init with Prometheus disabled and verifies
// DisableWorkerStats defaults to false in the connection manager config.
func TestInit_WithPrometheusDisabled(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Prometheus = &config.PrometheusConfig{
		Enable: false,
	}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	connSettings := app.connectionManagerConfig.GetSettings()
	if connSettings.DisableWorkerStats {
		t.Error("expected DisableWorkerStats=false by default")
	}
}

// TestInit_WithProfiling tests Init with profiling enabled and verifies
// the API handler config has profiling set.
func TestInit_WithProfiling(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Tuning = &config.Tuning{
		Profiling: true,
	}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	apiSettings := app.apiHandlerConfig.GetSettings()
	if !apiSettings.ProfilingEnabled {
		t.Error("expected ProfilingEnabled=true")
	}
}

// TestInit_WithProfilingDisabled tests Init with profiling disabled and verifies
// the API handler config reflects that.
func TestInit_WithProfilingDisabled(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Tuning = &config.Tuning{
		Profiling: false,
	}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	apiSettings := app.apiHandlerConfig.GetSettings()
	if apiSettings.ProfilingEnabled {
		t.Error("expected ProfilingEnabled=false")
	}
}

// TestInit_WithListenerSecrets tests Init with various secret configurations
// and verifies auth middleware is always created.
func TestInit_WithListenerSecrets(t *testing.T) {
	tests := []struct {
		name             string
		deviceSecret     string
		controllerSecret string
		httpSecret       string
	}{
		{
			name:             "no secrets",
			deviceSecret:     "",
			controllerSecret: "",
			httpSecret:       "",
		},
		{
			name:             "all secrets set",
			deviceSecret:     "dev-secret",
			controllerSecret: "ctrl-secret",
			httpSecret:       "http-secret",
		},
		{
			name:             "only device secret",
			deviceSecret:     "dev-secret",
			controllerSecret: "",
			httpSecret:       "",
		},
		{
			name:             "only controller secret",
			deviceSecret:     "",
			controllerSecret: "ctrl-secret",
			httpSecret:       "",
		},
		{
			name:             "only http secret",
			deviceSecret:     "",
			controllerSecret: "",
			httpSecret:       "http-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.DeviceListener.Secret = tt.deviceSecret
			cfg.ControllerListener.Secret = tt.controllerSecret
			cfg.HTTPListener.Secret = tt.httpSecret

			app, err := newTestApp(t, cfg, newTestFlagConfig())
			if err != nil {
				t.Fatalf("NewApp returned error: %v", err)
			}

			if err := app.Init(); err != nil {
				t.Fatalf("Init returned error: %v", err)
			}

			if app.deviceAuthMiddleware == nil {
				t.Error("expected deviceAuthMiddleware to be set")
			}
			if app.controllerAuthMiddleware == nil {
				t.Error("expected controllerAuthMiddleware to be set")
			}
			if app.httpAuthMiddleware == nil {
				t.Error("expected httpAuthMiddleware to be set")
			}
		})
	}
}

// TestInit_AllFieldsPopulated verifies every field on App is non-nil/non-zero
// after a successful Init with a minimal config.
func TestInit_AllFieldsPopulated(t *testing.T) {
	cfg := newTestConfig(t)
	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	if app.ctx == nil {
		t.Error("expected ctx to be set")
	}
	if app.cancel == nil {
		t.Error("expected cancel to be set")
	}
	if app.bufferPool == nil {
		t.Error("expected bufferPool to be set")
	}
	if app.statsCollector == nil {
		t.Error("expected statsCollector to be set")
	}
	if app.connectionManager == nil {
		t.Error("expected connectionManager to be set")
	}
	if app.jobsManager == nil {
		t.Error("expected jobsManager to be set")
	}
	if app.deviceAuthMiddleware == nil {
		t.Error("expected deviceAuthMiddleware to be set")
	}
	if app.controllerAuthMiddleware == nil {
		t.Error("expected controllerAuthMiddleware to be set")
	}
	if app.httpAuthMiddleware == nil {
		t.Error("expected httpAuthMiddleware to be set")
	}
	if app.deviceServer == nil {
		t.Error("expected deviceServer to be set")
	}
	if app.controllerServer == nil {
		t.Error("expected controllerServer to be set")
	}
	if app.httpServer == nil {
		t.Error("expected httpServer to be set")
	}
}

// TestInit_ConnectionManagerConfig verifies the connection manager config fields
// are wired up correctly after Init.
func TestInit_ConnectionManagerConfig(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Prometheus = &config.PrometheusConfig{
		Enable: true,
	}
	cfg.Tuning.DisableWorkerStats = true
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	cmCfg := app.connectionManagerConfig
	if cmCfg.Logger == nil {
		t.Error("expected connectionManagerConfig.Logger to be set")
	}
	if cmCfg.WorkerSelector == nil {
		t.Error("expected connectionManagerConfig.WorkerSelector to be set")
	}
	if cmCfg.JobsRunner == nil {
		t.Error("expected connectionManagerConfig.JobsRunner to be set")
	}
	if cmCfg.StatsCollector == nil {
		t.Error("expected connectionManagerConfig.StatsCollector to be set")
	}
	if cmCfg.NewController == nil {
		t.Error("expected connectionManagerConfig.NewController to be set")
	}
	if cmCfg.UserAgent != userAgent {
		t.Errorf("expected UserAgent=%q, got %q", userAgent, cmCfg.UserAgent)
	}
}

// TestInit_JobsManagerConfig verifies the jobs manager config is wired up correctly.
func TestInit_JobsManagerConfig(t *testing.T) {
	jobsDir := t.TempDir()
	cfg := newTestConfig(t)
	cfg.Jobs = &config.JobsConfig{
		Path: jobsDir,
	}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	jmCfg := app.jobsManagerConfig
	if jmCfg.Logger == nil {
		t.Error("expected jobsManagerConfig.Logger to be set")
	}

	jmSettings := jmCfg.GetSettings()
	if jmSettings.JobsPath != jobsDir {
		t.Errorf("expected JobsPath=%q, got %q", jobsDir, jmSettings.JobsPath)
	}
}

// TestInit_HTTPAPIHandlerConfig verifies the HTTP API handler config fields.
func TestInit_HTTPAPIHandlerConfig(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Instance = "verify-instance"
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	httpCfg := app.httpAPIHandlerConfig
	if httpCfg.Logger == nil {
		t.Error("expected httpAPIHandlerConfig.Logger to be set")
	}
	if httpCfg.APIHandler == nil {
		t.Error("expected httpAPIHandlerConfig.APIHandler to be set")
	}
	if httpCfg.AppVersion != appVersion {
		t.Errorf("expected AppVersion=%q, got %q", appVersion, httpCfg.AppVersion)
	}
	if httpCfg.GitSHA == "" {
		t.Error("expected httpAPIHandlerConfig.GitSHA to be set")
	}
	if httpCfg.ReloadFn == nil {
		t.Error("expected httpAPIHandlerConfig.ReloadFn to be set")
	}

	httpSettings := httpCfg.GetSettings()
	if httpSettings.CurrentConfig.Instance != "verify-instance" {
		t.Errorf("expected CurrentConfig.Instance=%q, got %q", "verify-instance", httpSettings.CurrentConfig.Instance)
	}
}

// TestInit_APIHandlerConfig verifies the base API handler config fields.
func TestInit_APIHandlerConfig(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Tuning = &config.Tuning{Profiling: true}
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	apiCfg := app.apiHandlerConfig
	if apiCfg.Logger == nil {
		t.Error("expected apiHandlerConfig.Logger to be set")
	}
	if apiCfg.ConnectionManager == nil {
		t.Error("expected apiHandlerConfig.ConnectionManager to be set")
	}
	if apiCfg.JobsManager == nil {
		t.Error("expected apiHandlerConfig.JobsManager to be set")
	}
	if apiCfg.APIConverter == nil {
		t.Error("expected apiHandlerConfig.APIConverter to be set")
	}

	apiSettings := apiCfg.GetSettings()
	if !apiSettings.ProfilingEnabled {
		t.Error("expected ProfilingEnabled=true in apiHandlerConfig settings")
	}
}

// TestInit_ControllerHandlerConfig verifies the controller handler config fields.
func TestInit_ControllerHandlerConfig(t *testing.T) {
	cfg := newTestConfig(t)
	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	chCfg := app.controllerHandlerConfig
	if chCfg.Logger == nil {
		t.Error("expected controllerHandlerConfig.Logger to be set")
	}
	if chCfg.ConnectionManager == nil {
		t.Error("expected controllerHandlerConfig.ConnectionManager to be set")
	}
	if chCfg.BufferPool == nil {
		t.Error("expected controllerHandlerConfig.BufferPool to be set")
	}
	if chCfg.StatsCollector == nil {
		t.Error("expected controllerHandlerConfig.StatsCollector to be set")
	}
}

// TestInit_FullConfig tests Init with every config option populated and verifies
// all stored config structs.
func TestInit_FullConfig(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Instance = "test-instance"
	cfg.DeviceListener.Secret = "dev-secret"
	cfg.ControllerListener.Secret = "ctrl-secret"
	cfg.HTTPListener.Secret = "http-secret"
	cfg.ShutdownTimeout = 10 * time.Second
	cfg.RateLimit = &config.RateLimitConfig{
		Enable:                   true,
		MaxSelectionsPerDuration: 5,
		Duration:                 30 * time.Second,
	}
	cfg.Prometheus = &config.PrometheusConfig{
		Enable:    true,
		Namespace: "full_test",
	}
	cfg.Tuning = &config.Tuning{
		Profiling:          true,
		DisableWorkerStats: true,
	}
	cfg.SetDefaults()

	flagCfg := FlagConfig{
		DebugMode: true,
		UIDev:     true,
		ReloadConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// Verify all top-level objects are created
	if app.deviceServer == nil {
		t.Error("expected deviceServer")
	}
	if app.controllerServer == nil {
		t.Error("expected controllerServer")
	}
	if app.httpServer == nil {
		t.Error("expected httpServer")
	}
	if app.connectionManager == nil {
		t.Error("expected connectionManager")
	}
	if app.jobsManager == nil {
		t.Error("expected jobsManager")
	}
	if app.bufferPool == nil {
		t.Error("expected bufferPool")
	}
	if app.statsCollector == nil {
		t.Error("expected statsCollector")
	}

	// Verify selector settings
	selSettings := app.selectorConfig.GetSettings()
	if !selSettings.DeviceRateLimit.Enabled {
		t.Error("expected rate limit enabled in full config")
	}
	if selSettings.DeviceRateLimit.MaxSelections != 5 {
		t.Errorf("expected MaxSelections=5, got %d", selSettings.DeviceRateLimit.MaxSelections)
	}
	if selSettings.DeviceRateLimit.Duration != 30*time.Second {
		t.Errorf("expected Duration=30s, got %v", selSettings.DeviceRateLimit.Duration)
	}

	// Verify connection manager settings
	connSettings := app.connectionManagerConfig.GetSettings()
	if !connSettings.DisableWorkerStats {
		t.Error("expected DisableWorkerStats=true in full config")
	}

	// Verify API handler settings
	apiSettings := app.apiHandlerConfig.GetSettings()
	if !apiSettings.ProfilingEnabled {
		t.Error("expected ProfilingEnabled=true in full config")
	}

	// Verify HTTP API handler settings
	httpSettings := app.httpAPIHandlerConfig.GetSettings()
	if httpSettings.CurrentConfig.Instance != "test-instance" {
		t.Errorf("expected Instance=%q, got %q", "test-instance", httpSettings.CurrentConfig.Instance)
	}

	// Verify jobs path
	jmSettings := app.jobsManagerConfig.GetSettings()
	if jmSettings.JobsPath == "" {
		t.Error("expected JobsPath to be set")
	}

	// Verify debug mode was applied
	if app.levelVar.Level() != slog.LevelDebug {
		t.Error("expected debug level enabled in full config with debug mode")
	}
}

// TestInit_DisableWorkerStatsDefault tests that DisableWorkerStats defaults to false.
func TestInit_DisableWorkerStatsDefault(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	connSettings := app.connectionManagerConfig.GetSettings()
	if connSettings.DisableWorkerStats {
		t.Error("expected DisableWorkerStats=false by default")
	}
}

// TestInit_CustomShutdownTimeout tests Init with a custom shutdown timeout.
func TestInit_CustomShutdownTimeout(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ShutdownTimeout = 30 * time.Second
	cfg.SetDefaults()

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	if err := app.Init(); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	if app.cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("expected shutdown timeout 30s, got %v", app.cfg.ShutdownTimeout)
	}
}

// TestInit_EmptyJobsPath tests Init fails when jobs path is empty.
func TestInit_EmptyJobsPath(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Jobs = &config.JobsConfig{
		Path: "",
	}
	// Don't call SetDefaults — it would fill in the default path.
	// Instead, manually clear it after SetDefaults since other fields need defaults.
	cfg.SetDefaults()
	cfg.Jobs.Path = ""

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	err = app.Init()
	if err == nil {
		t.Fatal("expected error for empty jobs path")
	}
	if !strings.Contains(err.Error(), "jobs") {
		t.Errorf("expected error about jobs, got: %v", err)
	}
}

// TestInit_UIPathNotReadable tests Init fails when UI path exists but index.html
// is not accessible (directory with no index.html).
func TestInit_UIPathNotReadable(t *testing.T) {
	tmpDir := t.TempDir()
	// tmpDir exists but has no index.html

	cfg := newTestConfig(t)
	flagCfg := newTestFlagConfig()
	flagCfg.UIDev = false
	flagCfg.UIPath = tmpDir
	flagCfg.UIFS = nil

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	err = app.Init()
	if err == nil {
		t.Fatal("expected error when index.html does not exist in UI path")
	}
	if !strings.Contains(err.Error(), "UI index.html") {
		t.Errorf("expected error about UI index.html, got: %v", err)
	}
}

// TestInit_EmbeddedUIOverriddenByPath tests Init validates UIPath even when
// embedded FS is provided, if UIPath is explicitly set.
func TestInit_EmbeddedUIOverriddenByPath(t *testing.T) {
	cfg := newTestConfig(t)
	flagCfg := newTestFlagConfig()
	flagCfg.UIDev = false
	flagCfg.UIPath = "/nonexistent/override/path"
	flagCfg.UIFS = &embed.FS{} // embedded provided but path overrides

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	err = app.Init()
	if err == nil {
		t.Fatal("expected error when UIPath overrides embedded FS with nonexistent path")
	}
	if !strings.Contains(err.Error(), "UI index.html") {
		t.Errorf("expected error about UI index.html, got: %v", err)
	}
}

// TestApp_Logger tests the Logger() accessor method.
func TestApp_Logger(t *testing.T) {
	cfg := newTestConfig(t)
	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}

	logger := app.Logger()
	if logger == nil {
		t.Fatal("expected Logger() to return non-nil")
	}
}

// TestGetSelectorSettings tests the getSelectorSettings helper.
func TestGetSelectorSettings(t *testing.T) {
	tests := []struct {
		name                string
		rateLimit           *config.RateLimitConfig
		expectEnabled       bool
		expectMaxSelections int
		expectDuration      time.Duration
	}{
		{
			name: "rate limiting disabled",
			rateLimit: &config.RateLimitConfig{
				Enable: false,
			},
			expectEnabled: false,
		},
		{
			name: "rate limiting enabled",
			rateLimit: &config.RateLimitConfig{
				Enable:                   true,
				MaxSelectionsPerDuration: 20,
				Duration:                 2 * time.Minute,
			},
			expectEnabled:       true,
			expectMaxSelections: 20,
			expectDuration:      2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.RateLimit = tt.rateLimit
			cfg.SetDefaults()

			settings := getSelectorSettings(cfg)

			if settings.DeviceRateLimit.Enabled != tt.expectEnabled {
				t.Errorf("expected Enabled=%v, got %v", tt.expectEnabled, settings.DeviceRateLimit.Enabled)
			}
			if tt.expectEnabled {
				if settings.DeviceRateLimit.MaxSelections != tt.expectMaxSelections {
					t.Errorf("expected MaxSelections=%d, got %d", tt.expectMaxSelections, settings.DeviceRateLimit.MaxSelections)
				}
				if settings.DeviceRateLimit.Duration != tt.expectDuration {
					t.Errorf("expected Duration=%v, got %v", tt.expectDuration, settings.DeviceRateLimit.Duration)
				}
			}
		})
	}
}

// TestGetConnectionManagerSettings tests the getConnectionManagerSettings helper.
func TestGetConnectionManagerSettings(t *testing.T) {
	tests := []struct {
		name               string
		disableWorkerStats bool
	}{
		{
			name:               "default (stats enabled)",
			disableWorkerStats: false,
		},
		{
			name:               "stats disabled",
			disableWorkerStats: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.Tuning.DisableWorkerStats = tt.disableWorkerStats
			cfg.SetDefaults()

			settings := getConnectionManagerSettings(cfg)
			if settings.DisableWorkerStats != tt.disableWorkerStats {
				t.Errorf("expected DisableWorkerStats=%v, got %v", tt.disableWorkerStats, settings.DisableWorkerStats)
			}
		})
	}
}

// TestGetJobsManagerSettings tests the getJobsManagerSettings helper.
func TestGetJobsManagerSettings(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Jobs = &config.JobsConfig{
		Path: "/custom/jobs/path",
	}
	cfg.SetDefaults()

	settings := getJobsManagerSettings(cfg)
	if settings.JobsPath != "/custom/jobs/path" {
		t.Errorf("expected JobsPath '/custom/jobs/path', got '%s'", settings.JobsPath)
	}
}

// TestGetHTTPAPIHandlerSettings tests the getHTTPAPIHandlerSettings helper.
func TestGetHTTPAPIHandlerSettings(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Instance = "my-instance"
	cfg.SetDefaults()

	settings := getHTTPAPIHandlerSettings(cfg)
	if settings.CurrentConfig.Instance != "my-instance" {
		t.Errorf("expected Instance 'my-instance', got '%s'", settings.CurrentConfig.Instance)
	}
}

// TestGetBaseAPIHandlerSettings tests the getBaseAPIHandlerSettings helper.
func TestGetBaseAPIHandlerSettings(t *testing.T) {
	tests := []struct {
		name        string
		profiling   bool
		jobsEnabled bool
	}{
		{"profiling enabled jobs enabled", true, true},
		{"profiling disabled jobs disabled", false, false},
		{"profiling enabled jobs disabled", true, false},
		{"profiling disabled jobs enabled", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			cfg.Tuning = &config.Tuning{
				Profiling: tt.profiling,
			}
			cfg.Jobs = &config.JobsConfig{
				Enable: tt.jobsEnabled,
			}
			cfg.SetDefaults()

			settings := getBaseAPIHandlerSettings(cfg)
			if settings.ProfilingEnabled != tt.profiling {
				t.Errorf("expected ProfilingEnabled=%v, got %v", tt.profiling, settings.ProfilingEnabled)
			}
			if settings.JobsEnabled != tt.jobsEnabled {
				t.Errorf("expected JobsEnabled=%v, got %v", tt.jobsEnabled, settings.JobsEnabled)
			}
		})
	}
}
