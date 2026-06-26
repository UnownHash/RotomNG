package app

import (
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
)

func TestGetDeviceHandlerSettings(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.DeviceListener.PingInterval = 12 * time.Second
	cfg.DeviceListener.PongWait = 4 * time.Second

	s := getDeviceHandlerSettings(cfg)
	if s.PingInterval != 12*time.Second {
		t.Errorf("PingInterval = %v, want 12s", s.PingInterval)
	}
	if s.PongWait != 4*time.Second {
		t.Errorf("PongWait = %v, want 4s", s.PongWait)
	}
}

func TestGetControllerHandlerSettings(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.ControllerListener.PingInterval = 22 * time.Second
	cfg.ControllerListener.PongWait = 6 * time.Second
	cfg.ControllerListener.RegistrationTimeout = 90 * time.Second

	s := getControllerHandlerSettings(cfg)
	if s.PingInterval != 22*time.Second {
		t.Errorf("PingInterval = %v, want 22s", s.PingInterval)
	}
	if s.PongWait != 6*time.Second {
		t.Errorf("PongWait = %v, want 6s", s.PongWait)
	}
	if s.RegistrationTimeout != 90*time.Second {
		t.Errorf("RegistrationTimeout = %v, want 90s", s.RegistrationTimeout)
	}
}

// TestInit_DeviceHandlerPingSettings verifies Init seeds the device handler settings
// container from the device listener config.
func TestInit_DeviceHandlerPingSettings(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.DeviceListener.PingInterval = 17 * time.Second
	cfg.DeviceListener.PongWait = 8 * time.Second

	app, err := newTestApp(t, cfg, newTestFlagConfig())
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if err := app.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got := app.deviceHandlerConfig.GetSettings()
	if got.PingInterval != 17*time.Second || got.PongWait != 8*time.Second {
		t.Errorf("device handler settings = %+v, want 17s/8s", got)
	}
}

// TestReload_AppliesPingSettings verifies a SIGHUP-style reload pushes new ping
// settings into both the device and controller handler settings containers.
func TestReload_AppliesPingSettings(t *testing.T) {
	cfg := newTestConfig(t)

	reloadCfg := newTestConfig(t)
	reloadCfg.DeviceListener.PingInterval = 15 * time.Second
	reloadCfg.DeviceListener.PongWait = 5 * time.Second
	reloadCfg.ControllerListener.PingInterval = 45 * time.Second
	reloadCfg.ControllerListener.PongWait = 7 * time.Second
	reloadCfg.SetDefaults()

	flagCfg := newTestFlagConfig()
	flagCfg.ReloadConfig = func() (*config.Config, error) {
		return reloadCfg, nil
	}

	app, err := newTestApp(t, cfg, flagCfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if err := app.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Initial values come from defaults applied by newTestConfig.
	if got := app.deviceHandlerConfig.GetSettings(); got.PingInterval != config.DefaultDevicePingInterval || got.PongWait != config.DefaultDevicePongWait {
		t.Fatalf("initial device settings = %+v, want defaults", got)
	}

	if err := app.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if got := app.deviceHandlerConfig.GetSettings(); got.PingInterval != 15*time.Second || got.PongWait != 5*time.Second {
		t.Errorf("device settings after reload = %+v, want 15s/5s", got)
	}
	if got := app.controllerHandlerConfig.GetSettings(); got.PingInterval != 45*time.Second || got.PongWait != 7*time.Second {
		t.Errorf("controller settings after reload = %+v, want 45s/7s", got)
	}
}
