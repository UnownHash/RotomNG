package testutil

import (
	"fmt"
	"os"
	"time"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/logging"
)

// Option is a functional option for configuring a test Config.
type Option func(*config.Config)

// WithDeviceSecret sets the device listener auth secret.
func WithDeviceSecret(secret string) Option {
	return func(cfg *config.Config) {
		cfg.DeviceListener.Secret = secret
	}
}

// WithControllerSecret sets the controller listener auth secret.
func WithControllerSecret(secret string) Option {
	return func(cfg *config.Config) {
		cfg.ControllerListener.Secret = secret
	}
}

// WithHTTPSecret sets the HTTP listener auth secret.
func WithHTTPSecret(secret string) Option {
	return func(cfg *config.Config) {
		cfg.HTTPListener.Secret = secret
	}
}

// WithShutdownTimeout sets the shutdown timeout duration.
func WithShutdownTimeout(d time.Duration) Option {
	return func(cfg *config.Config) {
		cfg.ShutdownTimeout = d
	}
}

// WithPrometheus enables or disables the Prometheus metrics endpoint.
func WithPrometheus(enabled bool) Option {
	return func(cfg *config.Config) {
		cfg.Prometheus.Enable = enabled
	}
}

// WithDisableWorkerStats sets the DisableWorkerStats Prometheus option.
// When enabled, worker request duration stats are not collected, providing a slight speedup.
func WithDisableWorkerStats(disabled bool) Option {
	return func(cfg *config.Config) {
		cfg.Tuning.DisableWorkerStats = disabled
	}
}

// WithDisableUI withholds the web UI, leaving the REST API as the only thing
// the http listener answers.
func WithDisableUI(disabled bool) Option {
	return func(cfg *config.Config) {
		cfg.HTTPListener.DisableUI = disabled
	}
}

// WithPrometheusNamespace sets the Prometheus metrics namespace.
func WithPrometheusNamespace(namespace string) Option {
	return func(cfg *config.Config) {
		cfg.Prometheus.Namespace = namespace
	}
}

// WithJobsEnabled sets the jobs enabled flag in the config.
func WithJobsEnabled(enabled bool) Option {
	return func(cfg *config.Config) {
		cfg.Jobs.Enable = enabled
	}
}

// WithInstance sets the instance name in the config.
func WithInstance(name string) Option {
	return func(cfg *config.Config) {
		cfg.Instance = name
	}
}

// WithRateLimit configures device selection rate limiting for the test environment.
// When enabled, each device can only be selected maxSelections times within the
// given duration window. This is applied AFTER SetDefaults(), correctly overriding
// the default disabled state.
func WithRateLimit(enabled bool, maxSelections int, duration time.Duration) Option {
	return func(cfg *config.Config) {
		cfg.RateLimit = &config.RateLimitConfig{
			Enable:                   enabled,
			MaxSelectionsPerDuration: maxSelections,
			Duration:                 duration,
		}
	}
}

// TestEnvOption is a functional option for configuring TestEnv behavior
// beyond config.Config (e.g., FlagConfig fields).
type TestEnvOption func(*TestEnv)

// WithReloadConfig sets the ReloadConfig callback used when SIGHUP triggers
// a config reload. The callback should return a valid *config.Config with
// SetDefaults() already called. If not set, TestEnv uses a default callback
// that returns (nil, nil).
func WithReloadConfig(fn func() (*config.Config, error)) TestEnvOption {
	return func(te *TestEnv) {
		te.reloadConfigFn = fn
	}
}

// NewTestConfig creates a valid *config.Config suitable for integration tests.
// It uses 127.0.0.1:0 addresses for all listeners (ephemeral ports).
// The returned cleanup function removes all temp directories created.
// This function does NOT import Go's testing package.
func NewTestConfig(opts ...Option) (*config.Config, func(), error) {
	tmpDir, err := os.MkdirTemp("", "rotom-test-*")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp dir: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	cfg := &config.Config{
		DeviceListener: &config.DeviceListener{
			Address: defaultTestAddr,
		},
		ControllerListener: &config.ControllerListener{
			Address: defaultTestAddr,
		},
		HTTPListener: &config.HTTPListener{
			Address: defaultTestAddr,
		},
		Logging: &logging.Config{
			Level:  "warn",
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

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg, cleanup, nil
}
