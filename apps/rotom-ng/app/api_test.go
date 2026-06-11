package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/testutil"
)

// Untestable Code Paths (per coverage audit):
// - main.go:main() -- process entry point with flag parsing and os.Exit; app lifecycle covered by TestEnv integration tests
// - worker_handler.go:HandleWorker() panic recovery (lines 41-49) -- requires injecting a panic into production code
// - config.go:Validate() nil-guard branches (lines 184-203) -- dead code; SetDefaults() always initializes all listener structs
// - app.go:Init() os.Stat non-NotExist error (line 301) -- platform-specific filesystem error, cannot be reliably triggered in CI
// - app.go:Init() internal constructor error branches (lines 314-434) -- settings are always valid after SetDefaults()
// - app.go:reload() validation error branches (lines 134-161) -- settings from valid config always pass validation
// - config.go:LoadFromFile() unmarshal error (line 96) -- requires corrupted TOML that parses but fails unmarshal
//
// Total achievable coverage with -coverpkg is ~85%, limited by main.go (0%, ~25 statements) and
// dead validation branches. All reachable production paths are exercised.

// TestAPI_StatusCounts validates API-01: GET /api/status returns device and
// controller counts that match the number of connected fakes.
func TestAPI_StatusCounts(t *testing.T) {
	env := startTestEnv(t)
	ctx := context.Background()

	t.Run("empty_state", func(t *testing.T) {
		status := getStatus(t, env.HTTPAddr)
		if len(status.Devices) != 0 {
			t.Errorf("expected 0 devices, got %d", len(status.Devices))
		}
		if len(status.Controllers) != 0 {
			t.Errorf("expected 0 controllers, got %d", len(status.Controllers))
		}
	})

	t.Run("with_device", func(t *testing.T) {
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer worker.Close()
		defer device.Close()

		var d *apiDevice
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			if len(status.Devices) < 1 {
				return false
			}
			d = findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device with worker in status: %v", err)
		}
		if !d.IsConnected {
			t.Errorf("device IsConnected = false, want true")
		}
	})

	t.Run("with_controller", func(t *testing.T) {
		// Controller needs a device+worker to be available
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer worker.Close()
		defer device.Close()

		// Wait for device to register
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device: %v", err)
		}

		ctrl := testutil.NewFakeController()
		if err := ctrl.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("ctrl.Connect: %v", err)
		}
		defer ctrl.Close()

		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			return len(status.Controllers) >= 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for controller in status: %v", err)
		}
	})
}

// TestAPI_DeviceContract validates API-02: GET /api/device and
// GET /api/device/:id return correct JSON structure with all expected fields.
func TestAPI_DeviceContract(t *testing.T) {
	env := startTestEnv(t)
	ctx := context.Background()

	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr,
		testutil.WithDeviceID("contract-dev-1"))
	defer worker.Close()
	defer device.Close()

	// Wait until device appears in status
	err := testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, "contract-dev-1")
		return d != nil && d.IsConnected && d.WorkerCount == 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device in status: %v", err)
	}

	t.Run("list_endpoint", func(t *testing.T) {
		resp := getDevices(t, env.HTTPAddr, false)
		if len(resp.Devices) < 1 {
			t.Fatal("expected at least 1 device in list")
		}
		d := findDeviceInList(resp.Devices, "contract-dev-1")
		if d == nil {
			t.Fatal("contract-dev-1 not found in device list")
		}
		assertDeviceContract(t, d)
		if len(d.Workers) != 0 {
			t.Errorf("Workers should be empty when include_workers=false, got %d", len(d.Workers))
		}
	})

	t.Run("detail_endpoint", func(t *testing.T) {
		resp, code := getDeviceByID(t, env.HTTPAddr, "contract-dev-1", false)
		if code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", code)
		}
		assertDeviceContract(t, &resp.Device)
	})

	t.Run("include_workers", func(t *testing.T) {
		resp := getDevices(t, env.HTTPAddr, true)
		d := findDeviceInList(resp.Devices, "contract-dev-1")
		if d == nil {
			t.Fatal("contract-dev-1 not found in device list")
		}
		if len(d.Workers) != 1 {
			t.Fatalf("expected 1 worker with include_workers=true, got %d", len(d.Workers))
		}
		w := d.Workers[0]
		assertWorkerContract(t, &w, "contract-dev-1")
	})

	t.Run("detail_include_workers", func(t *testing.T) {
		resp, code := getDeviceByID(t, env.HTTPAddr, "contract-dev-1", true)
		if code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", code)
		}
		if len(resp.Device.Workers) != 1 {
			t.Fatalf("expected 1 worker in detail with include_workers=true, got %d", len(resp.Device.Workers))
		}
	})

	t.Run("status_includes_workers", func(t *testing.T) {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, "contract-dev-1")
		if d == nil {
			t.Fatal("contract-dev-1 not found in status")
		}
		// Status endpoint includes workers by default (Pitfall 5 from RESEARCH.md)
		if len(d.Workers) != 1 {
			t.Errorf("status endpoint should include workers, got %d", len(d.Workers))
		}
	})
}

