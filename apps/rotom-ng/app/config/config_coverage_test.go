package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_NilDeviceListener(t *testing.T) {
	cfg := &Config{defaultsSet: true, DeviceListener: nil}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "device_listener") {
		t.Errorf("expected device_listener error, got: %v", err)
	}
}

func TestValidate_NilControllerListener(t *testing.T) {
	cfg := &Config{
		defaultsSet:    true,
		DeviceListener: &DeviceListener{Address: ":7070"},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "controller_listener") {
		t.Errorf("expected controller_listener error, got: %v", err)
	}
}

func TestValidate_NilHTTPListener(t *testing.T) {
	cfg := &Config{
		defaultsSet:        true,
		DeviceListener:     &DeviceListener{Address: ":7070"},
		ControllerListener: &ControllerListener{Address: ":7071"},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "http_listener") {
		t.Errorf("expected http_listener error, got: %v", err)
	}
}

func TestValidate_EmptyDeviceAddress(t *testing.T) {
	cfg := &Config{
		defaultsSet:        true,
		DeviceListener:     &DeviceListener{Address: ""},
		ControllerListener: &ControllerListener{Address: ":7071"},
		HTTPListener:       &HTTPListener{Address: ":7072"},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "device_listener address") {
		t.Errorf("expected device_listener address error, got: %v", err)
	}
}

func TestValidate_EmptyControllerAddress(t *testing.T) {
	cfg := &Config{
		defaultsSet:        true,
		DeviceListener:     &DeviceListener{Address: ":7070"},
		ControllerListener: &ControllerListener{Address: ""},
		HTTPListener:       &HTTPListener{Address: ":7072"},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "controller_listener address") {
		t.Errorf("expected controller_listener address error, got: %v", err)
	}
}

func TestValidate_EmptyHTTPAddress(t *testing.T) {
	cfg := &Config{
		defaultsSet:        true,
		DeviceListener:     &DeviceListener{Address: ":7070"},
		ControllerListener: &ControllerListener{Address: ":7071"},
		HTTPListener:       &HTTPListener{Address: ""},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "http_listener address") {
		t.Errorf("expected http_listener address error, got: %v", err)
	}
}

func TestValidate_NilLogging(t *testing.T) {
	cfg := &Config{
		defaultsSet:        true,
		DeviceListener:     &DeviceListener{Address: ":7070"},
		ControllerListener: &ControllerListener{Address: ":7071"},
		HTTPListener:       &HTTPListener{Address: ":7072"},
		Logging:            nil,
	}
	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected nil error when Logging is nil, got: %v", err)
	}
}

func TestLoadFromFile_InvalidTOML(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "bad.toml")

	// Write invalid TOML that will parse but unmarshal incorrectly
	// Actually, koanf's TOML parser is lenient. Let's write something that
	// causes a parse error.
	err := os.WriteFile(configFile, []byte("[[[[invalid toml"), 0644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err = LoadFromFile(configFile)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}
