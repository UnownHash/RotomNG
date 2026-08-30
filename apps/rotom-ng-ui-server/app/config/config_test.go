package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rotom-ng-ui.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadFromFileDefaults(t *testing.T) {
	path := writeConfig(t, "")

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if cfg.HTTPListener.Address != DefaultHTTPAddress {
		t.Errorf("address = %q, want %q", cfg.HTTPListener.Address, DefaultHTTPAddress)
	}
	if cfg.HTTPListener.UISessionTTL != DefaultUISessionTTL {
		t.Errorf("ui_session_ttl = %v, want %v", cfg.HTTPListener.UISessionTTL, DefaultUISessionTTL)
	}
	if cfg.InstanceMonitor.Interval != DefaultMonitorInterval {
		t.Errorf("interval = %v, want %v", cfg.InstanceMonitor.Interval, DefaultMonitorInterval)
	}
	if cfg.InstanceMonitor.Timeout != DefaultMonitorTimeout {
		t.Errorf("timeout = %v, want %v", cfg.InstanceMonitor.Timeout, DefaultMonitorTimeout)
	}
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("shutdown_timeout = %v, want %v", cfg.ShutdownTimeout, DefaultShutdownTimeout)
	}
	// An operator may legitimately run with none configured yet; that must
	// load rather than refuse to start.
	if len(cfg.Instances) != 0 {
		t.Errorf("instances = %v, want none", cfg.Instances)
	}
}

func TestLoadFromFileFull(t *testing.T) {
	path := writeConfig(t, `
instance = "admin"
shutdown_timeout = "9s"

[http_listener]
address = ":9999"
secret = "admin-secret"
ui_session_ttl = "30m"

[instance_monitor]
interval = "3s"
timeout = "1s"

[[instances]]
base_url = "http://one:7072/"
api_secret = "one-secret"

[[instances]]
base_url = "http://two:7072"
`)

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if cfg.Instance != "admin" {
		t.Errorf("instance = %q, want %q", cfg.Instance, "admin")
	}
	if cfg.HTTPListener.Address != ":9999" || cfg.HTTPListener.Secret != "admin-secret" {
		t.Errorf("http_listener = %+v", cfg.HTTPListener)
	}
	if cfg.HTTPListener.UISessionTTL != 30*time.Minute {
		t.Errorf("ui_session_ttl = %v, want 30m", cfg.HTTPListener.UISessionTTL)
	}
	if cfg.ShutdownTimeout != 9*time.Second {
		t.Errorf("shutdown_timeout = %v, want 9s", cfg.ShutdownTimeout)
	}
	if cfg.InstanceMonitor.Interval != 3*time.Second || cfg.InstanceMonitor.Timeout != time.Second {
		t.Errorf("instance_monitor = %+v", cfg.InstanceMonitor)
	}

	if len(cfg.Instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(cfg.Instances))
	}
	// The trailing slash must be gone: it would otherwise produce "//api"
	// upstream and defeat the base-URL lookup the proxy does.
	if cfg.Instances[0].BaseURL != "http://one:7072" {
		t.Errorf("instances[0].base_url = %q, want %q", cfg.Instances[0].BaseURL, "http://one:7072")
	}
	if cfg.Instances[0].APISecret != "one-secret" {
		t.Errorf("instances[0].api_secret = %q", cfg.Instances[0].APISecret)
	}
	if cfg.Instances[1].APISecret != "" {
		t.Errorf("instances[1].api_secret = %q, want empty", cfg.Instances[1].APISecret)
	}
}

func TestLoadFromFileRejectsBadInstances(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name:     "missing base url",
			contents: "[[instances]]\napi_secret = \"x\"\n",
			wantErr:  "base_url is required",
		},
		{
			name:     "not a url",
			contents: "[[instances]]\nbase_url = \"one:7072\"\n",
			wantErr:  "must be an http:// or https:// URL",
		},
		{
			name:     "no host",
			contents: "[[instances]]\nbase_url = \"http://\"\n",
			wantErr:  "has no host",
		},
		{
			name:     "duplicate",
			contents: "[[instances]]\nbase_url = \"http://one:7072\"\n[[instances]]\nbase_url = \"http://one:7072/\"\n",
			wantErr:  "duplicate base_url",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadFromFile(writeConfig(t, test.contents))
			if err == nil {
				t.Fatalf("LoadFromFile succeeded, want error containing %q", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateRequiresDefaults(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate on a config without defaults succeeded, want error")
	}
}