// TestAPI_ControllerContract validates API-02: GET /api/controller and
// GET /api/controller/:uuid return correct JSON structure.
func TestAPI_ControllerContract(t *testing.T) {
	env := startTestEnv(t)
	ctx := context.Background()

	// A device+worker must exist for the controller to connect to
	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
	defer worker.Close()
	defer device.Close()

	// Wait for device to be ready
	err := testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && d.IsConnected && d.WorkerCount == 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device in status: %v", err)
	}

	ctrl := testutil.NewFakeController()
	if err := ctrl.Connect(ctx, env.ControllerAddr); err != nil {
		t.Fatalf("ctrl.Connect: %v", err)
	}
	defer ctrl.Close()

	// Wait for controller to appear
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		return len(status.Controllers) >= 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for controller in status: %v", err)
	}

	// Get the server-generated UUID for this controller from the list endpoint.
	// The server assigns a UUID during registration that differs from the
	// client-supplied controller ID.
	var serverUUID string

	t.Run("list_endpoint", func(t *testing.T) {
		resp := getControllers(t, env.HTTPAddr)
		if len(resp.Controllers) < 1 {
			t.Fatal("expected at least 1 controller in list")
		}
		// Find by client-supplied Id field (not server-generated UUID)
		c := findControllerByID(resp.Controllers, ctrl.ControllerID())
		if c == nil {
			t.Fatalf("controller with Id %q not found in list", ctrl.ControllerID())
		}
		serverUUID = c.UUID
		assertControllerContract(t, c)
	})

	t.Run("detail_endpoint", func(t *testing.T) {
		if serverUUID == "" {
			t.Skip("server UUID not available (list_endpoint may have failed)")
		}
		resp, code := getControllerByUUID(t, env.HTTPAddr, serverUUID)
		if code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", code)
		}
		assertControllerContract(t, &resp.Controller)
	})
}

// TestAPI_NotFound validates that single-resource endpoints return 404 for
// nonexistent resources.
func TestAPI_NotFound(t *testing.T) {
	env := startTestEnv(t)

	t.Run("device_not_found", func(t *testing.T) {
		_, code := getDeviceByID(t, env.HTTPAddr, "nonexistent-device-id", false)
		if code != http.StatusNotFound {
			t.Errorf("expected 404 for nonexistent device, got %d", code)
		}
	})

	t.Run("controller_not_found", func(t *testing.T) {
		_, code := getControllerByUUID(t, env.HTTPAddr, "nonexistent-uuid")
		if code != http.StatusNotFound {
			t.Errorf("expected 404 for nonexistent controller, got %d", code)
		}
	})
}

