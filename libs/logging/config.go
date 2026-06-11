// Package logging provides structured logging configuration and handlers for slog.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

const (
	levelPanic   = "panic"
	levelFatal   = "fatal"
	levelError   = "error"
	levelWarn    = "warn"
	levelWarning = "warning"
	levelInfo    = "info"
	levelDebug   = "debug"
	levelTrace   = "trace"

	formatPlain = "plain"
	formatJSON  = "json"

	fieldSystem    = "system"
	fieldComponent = "component"

	timeFormat = "2006-01-02 15:04:05"

	levelPanicShort = "PANC"
	levelFatalShort = "FATL"
	levelErrorShort = "ERRO"
	levelWarnShort  = "WARN"
	levelInfoShort  = "INFO"
	levelDebugShort = "DEBG"

	levelPanicFull   = "PANIC"
	levelFatalFull   = "FATAL"
	levelErrorFull   = "ERROR"
	levelWarningFull = "WARNING"
	levelInfoFull    = "INFO"
	levelDebugFull   = "DEBUG"
)

var validLevels = map[string]bool{
	levelPanic:   true,
	levelFatal:   true,
	levelError:   true,
	levelWarn:    true,
	levelWarning: true,
	levelInfo:    true,
	levelDebug:   true,
	levelTrace:   true,
}

// FileConfig holds configuration for file-based log output with rotation settings.
type FileConfig struct {
	Disable       bool   `koanf:"disable"`
	Path          string `koanf:"path"`
	MaxSizeMB     int    `koanf:"max_size_mb"`
	MaxBackups    int    `koanf:"max_backups"`
	MaxAgeDays    int    `koanf:"max_age_days"`
	Compress      bool   `koanf:"compress"`
	RotateOnStart bool   `koanf:"rotate_on_start"`
}

// Validate checks that required FileConfig fields are set when file logging is enabled.
func (cfg *FileConfig) Validate() error {
	if cfg.Disable {
		return nil
	}
	if cfg.Path == "" {
		return errors.New("missing 'file.path' in logging config")
	}
	return nil
}

// Config holds the overall logging configuration including level, format, and file output.
type Config struct {
	Level        string      `koanf:"level"`
	Format       string      `koanf:"format"` // "plain" or "json"
	File         *FileConfig `koanf:"file"`
	NoConsoleLog bool        `koanf:"no_console_log"`

	defaultsSet bool
}

// SetDefaults applies default values for Level and Format if not already set.
func (cfg *Config) SetDefaults() {
	if cfg.Level == "" {
		cfg.Level = levelInfo
	}
	if cfg.Format == "" {
		cfg.Format = formatPlain
	}
	cfg.defaultsSet = true
}

// Validate checks that the Config has valid level and format values.
func (cfg *Config) Validate() error {
	if !cfg.defaultsSet {
		return errors.New("logging config defaults were not set before calling Validate")
	}

	if cfg.Level != "" {
		if !validLevels[strings.ToLower(cfg.Level)] {
			return fmt.Errorf("invalid log level '%s'", cfg.Level)
		}
	}

	// Validate format
	format := strings.ToLower(cfg.Format)
	if format != formatPlain && format != formatJSON {
		return fmt.Errorf("invalid log format '%s', valid formats are: plain, json", cfg.Format)
	}

	return nil
}

// ParseSlogLevel maps a level string to a slog.Level value.
// It is case-insensitive and supports all existing level strings:
// trace, debug, info, warn, warning, error, fatal, panic.
func ParseSlogLevel(levelStr string) (slog.Level, error) {
	switch strings.ToLower(levelStr) {
	case levelTrace:
		return LevelTrace, nil
	case levelDebug:
		return slog.LevelDebug, nil
	case levelInfo:
		return slog.LevelInfo, nil
	case levelWarn, levelWarning:
		return slog.LevelWarn, nil
	case levelError:
		return slog.LevelError, nil
	case levelFatal:
		return LevelFatal, nil
	case levelPanic:
		return LevelPanic, nil
	default:
		return 0, fmt.Errorf("invalid slog level '%s'", levelStr)
	}
}

// GetLoggingWriter delegates to the package-level GetLoggingWriter function.
func (cfg *Config) GetLoggingWriter(consoleWriter io.Writer) (io.WriteCloser, error) {
	return GetLoggingWriter(cfg, consoleWriter)
}
