package app_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/testutil"
)

// TestLifecycle_DeviceRegistration validates LIFE-01: a fake device connects,
// registers a worker, and appears in GET /api/status with worker_count 1.
func TestLifecycle_DeviceRegistration(t *testing.T) {
	env, err := testutil.NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}

	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer env.Stop()

	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	t.Run("device_connects_and_appears_in_api", func(t *testing.T) {
		ctx := context.Background()

		device := testutil.NewFakeDevice()
		if err := device.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("device.Connect: %v", err)
		}
		defer device.Close()

		worker := device.NewWorker()
		if err := worker.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker.Connect: %v", err)
		}
		defer worker.Close()

		// Wait until this specific device appears in the API with worker_count == 1
		var foundDevice *apiDevice
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			foundDevice = findDeviceInList(status.Devices, device.DeviceID())
			return foundDevice != nil && foundDevice.IsConnected && foundDevice.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device to appear in API: %v", err)
		}

		if !foundDevice.IsConnected {
			t.Errorf("device IsConnected = false, want true")
		}
	})

	t.Run("device_disconnect_updates_api", func(t *testing.T) {
		ctx := context.Background()

		device := testutil.NewFakeDevice()
		if err := device.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("device.Connect: %v", err)
		}

		worker := device.NewWorker()
		if err := worker.Connect(ctx, env.DeviceAddr); err != nil {
			device.Close()
			t.Fatalf("worker.Connect: %v", err)
		}

		// Wait until this specific device appears in the API
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			return findDeviceInList(status.Devices, device.DeviceID()) != nil
		}, waitTimeout)
		if err != nil {
			worker.Close()
			device.Close()
			t.Fatalf("waiting for device to appear in API: %v", err)
		}

		// Disconnect worker then device
		worker.Close()
		device.Close()

		// Wait until the device shows as disconnected with no workers
		var foundDevice *apiDevice
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			foundDevice = findDeviceInList(status.Devices, device.DeviceID())
			return foundDevice != nil && !foundDevice.IsConnected && foundDevice.WorkerCount == 0
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device disconnect in API: %v", err)
		}

		if foundDevice.IsConnected {
			t.Errorf("device IsConnected = true after disconnect, want false")
		}
	})
}

// TestLifecycle_DisconnectRemoval validates LIFE-04: after a device disconnects,
// the device is marked as disconnected with no workers in GET /api/status,
// and can be fully removed via the delete API.
func TestLifecycle_DisconnectRemoval(t *testing.T) {
	env, err := testutil.NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}

	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer env.Stop()

	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	ctx := context.Background()

	device := testutil.NewFakeDevice()
	if err := device.Connect(ctx, env.DeviceAddr); err != nil {
		t.Fatalf("device.Connect: %v", err)
	}

	worker := device.NewWorker()
	if err := worker.Connect(ctx, env.DeviceAddr); err != nil {
		device.Close()
		t.Fatalf("worker.Connect: %v", err)
	}

	// Wait until the device appears in the API with worker_count == 1
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && d.IsConnected && d.WorkerCount == 1
	}, waitTimeout)
	if err != nil {
		worker.Close()
		device.Close()
		t.Fatalf("waiting for device to appear in API: %v", err)
	}

	// Disconnect worker then device
	worker.Close()
	device.Close()

	// Wait until the device shows as disconnected
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && !d.IsConnected && d.WorkerCount == 0
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device disconnect in API: %v", err)
	}

	// Delete unconnected devices via the API
	deleteResp, err := http.NewRequest(http.MethodPut,
		fmt.Sprintf("http://%s/api/device/_/action/delete", env.HTTPAddr), nil)
	if err != nil {
		t.Fatalf("creating delete request: %v", err)
	}
	resp, err := testHTTPClient.Do(deleteResp)
	if err != nil {
		t.Fatalf("DELETE unconnected devices: %v", err)
	}
	resp.Body.Close()

	// Verify the device is fully removed
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		return findDeviceInList(status.Devices, device.DeviceID()) == nil
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for device removal from API after delete: %v", err)
	}
}