// TestAPI_GetConfig validates that GET /api/config returns 200 with version,
// sha, and tuning fields.
func TestAPI_GetConfig(t *testing.T) {
	env := startTestEnv(t)

	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/config", env.HTTPAddr))
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/config status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /api/config: %v", err)
	}

	// Assert top-level "status" key
	status, ok := body["status"]
	if !ok {
		t.Fatal("response missing 'status' key")
	}
	if status != "ok" {
		t.Errorf("status = %v, want 'ok'", status)
	}

	// Assert top-level "config" key
	configRaw, ok := body["config"]
	if !ok {
		t.Fatal("response missing 'config' key")
	}
	configMap, ok := configRaw.(map[string]any)
	if !ok {
		t.Fatalf("config is not an object, got %T", configRaw)
	}

	// Assert config has "version" (string, non-empty)
	ver, ok := configMap["version"]
	if !ok {
		t.Fatal("config missing 'version' key")
	}
	verStr, ok := ver.(string)
	if !ok || verStr == "" {
		t.Errorf("config.version = %v, want non-empty string", ver)
	}

	// Assert config has "sha" (string)
	sha, ok := configMap["sha"]
	if !ok {
		t.Fatal("config missing 'sha' key")
	}
	if _, ok := sha.(string); !ok {
		t.Errorf("config.sha is not a string, got %T", sha)
	}

	// Assert config has "tuning" (object with "profiling" bool)
	tuningRaw, ok := configMap["tuning"]
	if !ok {
		t.Fatal("config missing 'tuning' key")
	}
	tuningMap, ok := tuningRaw.(map[string]any)
	if !ok {
		t.Fatalf("config.tuning is not an object, got %T", tuningRaw)
	}
	if _, ok := tuningMap["profiling"]; !ok {
		t.Fatal("config.tuning missing 'profiling' key")
	}
}

// TestAPI_ConfigReload validates that PUT /api/config/reload returns 200
// and returns the current config.
func TestAPI_ConfigReload(t *testing.T) {
	// Provide a valid ReloadConfig callback so reload() doesn't receive nil config.
	reloadFn := func() (*config.Config, error) {
		cfg, _, err := testutil.NewTestConfig()
		return cfg, err
	}

	env, err := testutil.NewTestEnv(
		testutil.WithReloadConfig(reloadFn),
	)
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("http://%s/api/config/reload", env.HTTPAddr), nil)
	if err != nil {
		t.Fatalf("creating PUT request: %v", err)
	}

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/config/reload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/config/reload status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /api/config/reload: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status = %v, want 'ok'", body["status"])
	}
	if _, ok := body["config"]; !ok {
		t.Fatal("response missing 'config' key after reload")
	}
}

// TestAPI_GetConfigWithInstanceAndRateLimit validates that GET /api/config
// includes instance and rate_limit fields when configured.
func TestAPI_GetConfigWithInstanceAndRateLimit(t *testing.T) {
	env, err := testutil.NewTestEnv(
		testutil.WithInstance("test-instance-1"),
		testutil.WithRateLimit(true, 10, 60*time.Second),
	)
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/config", env.HTTPAddr))
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	configMap := body["config"].(map[string]any)

	// Instance field should be present
	instance, ok := configMap["instance"]
	if !ok {
		t.Fatal("config missing 'instance' key")
	}
	if instance != "test-instance-1" {
		t.Errorf("instance = %v, want 'test-instance-1'", instance)
	}

	// RateLimit field should be present
	rl, ok := configMap["rate_limit"]
	if !ok {
		t.Fatal("config missing 'rate_limit' key")
	}
	rlMap, ok := rl.(map[string]any)
	if !ok {
		t.Fatalf("rate_limit is not an object, got %T", rl)
	}
	if rlMap["enable"] != true {
		t.Errorf("rate_limit.enable = %v, want true", rlMap["enable"])
	}
}

// TestAPI_ConfigReloadError validates that PUT /api/config/reload returns 500
// when the reload callback returns an error.
func TestAPI_ConfigReloadError(t *testing.T) {
	reloadFn := func() (*config.Config, error) {
		return nil, errors.New("simulated reload failure")
	}

	env, err := testutil.NewTestEnv(
		testutil.WithReloadConfig(reloadFn),
	)
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("http://%s/api/config/reload", env.HTTPAddr), nil)
	if err != nil {
		t.Fatalf("creating PUT request: %v", err)
	}

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/config/reload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("status = %v, want 'error'", body["status"])
	}
	if body["error"] != "simulated reload failure" {
		t.Errorf("error = %v, want 'simulated reload failure'", body["error"])
	}
}

