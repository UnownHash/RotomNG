package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/testutil"
)

// dialRawControllerNoLogin creates a raw websocket connection to the controller
// endpoint, sends RegisterControllerRequest, and reads RegisterControllerResponse.
// Unlike dialRawController, it does NOT send LoginRequest and does NOT treat
// non-SUCCESS as fatal -- it returns the response and connection for the caller
// to inspect. Used for SEL-03 to verify NO_WORKERS_AVAILABLE status and close code.
func dialRawControllerNoLogin(t *testing.T, addr string) (*websocket.Conn, *protos.RegisterControllerResponse) {
	t.Helper()
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(context.Background(), "ws://"+addr+"/controller", nil)
	if err != nil {
		t.Fatalf("raw dial to %s/controller: %v", addr, err)
	}

	controllerID := uuid.New().String()

	regReq := &protos.RegisterControllerRequest{
		Id:                controllerID,
		ProtoMajorVersion: 2,
		ProtoMinorVersion: 0,
		Weight:            0,
	}
	data, err := proto.Marshal(regReq)
	if err != nil {
		conn.Close()
		t.Fatalf("marshal RegisterControllerRequest: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		conn.Close()
		t.Fatalf("send RegisterControllerRequest: %v", err)
	}

	_, respData, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		t.Fatalf("read RegisterControllerResponse: %v", err)
	}
	var regResp protos.RegisterControllerResponse
	if err := proto.Unmarshal(respData, &regResp); err != nil {
		conn.Close()
		t.Fatalf("unmarshal RegisterControllerResponse: %v", err)
	}

	return conn, &regResp
}

// TestWorkerSelection validates worker selection behavior: balanced distribution
// across devices (SEL-01), rate limiting enforcement (SEL-02), and proper error
// handling when no workers are available (SEL-03).
func TestWorkerSelection(t *testing.T) {
	t.Run("Distribution", func(t *testing.T) {
		env := startTestEnv(t)
		ctx := context.Background()

		// Connect 2 devices, each with 3 workers (6 workers total).
		device1, worker1a := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device1.Close()
		defer worker1a.Close()

		worker1b := device1.NewWorker()
		if err := worker1b.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker1b.Connect: %v", err)
		}
		defer worker1b.Close()

		worker1c := device1.NewWorker()
		if err := worker1c.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker1c.Connect: %v", err)
		}
		defer worker1c.Close()

		device2, worker2a := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device2.Close()
		defer worker2a.Close()

		worker2b := device2.NewWorker()
		if err := worker2b.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker2b.Connect: %v", err)
		}
		defer worker2b.Close()

		worker2c := device2.NewWorker()
		if err := worker2c.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker2c.Connect: %v", err)
		}
		defer worker2c.Close()

		// Wait for both devices to appear in API with 3 workers each.
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d1 := findDeviceInList(status.Devices, device1.DeviceID())
			d2 := findDeviceInList(status.Devices, device2.DeviceID())
			return d1 != nil && d1.IsConnected && d1.WorkerCount == 3 && d2 != nil && d2.IsConnected && d2.WorkerCount == 3
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for devices with workers in API: %v", err)
		}

		// Connect 4 controllers sequentially (fewer than total workers to ensure
		// both devices can receive assignments).
		const numControllers = 4
		controllers := make([]*testutil.FakeController, numControllers)
		for i := range numControllers {
			c := testutil.NewFakeController()
			if err := c.Connect(ctx, env.ControllerAddr); err != nil {
				t.Fatalf("controller[%d].Connect: %v", i, err)
			}
			defer c.Close()
			controllers[i] = c
		}

		// Wait for controllers to appear in API.
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == numControllers
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for %d controllers in API: %v", numControllers, err)
		}

		// Check distribution: count how many devices have WorkerInUseCount > 0.
		status := getStatus(t, env.HTTPAddr)
		devicesWithAssignments := 0
		for _, d := range status.Devices {
			if d.WorkerInUseCount > 0 {
				devicesWithAssignments++
			}
		}

		if devicesWithAssignments < 2 {
			t.Errorf("controllers distributed across %d devices, want >= 2", devicesWithAssignments)
		}
	})

	t.Run("RateLimiting", func(t *testing.T) {
		// Create TestEnv with rate limiting: 1 selection per 500ms per device.
		env, err := testutil.NewTestEnv(testutil.WithRateLimit(true, 1, 500*time.Millisecond))
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

		// Connect 1 device with 2 workers. Rate limit is per-device (maxSelections=1),
		// so after the first selection the device is rate-limited even though it has
		// another available worker. We need 2 workers so that after the rate limit
		// window expires, a worker is still available for the recovery test.
		device, worker1 := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device.Close()
		defer worker1.Close()

		worker2 := device.NewWorker()
		if err := worker2.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("worker2.Connect: %v", err)
		}
		defer worker2.Close()

		// Wait for device with 2 workers in API.
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 2
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device with workers in API: %v", err)
		}

		// First controller should succeed (consumes the 1 allowed selection).
		controller1 := testutil.NewFakeController()
		if err := controller1.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("controller1.Connect should succeed: %v", err)
		}
		defer controller1.Close()

		// Wait for controller to appear in API.
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for controller1 in API: %v", err)
		}

		// Second controller should fail -- rate limit exhausted on only device
		// (even though there is still an available worker on it).
		controller2 := testutil.NewFakeController()
		err = controller2.Connect(ctx, env.ControllerAddr)
		if err == nil {
			controller2.Close()
			t.Fatal("controller2.Connect should have failed with NO_WORKERS_AVAILABLE")
		}
		if !strings.Contains(err.Error(), "NO_WORKERS_AVAILABLE") {
			t.Fatalf("controller2 error = %q, want to contain NO_WORKERS_AVAILABLE", err)
		}

		// Wait for rate limit window to expire.
		time.Sleep(550 * time.Millisecond)

		// Third controller should succeed after rate limit window expires.
		controller3 := testutil.NewFakeController()
		if err := controller3.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("controller3.Connect should succeed after rate limit expiry: %v", err)
		}
		defer controller3.Close()
	})

	t.Run("NoWorkersAvailable", func(t *testing.T) {
		env := startTestEnv(t)

		// Do NOT connect any devices. Try connecting a controller.
		conn, regResp := dialRawControllerNoLogin(t, env.ControllerAddr)
		defer conn.Close()

		// Verify NO_WORKERS_AVAILABLE status.
		if regResp.Status != protos.RegisterControllerResponse_NO_WORKERS_AVAILABLE {
			t.Fatalf("status = %s, want NO_WORKERS_AVAILABLE", regResp.Status)
		}

		// Read from connection until close frame.
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		var closeErr *websocket.CloseError
		for {
			_, _, readErr := conn.ReadMessage()
			if readErr != nil {
				if !errors.As(readErr, &closeErr) {
					// Connection closed without a clean close frame -- still verify status above passed.
					t.Logf("connection closed without CloseError: %v", readErr)
					return
				}
				break
			}
		}

		if closeErr.Code != 3001 {
			t.Errorf("close code = %d, want 3001 (CLOSE_CODE_NO_MITM_WORKERS_AVAILABLE)", closeErr.Code)
		}
	})
}