// TestLifecycle_ControllerProxy validates LIFE-02: a fake controller connects,
// gets assigned to a registered device worker, and a MitmRequest/MitmResponse
// round trip completes through the proxy.
func TestLifecycle_ControllerProxy(t *testing.T) {
	env, err := testutil.NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}

	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer env.Stop()

	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	ctx := context.Background()

	t.Run("proxy_round_trip", func(t *testing.T) {
		// Connect a fake device
		device := testutil.NewFakeDevice()
		if err := device.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("device.Connect: %v", err)
		}
		defer device.Close()

		// Create and connect a worker for the device
		worker := device.NewWorker()
		if err := worker.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker.Connect: %v", err)
		}
		defer worker.Close()

		// Wait until the device appears in the API with WorkerCount == 1
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device with worker in API: %v", err)
		}

		// Connect a fake controller
		ctrl := testutil.NewFakeController()
		if err := ctrl.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("ctrl.Connect: %v", err)
		}
		defer ctrl.Close()

		// Wait until the controller appears in the API status
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			return len(status.Controllers) >= 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for controller to appear in API: %v", err)
		}

		// Build a MitmRequest with a specific ID and send it through the proxy
		req := testutil.BuildMitmRequest(testutil.WithRequestID(42))
		resp, err := ctrl.SendRequest(req)
		if err != nil {
			t.Fatalf("ctrl.SendRequest: %v", err)
		}

		if resp == nil {
			t.Fatal("SendRequest returned nil response")
		}

		if resp.Id != 42 {
			t.Errorf("response ID = %d, want 42", resp.Id)
		}
	})
}

// TestLifecycle_ConcurrentDevices validates LIFE-03: 10 fake devices connect
// concurrently, all register successfully, and all 10 appear in GET /api/status
// simultaneously. No data races detected under -race flag.
func TestLifecycle_ConcurrentDevices(t *testing.T) {
	env, err := testutil.NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}

	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer env.Stop()

	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	ctx := context.Background()

	const deviceCount = 10

	devices := make([]*testutil.FakeDevice, deviceCount)
	workers := make([]*testutil.FakeWorker, deviceCount)
	errs := make([]error, deviceCount)

	var wg sync.WaitGroup

	// Launch all devices concurrently
	for i := range deviceCount {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			device := testutil.NewFakeDevice()
			devices[idx] = device

			if err := device.Connect(ctx, env.DeviceAddr); err != nil {
				errs[idx] = fmt.Errorf("device %d connect: %w", idx, err)
				return
			}

			worker := device.NewWorker()
			workers[idx] = worker

			if err := worker.Connect(ctx, env.DeviceAddr); err != nil {
				errs[idx] = fmt.Errorf("device %d worker connect: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Check for connection errors (outside goroutines — safe to use t.Fatalf)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("device %d: %v", i, err)
		}
	}

	// Defer cleanup: close all workers then devices (nil-safe)
	defer func() {
		for i := range deviceCount {
			if workers[i] != nil {
				workers[i].Close()
			}
			if devices[i] != nil {
				devices[i].Close()
			}
		}
	}()

	// Wait until all devices appear in the API (10s timeout for concurrent registration)
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		return len(status.Devices) == deviceCount
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("waiting for %d devices in API: %v", deviceCount, err)
	}

	// Final assertion: verify each device has WorkerCount >= 1 and IsConnected
	status := getStatus(t, env.HTTPAddr)
	if len(status.Devices) != deviceCount {
		t.Fatalf("expected %d devices, got %d", deviceCount, len(status.Devices))
	}

	for _, d := range status.Devices {
		if !d.IsConnected {
			t.Errorf("device %s: IsConnected = false, want true", d.ID)
		}
		if d.WorkerCount < 1 {
			t.Errorf("device %s: WorkerCount = %d, want >= 1", d.ID, d.WorkerCount)
		}
	}
}
