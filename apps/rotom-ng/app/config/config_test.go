package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromFile(t *testing.T) {
	// Create a temporary TOML config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.toml")

	configContent := `shutdown_timeout = "45s"

[device_listener]
address = ":8080"

[controller_listener]
address = ":8081"
secret = "test-controller-secret"

[http_listener]
address = ":8082"
secret = "test-api-secret"

[logging]
level = "debug"
format = "json"

[logging.file]
path = "/tmp/logs/rotom.log"
max_size_mb = 100
max_backups = 5
max_age_days = 30
compress = true
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading the config
	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify the loaded config
	if cfg.DeviceListener == nil {
		t.Fatal("Expected DeviceListener to be set")
	}
	if cfg.DeviceListener.Address != ":8080" {
		t.Errorf("Expected DeviceListener.Address to be ':8080', got %s", cfg.DeviceListener.Address)
	}

	if cfg.ControllerListener == nil {
		t.Fatal("Expected ControllerListener to be set")
	}
	if cfg.ControllerListener.Address != ":8081" {
		t.Errorf("Expected ControllerListener.Address to be ':8081', got %s", cfg.ControllerListener.Address)
	}
	if cfg.ControllerListener.Secret != "test-controller-secret" {
		t.Errorf("Expected ControllerListener.Secret to be 'test-controller-secret', got %s", cfg.ControllerListener.Secret)
	}

	if cfg.HTTPListener == nil {
		t.Fatal("Expected HTTPListener to be set")
	}
	if cfg.HTTPListener.Address != ":8082" {
		t.Errorf("Expected HTTPListener.Address to be ':8082', got %s", cfg.HTTPListener.Address)
	}
	if cfg.HTTPListener.Secret != "test-api-secret" {
		t.Errorf("Expected HTTPListener.Secret to be 'test-api-secret', got %s", cfg.HTTPListener.Secret)
	}

	// Verify global shutdown timeout
	expectedTimeout := 45 * time.Second
	if cfg.ShutdownTimeout != expectedTimeout {
		t.Errorf("Expected ShutdownTimeout to be %v, got %v", expectedTimeout, cfg.ShutdownTimeout)
	}

	if cfg.Logging == nil {
		t.Fatal("Expected Logging config to be set")
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected Level to be 'debug', got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("Expected Format to be 'json', got %s", cfg.Logging.Format)
	}

	if cfg.Logging.File == nil {
		t.Fatal("Expected File config to be set")
	}

	if cfg.Logging.File.Path != "/tmp/logs/rotom.log" {
		t.Errorf("Expected Path to be '/tmp/logs/rotom.log', got %s", cfg.Logging.File.Path)
	}

	if cfg.Logging.File.MaxSizeMB != 100 {
		t.Errorf("Expected MaxSizeMB to be 100, got %d", cfg.Logging.File.MaxSizeMB)
	}

	if cfg.Logging.File.MaxBackups != 5 {
		t.Errorf("Expected MaxBackups to be 5, got %d", cfg.Logging.File.MaxBackups)
	}

	if cfg.Logging.File.MaxAgeDays != 30 {
		t.Errorf("Expected MaxAgeDays to be 30, got %d", cfg.Logging.File.MaxAgeDays)
	}

	if !cfg.Logging.File.Compress {
		t.Error("Expected Compress to be true")
	}
}

func TestLoadFromFileWithoutLoggingFile(t *testing.T) {
	// Create a temporary TOML config file without logging.file section
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config_no_file.toml")

	configContent := `[device_listener]
address = ":8080"

[controller_listener]
address = ":8081"
secret = "test-controller-secret"

[http_listener]
address = ":8082"
secret = "test-api-secret"

[logging]
level = "info"
format = "plain"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading the config
	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify the loaded config
	if cfg.Logging == nil {
		t.Fatal("Expected Logging config to be set")
	}

	// File config should be initialized with defaults (file logging is on by default)
	if cfg.Logging.File == nil {
		t.Fatal("Expected File config to be initialized with defaults")
	}
	if cfg.Logging.File.Path != "./logs/rotom-ng.log" {
		t.Errorf("Expected default File.Path to be './logs/rotom-ng.log', got %s", cfg.Logging.File.Path)
	}
	if cfg.Logging.File.Disable {
		t.Error("Expected File.Disable to be false by default")
	}

	// Other logging settings should still be present
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected Level to be 'info', got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "plain" {
		t.Errorf("Expected Format to be 'plain', got %s", cfg.Logging.Format)
	}
}

