// Package config provides application configuration loading and validation for RotomNG.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/UnownHash/RotomNG/libs/logging"
)

// Default configuration values.
const (
	DefaultDeviceAddress     = ":7070"
	DefaultControllerAddress = ":7071"
	DefaultHTTPAddress       = ":7072"
	DefaultShutdownTimeout   = 5 * time.Second
	DefaultPromNamespace     = "rotom_ng"

	DefaultJobsPath    = "./jobs"
	DefaultLogFilePath = "./logs/rotom-ng.log"

	DefaultDevicePingInterval     = 30 * time.Second
	DefaultDevicePongWait         = 30 * time.Second
	DefaultControllerPingInterval = 30 * time.Second
	DefaultControllerPongWait     = 30 * time.Second

	DefaultControllerRegistrationTimeout = 60 * time.Second
)

// DeviceListener holds configuration for the device WebSocket listener.
type DeviceListener struct {
	Address      string        `koanf:"address"`
	Listener     net.Listener  `koanf:"-"`
	Secret       string        `koanf:"secret"`
	PingInterval time.Duration `koanf:"ping_interval"`
	PongWait     time.Duration `koanf:"pong_wait"`
}

// ControllerListener holds configuration for the controller WebSocket listener.
type ControllerListener struct {
	Address             string        `koanf:"address"`
	Listener            net.Listener  `koanf:"-"`
	Secret              string        `koanf:"secret"`
	PingInterval        time.Duration `koanf:"ping_interval"`
	PongWait            time.Duration `koanf:"pong_wait"`
	RegistrationTimeout time.Duration `koanf:"registration_timeout"`
}

// HTTPListener holds configuration for the HTTP API listener.
type HTTPListener struct {
	Address  string       `koanf:"address"`
	Listener net.Listener `koanf:"-"`
	Secret   string       `koanf:"secret"`
}

// Tuning holds performance tuning options.
type Tuning struct {
	Profiling          bool `koanf:"profiling"`
	DisableWorkerStats bool `koanf:"disable_worker_stats"`
}

// RateLimitConfig holds rate limiting settings for device selection.
type RateLimitConfig struct {
	// Enable is whether to enable or not
	Enable bool `koanf:"enable"`
	// MaxSelectionsPerDuration specifies how many times a device can be selected
	// within the specified duration. If <= 0, no rate limiting is applied.
	MaxSelectionsPerDuration int `koanf:"max_selections"`

	// Duration specifies the time window for rate limiting (e.g., "1m", "30s")
	Duration time.Duration `koanf:"duration"`
}

// PrometheusConfig holds Prometheus metrics configuration.
type PrometheusConfig struct {
	Enable    bool   `koanf:"enable"`
	Namespace string `koanf:"namespace"`
}

// JobsConfig holds configuration for the jobs system.
type JobsConfig struct {
	Enable bool   `koanf:"enable"`
	Path   string `koanf:"path"`
}

// Config is the top-level application configuration.
type Config struct {
	Instance           string              `koanf:"instance"`
	DeviceListener     *DeviceListener     `koanf:"device_listener"`
	ControllerListener *ControllerListener `koanf:"controller_listener"`
	HTTPListener       *HTTPListener       `koanf:"http_listener"`
	Logging            *logging.Config     `koanf:"logging"`
	RateLimit          *RateLimitConfig    `koanf:"rate_limit"`
	ShutdownTimeout    time.Duration       `koanf:"shutdown_timeout"`
	Tuning             *Tuning             `koanf:"tuning"`
	Prometheus         *PrometheusConfig   `koanf:"prometheus"`
	Jobs               *JobsConfig         `koanf:"jobs"`

	defaultsSet bool
}

