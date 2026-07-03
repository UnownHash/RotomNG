package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSetDefaults_PingDefaults verifies the ping interval / pong wait defaults are
// applied to both listeners when not specified.
func TestSetDefaults_PingDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()

	if cfg.DeviceListener.PingInterval != DefaultDevicePingInterval {
		t.Errorf("device PingInterval = %v, want %v", cfg.DeviceListener.PingInterval, DefaultDevicePingInterval)
	}
	if cfg.DeviceListener.PongWait != DefaultDevicePongWait {
		t.Errorf("device PongWait = %v, want %v", cfg.DeviceListener.PongWait, DefaultDevicePongWait)
	}
	if cfg.ControllerListener.PingInterval != DefaultControllerPingInterval {
		t.Errorf("controller PingInterval = %v, want %v", cfg.ControllerListener.PingInterval, DefaultControllerPingInterval)
	}
	if cfg.ControllerListener.PongWait != DefaultControllerPongWait {
		t.Errorf("controller PongWait = %v, want %v", cfg.ControllerListener.PongWait, DefaultControllerPongWait)
	}
	if cfg.ControllerListener.RegistrationTimeout != DefaultControllerRegistrationTimeout {
		t.Errorf("controller RegistrationTimeout = %v, want %v", cfg.ControllerListener.RegistrationTimeout, DefaultControllerRegistrationTimeout)
	}
}

// TestSetDefaults_PingPreservesExplicit verifies explicit (positive) values survive SetDefaults.
func TestSetDefaults_PingPreservesExplicit(t *testing.T) {
	cfg := &Config{
		DeviceListener:     &DeviceListener{PingInterval: 11 * time.Second, PongWait: 3 * time.Second},
		ControllerListener: &ControllerListener{PingInterval: 22 * time.Second, PongWait: 6 * time.Second, RegistrationTimeout: 120 * time.Second},
	}
	cfg.SetDefaults()

	if cfg.DeviceListener.PingInterval != 11*time.Second {
		t.Errorf("device PingInterval = %v, want 11s", cfg.DeviceListener.PingInterval)
	}
	if cfg.DeviceListener.PongWait != 3*time.Second {
		t.Errorf("device PongWait = %v, want 3s", cfg.DeviceListener.PongWait)
	}
	if cfg.ControllerListener.PingInterval != 22*time.Second {
		t.Errorf("controller PingInterval = %v, want 22s", cfg.ControllerListener.PingInterval)
	}
	if cfg.ControllerListener.PongWait != 6*time.Second {
		t.Errorf("controller PongWait = %v, want 6s", cfg.ControllerListener.PongWait)
	}
	if cfg.ControllerListener.RegistrationTimeout != 120*time.Second {
		t.Errorf("controller RegistrationTimeout = %v, want 120s", cfg.ControllerListener.RegistrationTimeout)
	}
}

// TestLoadFromFile_PingValues verifies ping_interval / pong_wait are parsed from TOML.
func TestLoadFromFile_PingValues(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ping_config.toml")

	configContent := `[device_listener]
address = ":8080"
ping_interval = "15s"
pong_wait = "4s"

[controller_listener]
address = ":8081"
ping_interval = "45s"
pong_wait = "7s"
registration_timeout = "90s"

[http_listener]
address = ":8082"

[logging.file]
disable = true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if cfg.DeviceListener.PingInterval != 15*time.Second {
		t.Errorf("device PingInterval = %v, want 15s", cfg.DeviceListener.PingInterval)
	}
	if cfg.DeviceListener.PongWait != 4*time.Second {
		t.Errorf("device PongWait = %v, want 4s", cfg.DeviceListener.PongWait)
	}
	if cfg.ControllerListener.PingInterval != 45*time.Second {
		t.Errorf("controller PingInterval = %v, want 45s", cfg.ControllerListener.PingInterval)
	}
	if cfg.ControllerListener.PongWait != 7*time.Second {
		t.Errorf("controller PongWait = %v, want 7s", cfg.ControllerListener.PongWait)
	}
	if cfg.ControllerListener.RegistrationTimeout != 90*time.Second {
		t.Errorf("controller RegistrationTimeout = %v, want 90s", cfg.ControllerListener.RegistrationTimeout)
	}
}

// TestControllerDataTimeout verifies the controller data_timeout parses from TOML,
// defaults to DefaultControllerDataTimeout when unset, and — thanks to the pointer
// field — distinguishes an explicit 0 (disabled) from unset.
func TestControllerDataTimeout(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "data_timeout.toml")

	configContent := `[controller_listener]
address = ":8081"
data_timeout = "90s"

[http_listener]
address = ":8082"

[logging.file]
disable = true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.ControllerListener.DataTimeout == nil || *cfg.ControllerListener.DataTimeout != 90*time.Second {
		t.Errorf("controller DataTimeout = %v, want 90s", cfg.ControllerListener.DataTimeout)
	}

	// Unset (nil) falls back to the default (2m).
	def := &Config{}
	def.SetDefaults()
	if def.ControllerListener.DataTimeout == nil || *def.ControllerListener.DataTimeout != DefaultControllerDataTimeout {
		t.Errorf("default DataTimeout = %v, want %v", def.ControllerListener.DataTimeout, DefaultControllerDataTimeout)
	}

	// An explicit 0 is preserved (disabled), NOT overridden by the default.
	zero := time.Duration(0)
	c := &Config{ControllerListener: &ControllerListener{DataTimeout: &zero}}
	c.SetDefaults()
	if c.ControllerListener.DataTimeout == nil || *c.ControllerListener.DataTimeout != 0 {
		t.Errorf("explicit 0 DataTimeout = %v, want 0 (disabled)", c.ControllerListener.DataTimeout)
	}

	// A negative value is meaningless and is clamped to disabled (0).
	neg := -5 * time.Second
	n := &Config{ControllerListener: &ControllerListener{DataTimeout: &neg}}
	n.SetDefaults()
	if n.ControllerListener.DataTimeout == nil || *n.ControllerListener.DataTimeout != 0 {
		t.Errorf("negative DataTimeout = %v, want 0 (disabled)", n.ControllerListener.DataTimeout)
	}
}