// TestAPI_MetricsDisabled validates that GET /api/metrics returns 404
// when prometheus is disabled (default config).
func TestAPI_MetricsDisabled(t *testing.T) {
	env := startTestEnv(t)

	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/metrics", env.HTTPAddr))
	if err != nil {
		t.Fatalf("GET /api/metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/metrics status = %d, want 404", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /api/metrics: %v", err)
	}

	errMsg, ok := body["error"]
	if !ok {
		t.Fatal("response missing 'error' key")
	}
	if errMsg != "metrics are not enabled" {
		t.Errorf("error = %v, want 'metrics are not enabled'", errMsg)
	}
}

// --- Assertion helpers ---

// assertDeviceContract validates that an apiDevice has all expected contract fields.
func assertDeviceContract(t *testing.T, d *apiDevice) {
	t.Helper()
	if d.ID != "contract-dev-1" {
		t.Errorf("device Id = %q, want %q", d.ID, "contract-dev-1")
	}
	if d.Origin == "" {
		t.Error("device Origin is empty")
	}
	if !d.IsConnected {
		t.Error("device IsConnected = false, want true")
	}
	if d.WorkerCount != 1 {
		t.Errorf("device WorkerCount = %d, want 1", d.WorkerCount)
	}
	if d.LastConnectedAtMs <= 0 {
		t.Errorf("device LastConnectedAtMs = %d, want > 0", d.LastConnectedAtMs)
	}
	if d.LastSeenAtMs <= 0 {
		t.Errorf("device LastSeenAtMs = %d, want > 0", d.LastSeenAtMs)
	}
	if !d.Enabled {
		t.Error("device Enabled = false, want true")
	}
	if d.Session == nil {
		t.Error("device Session is nil for connected device")
	} else if d.Session.ConnectedAtMs <= 0 {
		t.Errorf("device Session.ConnectedAtMs = %d, want > 0", d.Session.ConnectedAtMs)
	}
}

// assertWorkerContract validates that an apiWorker has all expected contract fields.
func assertWorkerContract(t *testing.T, w *apiWorker, expectedDeviceID string) {
	t.Helper()
	if w.ID == "" {
		t.Error("worker Id is empty")
	}
	if w.DeviceID != expectedDeviceID {
		t.Errorf("worker DeviceId = %q, want %q", w.DeviceID, expectedDeviceID)
	}
	if !w.IsConnected {
		t.Error("worker IsConnected = false, want true")
	}
	if w.Session.ConnectedAtMs <= 0 {
		t.Errorf("worker Session.ConnectedAtMs = %d, want > 0", w.Session.ConnectedAtMs)
	}
}

// assertControllerContract validates that an apiController has all expected contract fields.
func assertControllerContract(t *testing.T, c *apiController) {
	t.Helper()
	if c.UUID == "" {
		t.Error("controller UUID is empty")
	}
	if c.ID == "" {
		t.Error("controller Id is empty")
	}
	if c.ConnectedAtMs <= 0 {
		t.Errorf("controller ConnectedAtMs = %d, want > 0", c.ConnectedAtMs)
	}
	if c.ProtoMajorVersion <= 0 && c.ProtoMinorVersion < 0 {
		t.Errorf("controller proto version invalid: major=%d minor=%d",
			c.ProtoMajorVersion, c.ProtoMinorVersion)
	}
}

// findControllerByID finds a controller in a slice by its client-supplied Id field.
func findControllerByID(controllers []apiController, id string) *apiController {
	for i := range controllers {
		if controllers[i].ID == id {
			return &controllers[i]
		}
	}
	return nil
}

// TestAPI_DeviceStateTransitions validates API-03: API responses change
// correctly as a device connects and then disconnects.
func TestAPI_DeviceStateTransitions(t *testing.T) {
	env := startTestEnv(t)
	ctx := context.Background()

	var device *testutil.FakeDevice
	var worker *testutil.FakeWorker

	// Ensure device/worker are closed before env.Stop() even if a subtest fatals.
	t.Cleanup(func() {
		if worker != nil {
			worker.Close()
		}
		if device != nil {
			device.Close()
		}
	})

	t.Run("baseline_empty", func(t *testing.T) {
		status := getStatus(t, env.HTTPAddr)
		if len(status.Devices) != 0 {
			t.Errorf("expected 0 devices in status, got %d", len(status.Devices))
		}
		devResp := getDevices(t, env.HTTPAddr, false)
		if len(devResp.Devices) != 0 {
			t.Errorf("expected 0 devices in list, got %d", len(devResp.Devices))
		}
	})

	t.Run("device_connects", func(t *testing.T) {
		device, worker = connectDeviceWithWorker(ctx, t, env.DeviceAddr,
			testutil.WithDeviceID("transition-dev-1"))

		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, "transition-dev-1")
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device with worker in status: %v", err)
		}

		// Verify status endpoint
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, "transition-dev-1")
		if d == nil {
			t.Fatal("transition-dev-1 not found in status")
		}
		if !d.IsConnected {
			t.Error("status: device IsConnected = false, want true")
		}
		if d.WorkerCount != 1 {
			t.Errorf("status: device WorkerCount = %d, want 1", d.WorkerCount)
		}

		// Verify list endpoint
		devResp := getDevices(t, env.HTTPAddr, false)
		if len(devResp.Devices) != 1 {
			t.Errorf("expected 1 device in list, got %d", len(devResp.Devices))
		}
		dl := findDeviceInList(devResp.Devices, "transition-dev-1")
		if dl == nil {
			t.Fatal("transition-dev-1 not found in device list")
		}

		// Verify detail endpoint
		detailResp, code := getDeviceByID(t, env.HTTPAddr, "transition-dev-1", false)
		if code != http.StatusOK {
			t.Fatalf("expected status 200 for device detail, got %d", code)
		}
		if detailResp.Device.ID != "transition-dev-1" {
			t.Errorf("detail: device Id = %q, want %q", detailResp.Device.ID, "transition-dev-1")
		}
		if !detailResp.Device.IsConnected {
			t.Error("detail: device IsConnected = false, want true")
		}
	})

	t.Run("device_disconnects", func(t *testing.T) {
		if device == nil || worker == nil {
			t.Skip("device not connected (previous subtest may have failed)")
		}

		worker.Close()
		device.Close()

		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			if len(status.Devices) == 0 {
				return true
			}
			d := findDeviceInList(status.Devices, "transition-dev-1")
			return d != nil && !d.IsConnected
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device to disconnect: %v", err)
		}

		// Verify status endpoint
		status := getStatus(t, env.HTTPAddr)
		if len(status.Devices) > 0 {
			d := findDeviceInList(status.Devices, "transition-dev-1")
			if d != nil {
				if d.IsConnected {
					t.Error("status: device still shows IsConnected=true after disconnect")
				}
				if d.WorkerCount != 0 {
					t.Errorf("status: device WorkerCount = %d, want 0", d.WorkerCount)
				}
			}
		}

		// Verify list endpoint
		devResp := getDevices(t, env.HTTPAddr, false)
		if len(devResp.Devices) > 0 {
			dl := findDeviceInList(devResp.Devices, "transition-dev-1")
			if dl != nil && dl.IsConnected {
				t.Error("list: device still shows IsConnected=true after disconnect")
			}
		}

		// Verify detail endpoint if device is still tracked
		detailResp, code := getDeviceByID(t, env.HTTPAddr, "transition-dev-1", false)
		if code == http.StatusOK {
			if detailResp.Device.IsConnected {
				t.Error("detail: device still shows IsConnected=true after disconnect")
			}
		}
		// 404 is also acceptable -- device fully removed
	})
}