func TestLoadFromFileWithoutLoggingSection(t *testing.T) {
	// Create a temporary TOML config file without logging section at all
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config_no_logging.toml")

	configContent := `[device_listener]
address = ":8080"

[controller_listener]
address = ":8081"
secret = "test-controller-secret"

[http_listener]
address = ":8082"
secret = "test-api-secret"
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading the config
	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify the loaded config has defaults
	if cfg.Logging == nil {
		t.Fatal("Expected Logging config to be set with defaults")
	}

	// Should have default values
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default Level to be 'info', got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "plain" {
		t.Errorf("Expected default Format to be 'plain', got %s", cfg.Logging.Format)
	}

	// File config should be initialized with defaults (file logging is on by default)
	if cfg.Logging.File == nil {
		t.Fatal("Expected File config to be initialized with defaults")
	}
	if cfg.Logging.File.Path != "./logs/rotom-ng.log" {
		t.Errorf("Expected default File.Path to be './logs/rotom-ng.log', got %s", cfg.Logging.File.Path)
	}
}

func TestConfigValidationFailures(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectedError string
	}{
		{
			name: "invalid logging level",
			configContent: `[device_listener]
address = ":8080"

[controller_listener]
address = ":8081"
secret = "test-controller-secret"

[http_listener]
address = ":8082"
secret = "test-api-secret"

[logging]
level = "invalid"
`,
			expectedError: "invalid log level 'invalid'",
		},
		{
			name: "invalid log_format",
			configContent: `[device_listener]
address = ":8080"

[controller_listener]
address = ":8081"
secret = "test-controller-secret"

[http_listener]
address = ":8082"
secret = "test-api-secret"

[logging]
format = "xml"
`,
			expectedError: "invalid log format 'xml'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "test_config.toml")

			err := os.WriteFile(configFile, []byte(tt.configContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test config file: %v", err)
			}

			_, err = LoadFromFile(configFile)
			if err == nil {
				t.Errorf("Expected error containing '%s', but got no error", tt.expectedError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
			}
		})
	}
}

func TestConfigSetDefaults(t *testing.T) {
	// Test with minimal config that should get defaults
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config_minimal.toml")

	configContent := `# Minimal config with no secrets (they are optional)
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify defaults were set
	if cfg.DeviceListener == nil {
		t.Fatal("Expected DeviceListener to be initialized")
	}
	if cfg.DeviceListener.Address != DefaultDeviceAddress {
		t.Errorf("Expected default DeviceListener.Address to be '%s', got %s", DefaultDeviceAddress, cfg.DeviceListener.Address)
	}

	if cfg.ControllerListener == nil {
		t.Fatal("Expected ControllerListener to be initialized")
	}
	if cfg.ControllerListener.Address != DefaultControllerAddress {
		t.Errorf("Expected default ControllerListener.Address to be '%s', got %s", DefaultControllerAddress, cfg.ControllerListener.Address)
	}

	if cfg.HTTPListener == nil {
		t.Fatal("Expected HTTPListener to be initialized")
	}
	if cfg.HTTPListener.Address != DefaultHTTPAddress {
		t.Errorf("Expected default HTTPListener.Address to be '%s', got %s", DefaultHTTPAddress, cfg.HTTPListener.Address)
	}

	// Verify default shutdown timeout
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("Expected default ShutdownTimeout to be %v, got %v", DefaultShutdownTimeout, cfg.ShutdownTimeout)
	}

	if cfg.Logging == nil {
		t.Fatal("Expected Logging config to be initialized")
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default Level to be 'info', got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "plain" {
		t.Errorf("Expected default Format to be 'plain', got %s", cfg.Logging.Format)
	}
}

func TestValidateWithoutDefaults(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Expected error when Validate is called without SetDefaults")
	}
	expected := "SetDefaults should be called before"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error containing %q, got %q", expected, err.Error())
	}
}

func TestGetLogger(t *testing.T) {
	// Success case: Config with SetDefaults called
	cfg := &Config{}
	cfg.SetDefaults()

	logger, levelVar, closer, err := cfg.GetLogger()
	if err != nil {
		t.Fatalf("Expected no error from GetLogger after SetDefaults, got %v", err)
	}
	if logger == nil {
		t.Fatal("Expected non-nil logger from GetLogger")
	}
	if levelVar == nil {
		t.Fatal("Expected non-nil levelVar from GetLogger")
	}
	if closer != nil {
		defer closer.Close()
	}

	// Error case: nil Logging field
	cfg2 := &Config{Logging: nil}
	_, _, _, err = cfg2.GetLogger() //nolint:dogsled // testing error path only
	if err == nil {
		t.Fatal("Expected error when Logging is nil")
	}
	expected := "logging configuration is not set"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error containing %q, got %q", expected, err.Error())
	}
}

func TestLoadFromFileNotFound(t *testing.T) {
	_, err := LoadFromFile("nonexistent.toml")
	if err == nil {
		t.Error("Expected error when loading nonexistent file")
	}
}