// LoadFromFile loads configuration from a TOML file using koanf.
func LoadFromFile(filePath string) (*Config, error) {
	k := koanf.New(".")

	// Load the config file
	if err := k.Load(file.Provider(filePath), toml.Parser()); err != nil {
		return nil, fmt.Errorf("load config file %q: %w", filePath, err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.SetDefaults()

	return &cfg, cfg.Validate()
}

func (d *DeviceListener) setDefaults() {
	if d.Address == "" {
		d.Address = DefaultDeviceAddress
	}
	if d.PingInterval <= 0 {
		d.PingInterval = DefaultDevicePingInterval
	}
	if d.PongWait <= 0 {
		d.PongWait = DefaultDevicePongWait
	}
}

func (c *ControllerListener) setDefaults() {
	if c.Address == "" {
		c.Address = DefaultControllerAddress
	}
	if c.PingInterval <= 0 {
		c.PingInterval = DefaultControllerPingInterval
	}
	if c.PongWait <= 0 {
		c.PongWait = DefaultControllerPongWait
	}
	if c.RegistrationTimeout <= 0 {
		c.RegistrationTimeout = DefaultControllerRegistrationTimeout
	}
}

// SetDefaults sets default values for the configuration.
func (cfg *Config) SetDefaults() {
	// Set global shutdown timeout if not specified
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}

	// Initialize DeviceListener if nil
	if cfg.DeviceListener == nil {
		cfg.DeviceListener = &DeviceListener{}
	}
	cfg.DeviceListener.setDefaults()

	// Initialize ControllerListener if nil
	if cfg.ControllerListener == nil {
		cfg.ControllerListener = &ControllerListener{}
	}
	cfg.ControllerListener.setDefaults()

	// Initialize HTTPListener if nil
	if cfg.HTTPListener == nil {
		cfg.HTTPListener = &HTTPListener{}
	}
	if cfg.HTTPListener.Address == "" {
		cfg.HTTPListener.Address = DefaultHTTPAddress
	}

	// Initialize logging config if nil
	if cfg.Logging == nil {
		cfg.Logging = &logging.Config{}
	}
	cfg.Logging.SetDefaults()
	// File logging is on by default; create FileConfig if not provided
	if cfg.Logging.File == nil {
		cfg.Logging.File = &logging.FileConfig{}
	}
	if !cfg.Logging.File.Disable && cfg.Logging.File.Path == "" {
		cfg.Logging.File.Path = DefaultLogFilePath
	}

	// Initialize rate limit config if nil - no defaults, rate limiting is optional
	if cfg.RateLimit == nil {
		cfg.RateLimit = &RateLimitConfig{}
	}
	// Validate rate limit config values
	if cfg.RateLimit.MaxSelectionsPerDuration <= 0 || cfg.RateLimit.Duration <= 0 {
		cfg.RateLimit.Enable = false // Disable rate limiting if invalid values
	}

	// Initialize Prometheus config if nil
	if cfg.Prometheus == nil {
		cfg.Prometheus = &PrometheusConfig{}
	}

	if cfg.Prometheus.Namespace == "" {
		cfg.Prometheus.Namespace = DefaultPromNamespace
	}

	// Initialize Tuning config if nil
	if cfg.Tuning == nil {
		cfg.Tuning = &Tuning{}
	}

	// Initialize Jobs config if nil
	if cfg.Jobs == nil {
		cfg.Jobs = &JobsConfig{}
	}
	if cfg.Jobs.Path == "" {
		cfg.Jobs.Path = DefaultJobsPath
	}

	cfg.defaultsSet = true
}

// Validate validates the configuration after defaults have been applied.
func (cfg *Config) Validate() error {
	if !cfg.defaultsSet {
		return errors.New("SetDefaults should be called before validing config")
	}

	if cfg.DeviceListener == nil {
		return errors.New("device_listener configuration is required")
	}
	if cfg.ControllerListener == nil {
		return errors.New("controller_listener configuration is required")
	}
	if cfg.HTTPListener == nil {
		return errors.New("http_listener configuration is required")
	}

	// Addresses should have been set by defaults, but validate they're not empty
	if cfg.DeviceListener.Address == "" {
		return errors.New("device_listener address is required")
	}
	if cfg.ControllerListener.Address == "" {
		return errors.New("controller_listener address is required")
	}
	if cfg.HTTPListener.Address == "" {
		return errors.New("http_listener address is required")
	}

	// Validate logging config if present
	if cfg.Logging != nil {
		if err := cfg.Logging.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// GetLogger creates and returns a structured logger based on the logging configuration.
// Returns the logger, a LevelVar for dynamic level changes, and a Closer for shutdown cleanup.
func (cfg *Config) GetLogger() (*slog.Logger, *slog.LevelVar, io.Closer, error) {
	if cfg.Logging == nil {
		return nil, nil, nil, errors.New("logging configuration is not set")
	}

	parsedLevel, err := logging.ParseSlogLevel(cfg.Logging.Level)
	if err != nil {
		return nil, nil, nil, err
	}

	var levelVar slog.LevelVar
	levelVar.Set(parsedLevel)

	writer, err := cfg.Logging.GetLoggingWriter(os.Stdout)
	if err != nil {
		return nil, nil, nil, err
	}

	handler := logging.NewSlogHandler(writer, cfg.Logging.Format, &logging.SlogHandlerOptions{Level: &levelVar})
	logger := logging.CreateSlogLogger(handler)

	return logger, &levelVar, writer, nil
}