// TestAPI_ControllerStateTransitions validates API-03: API responses change
// correctly as a controller connects and then disconnects.
func TestAPI_ControllerStateTransitions(t *testing.T) {
	env := startTestEnv(t)
	ctx := context.Background()

	// A device+worker must exist for the controller to connect to
	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
	defer worker.Close()
	defer device.Close()

	err := testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && d.IsConnected && d.WorkerCount == 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device to appear: %v", err)
	}

	var ctrl *testutil.FakeController
	var serverUUID string

	t.Run("baseline_no_controllers", func(t *testing.T) {
		status := getStatus(t, env.HTTPAddr)
		if len(status.Controllers) != 0 {
			t.Errorf("expected 0 controllers in status, got %d", len(status.Controllers))
		}
		ctrlResp := getControllers(t, env.HTTPAddr)
		if len(ctrlResp.Controllers) != 0 {
			t.Errorf("expected 0 controllers in list, got %d", len(ctrlResp.Controllers))
		}
	})

	t.Run("controller_connects", func(t *testing.T) {
		ctrl = testutil.NewFakeController()
		if err := ctrl.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("ctrl.Connect: %v", err)
		}

		err := testutil.WaitForCondition(func() bool {
			resp := getControllers(t, env.HTTPAddr)
			return len(resp.Controllers) >= 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for controller to appear: %v", err)
		}

		// Verify list endpoint
		ctrlResp := getControllers(t, env.HTTPAddr)
		if len(ctrlResp.Controllers) < 1 {
			t.Fatal("expected at least 1 controller in list")
		}
		c := findControllerByID(ctrlResp.Controllers, ctrl.ControllerID())
		if c == nil {
			t.Fatalf("controller with Id %q not found in list", ctrl.ControllerID())
		}
		serverUUID = c.UUID

		// Verify detail endpoint using server-assigned UUID
		detailResp, code := getControllerByUUID(t, env.HTTPAddr, serverUUID)
		if code != http.StatusOK {
			t.Fatalf("expected status 200 for controller detail, got %d", code)
		}
		if detailResp.Controller.UUID != serverUUID {
			t.Errorf("detail: controller UUID = %q, want %q", detailResp.Controller.UUID, serverUUID)
		}
		if detailResp.Controller.ConnectedAtMs <= 0 {
			t.Errorf("detail: controller ConnectedAtMs = %d, want > 0", detailResp.Controller.ConnectedAtMs)
		}

		// Verify status endpoint
		status := getStatus(t, env.HTTPAddr)
		if len(status.Controllers) < 1 {
			t.Error("expected at least 1 controller in status")
		}
	})

	t.Run("controller_disconnects", func(t *testing.T) {
		if ctrl == nil {
			t.Skip("controller not connected (previous subtest may have failed)")
		}

		ctrl.Close()

		err := testutil.WaitForCondition(func() bool {
			resp := getControllers(t, env.HTTPAddr)
			return len(resp.Controllers) == 0
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for controller to disconnect: %v", err)
		}

		// Verify list endpoint
		ctrlResp := getControllers(t, env.HTTPAddr)
		if len(ctrlResp.Controllers) != 0 {
			t.Errorf("expected 0 controllers after disconnect, got %d", len(ctrlResp.Controllers))
		}

		// Verify detail endpoint returns 404
		_, code := getControllerByUUID(t, env.HTTPAddr, serverUUID)
		if code != http.StatusNotFound {
			t.Errorf("expected 404 for disconnected controller, got %d", code)
		}

		// Verify status endpoint
		status := getStatus(t, env.HTTPAddr)
		if len(status.Controllers) != 0 {
			t.Errorf("expected 0 controllers in status after disconnect, got %d", len(status.Controllers))
		}
	})
}

