// Package config provides configuration loading and validation for the
// RotomNG admin UI server.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/UnownHash/RotomNG/libs/logging"
)

// Default configuration values.
const (
	DefaultHTTPAddress     = ":7073"
	DefaultShutdownTimeout = 5 * time.Second

	DefaultLogFilePath = "./logs/rotom-ng-ui.log"

	// DefaultUISessionTTL is how long a web UI login lasts when
	// http_listener.ui_session_ttl is not set: one day. Written as 24h because
	// Go duration syntax has no day unit.
	DefaultUISessionTTL = 24 * time.Hour

	// DefaultMonitorInterval is how often each configured instance is probed
	// for reachability when instance_monitor.interval is not set.
	DefaultMonitorInterval = 10 * time.Second

	// DefaultMonitorTimeout bounds a single reachability probe when
	// instance_monitor.timeout is not set. Kept well under the interval so a
	// hung instance cannot delay the next round.
	DefaultMonitorTimeout = 5 * time.Second
)

// HTTPListener holds configuration for the HTTP API listener. It mirrors
// rotom-ng's own http_listener section so a single set of operator habits
// covers both services.
type HTTPListener struct {
	Address  string       `koanf:"address"`
	Listener net.Listener `koanf:"-"`
	Secret   string       `koanf:"secret"`
	// UISessionTTL is how long a web UI login stays valid (e.g. "30m", "12h").
	// Defaults to DefaultUISessionTTL when unset or <= 0. Only relevant when
	// Secret is set, since without a secret the UI never logs in.
	UISessionTTL time.Duration `koanf:"ui_session_ttl"`
}

// Instance identifies one rotom-ng server this service fronts.
type Instance struct {
	// BaseURL is the root of the rotom-ng HTTP listener, without the /api
	// prefix -- e.g. "http://10.0.0.4:7072". The prefix is appended when
	// building upstream request URLs.
	BaseURL string `koanf:"base_url"`
	// APISecret is that instance's http_listener secret, sent upstream as the
	// X-Rotom-Secret header. Empty when the instance has no secret set.
	APISecret string `koanf:"api_secret"`
}

// InstanceMonitor tunes the background reachability probes.
type InstanceMonitor struct {
	Interval time.Duration `koanf:"interval"`
	Timeout  time.Duration `koanf:"timeout"`
}

// Config is the top-level application configuration.
//
// It deliberately carries none of rotom-ng's device, controller, jobs, or
// rate-limit settings: this service owns no connections, and those settings
// belong to -- and are read from -- the instances it fronts.
type Config struct {
	Instance        string           `koanf:"instance"`
	HTTPListener    *HTTPListener    `koanf:"http_listener"`
	Instances       []Instance       `koanf:"instances"`
	InstanceMonitor *InstanceMonitor `koanf:"instance_monitor"`
	Logging         *logging.Config  `koanf:"logging"`
	ShutdownTimeout time.Duration    `koanf:"shutdown_timeout"`

	defaultsSet bool
}

// LoadFromFile loads configuration from a TOML file using koanf.
func LoadFromFile(filePath string) (*Config, error) {
	k := koanf.New(".")

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

// SetDefaults sets default values for the configuration.
func (cfg *Config) SetDefaults() {
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}

	if cfg.HTTPListener == nil {
		cfg.HTTPListener = &HTTPListener{}
	}
	if cfg.HTTPListener.Address == "" {
		cfg.HTTPListener.Address = DefaultHTTPAddress
	}
	if cfg.HTTPListener.UISessionTTL <= 0 {
		cfg.HTTPListener.UISessionTTL = DefaultUISessionTTL
	}

	if cfg.InstanceMonitor == nil {
		cfg.InstanceMonitor = &InstanceMonitor{}
	}
	if cfg.InstanceMonitor.Interval <= 0 {
		cfg.InstanceMonitor.Interval = DefaultMonitorInterval
	}
	if cfg.InstanceMonitor.Timeout <= 0 {
		cfg.InstanceMonitor.Timeout = DefaultMonitorTimeout
	}

	// Trailing slashes would produce "//api" once the prefix is appended, and
	// they also break the exact-match lookup the proxy does on the base URL,
	// so normalise them away before anything else sees the value.
	for idx := range cfg.Instances {
		cfg.Instances[idx].BaseURL = strings.TrimRight(cfg.Instances[idx].BaseURL, "/")
	}

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

	cfg.defaultsSet = true
}

// Validate validates the configuration after defaults have been applied.
func (cfg *Config) Validate() error {
	if !cfg.defaultsSet {
		return errors.New("SetDefaults should be called before validing config")
	}

	if cfg.HTTPListener == nil {
		return errors.New("http_listener configuration is required")
	}
	if cfg.HTTPListener.Address == "" {
		return errors.New("http_listener address is required")
	}

	if err := cfg.validateInstances(); err != nil {
		return err
	}

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

// validateInstances checks that every instance has a usable, distinct base
// URL. Duplicates are rejected rather than merged: the UI selects an instance
// by its base URL, so two entries sharing one would be indistinguishable.
func (cfg *Config) validateInstances() error {
	seen := make(map[string]struct{}, len(cfg.Instances))
	for idx, instance := range cfg.Instances {
		if instance.BaseURL == "" {
			return fmt.Errorf("instances[%d]: base_url is required", idx)
		}
		parsed, err := url.Parse(instance.BaseURL)
		if err != nil {
			return fmt.Errorf("instances[%d]: invalid base_url %q: %w", idx, instance.BaseURL, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("instances[%d]: base_url %q must be an http:// or https:// URL", idx, instance.BaseURL)
		}
		if parsed.Host == "" {
			return fmt.Errorf("instances[%d]: base_url %q has no host", idx, instance.BaseURL)
		}
		if _, dup := seen[instance.BaseURL]; dup {
			return fmt.Errorf("instances[%d]: duplicate base_url %q", idx, instance.BaseURL)
		}
		seen[instance.BaseURL] = struct{}{}
	}
	return nil
}