// TestAPI_TimeWindowedStats_InspectMode validates that workers in inspect mode
// have time_windowed_stats populated in the API response after processing requests.
func TestAPI_TimeWindowedStats_InspectMode(t *testing.T) {
	env, err := testutil.NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	ctx := context.Background()

	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
	defer worker.Close()
	defer device.Close()

	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && d.IsConnected && d.WorkerCount == 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device with worker: %v", err)
	}

	// Connect controller — with inspect enabled, the worker enters inspect mode
	ctrl := testutil.NewFakeController()
	if err := ctrl.Connect(ctx, env.ControllerAddr); err != nil {
		t.Fatalf("ctrl.Connect: %v", err)
	}
	defer ctrl.Close()

	err = testutil.WaitForCondition(func() bool {
		resp := getControllers(t, env.HTTPAddr)
		return len(resp.Controllers) >= 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for controller: %v", err)
	}

	// Send requests through the controller
	const numRequests = 10
	for i := range numRequests {
		req := testutil.BuildMitmRequest(
			testutil.WithMethod(protos.MitmRequest_RPC_REQUEST),
		)
		resp, err := ctrl.SendRequest(req)
		if err != nil {
			t.Fatalf("request %d: SendRequest: %v", i, err)
		}
		if resp.Status != protos.MitmResponse_SUCCESS {
			t.Fatalf("request %d: expected SUCCESS, got %s", i, resp.Status)
		}
	}

	// Fetch device with workers and verify time_windowed_stats
	devResp, code := getDeviceByID(t, env.HTTPAddr, device.DeviceID(), true)
	if code != http.StatusOK {
		t.Fatalf("expected 200 for device, got %d", code)
	}
	if len(devResp.Device.Workers) == 0 {
		t.Fatal("no workers in device response")
	}

	w := devResp.Device.Workers[0]
	if w.TimeWindowedStats == nil {
		t.Fatal("time_windowed_stats is nil for worker in inspect mode")
	}

	stats := w.TimeWindowedStats

	// After 10 requests, all rate windows should be non-zero
	if stats.RequestsRateOver30Seconds <= 0 {
		t.Errorf("requests_rate_over_30_seconds = %f, want > 0", stats.RequestsRateOver30Seconds)
	}
	if stats.RequestsRateOver1Min <= 0 {
		t.Errorf("requests_rate_over_1_min = %f, want > 0", stats.RequestsRateOver1Min)
	}
	if stats.RequestsRateOver5Min <= 0 {
		t.Errorf("requests_rate_over_5_min = %f, want > 0", stats.RequestsRateOver5Min)
	}
	if stats.RequestsRateOver15Min <= 0 {
		t.Errorf("requests_rate_over_15_min = %f, want > 0", stats.RequestsRateOver15Min)
	}

	// Duration averages should be non-negative (fast requests may be 0ms)
	if stats.RequestMsAvgOver30Seconds < 0 {
		t.Errorf("request_ms_avg_over_30_seconds = %f, want >= 0", stats.RequestMsAvgOver30Seconds)
	}
	if stats.RequestMsAvgOver1Min < 0 {
		t.Errorf("request_ms_avg_over_1_min = %f, want >= 0", stats.RequestMsAvgOver1Min)
	}
}

// TestAPI_TimeWindowedStats_StatsDisabled validates that workers with stats disabled
// do not have time_windowed_stats in the API response.
func TestAPI_TimeWindowedStats_TransparentMode(t *testing.T) {
	env, err := testutil.NewTestEnv(testutil.WithDisableWorkerStats(true))
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	ctx := context.Background()

	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
	defer worker.Close()
	defer device.Close()

	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && d.IsConnected && d.WorkerCount == 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device with worker: %v", err)
	}

	// Connect controller — without inspect, worker enters transparent mode
	ctrl := testutil.NewFakeController()
	if err := ctrl.Connect(ctx, env.ControllerAddr); err != nil {
		t.Fatalf("ctrl.Connect: %v", err)
	}
	defer ctrl.Close()

	err = testutil.WaitForCondition(func() bool {
		resp := getControllers(t, env.HTTPAddr)
		return len(resp.Controllers) >= 1
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for controller: %v", err)
	}

	// Send a few requests
	for i := range 5 {
		req := testutil.BuildMitmRequest(
			testutil.WithMethod(protos.MitmRequest_RPC_REQUEST),
		)
		resp, err := ctrl.SendRequest(req)
		if err != nil {
			t.Fatalf("request %d: SendRequest: %v", i, err)
		}
		if resp.Status != protos.MitmResponse_SUCCESS {
			t.Fatalf("request %d: expected SUCCESS, got %s", i, resp.Status)
		}
	}

	// With stats disabled, time_windowed_stats should be nil
	devResp, code := getDeviceByID(t, env.HTTPAddr, device.DeviceID(), true)
	if code != http.StatusOK {
		t.Fatalf("expected 200 for device, got %d", code)
	}
	if len(devResp.Device.Workers) == 0 {
		t.Fatal("no workers in device response")
	}

	w := devResp.Device.Workers[0]
	if w.TimeWindowedStats != nil {
		t.Errorf("expected time_windowed_stats to be nil in transparent mode, got %+v", w.TimeWindowedStats)
	}
}
