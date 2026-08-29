package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/testutil"
)

// dialRawDevice creates a raw websocket connection to the device control
// endpoint, bypassing testutil fakes for close code verification.
func dialRawDevice(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(context.Background(), "ws://"+addr+"/control", nil)
	if err != nil {
		t.Fatalf("raw dial to %s: %v", addr, err)
	}
	return conn
}

// sendRawDeviceInit sends a valid DeviceControlInitMessage as JSON on a raw
// websocket connection to complete the init handshake.
func sendRawDeviceInit(t *testing.T, conn *websocket.Conn, deviceID string) {
	t.Helper()
	initMsg := map[string]string{
		"deviceId": deviceID,
		"version":  "1.0",
		"origin":   "test-origin",
		"publicIp": "127.0.0.1",
	}
	data, err := json.Marshal(initMsg)
	if err != nil {
		t.Fatalf("marshal init: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("send init: %v", err)
	}
}

// dialRawController creates a raw websocket connection to the controller endpoint
// and completes the V2 registration+login handshake using protobuf, returning the
// raw *websocket.Conn so callers can read close frames directly.
func dialRawController(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(context.Background(), "ws://"+addr+"/controller", nil)
	if err != nil {
		t.Fatalf("raw dial to %s/controller: %v", addr, err)
	}

	controllerID := uuid.New().String()

	// Step 1: Send RegisterControllerRequest
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

	// Step 2: Read RegisterControllerResponse
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
	if regResp.Status != protos.RegisterControllerResponse_SUCCESS {
		conn.Close()
		t.Fatalf("registration failed: %s (%s)", regResp.Status, regResp.StatusReason)
	}

	// Step 3: Send LoginRequest
	loginReq := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: controllerID,
				Username: "test-user",
				//nolint:staticcheck
				Source: protos.MitmRequest_LoginRequest_PTC,
			},
		},
	}
	data, err = proto.Marshal(loginReq)
	if err != nil {
		conn.Close()
		t.Fatalf("marshal LoginRequest: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		conn.Close()
		t.Fatalf("send LoginRequest: %v", err)
	}

	return conn
}

// TestErrorHandling validates error conditions: duplicate device UUID replacement
// (ERR-01), device disconnect cleanup (ERR-02), controller disconnect (ERR-03),
// malformed messages (ERR-04), and WebSocket close codes (ERR-05).
func TestErrorHandling(t *testing.T) {
	t.Run("DuplicateUUID", func(t *testing.T) {
		env := startTestEnv(t)
		sharedUUID := "dup-test-device-001"

		// Connect device A using raw websocket for close code verification.
		connA := dialRawDevice(t, env.DeviceAddr)
		defer connA.Close()

		sendRawDeviceInit(t, connA, sharedUUID)

		// Wait for device A to appear in the API.
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, sharedUUID)
			return d != nil && d.IsConnected
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device A in API: %v", err)
		}

		// Connect device B with the same UUID -- this triggers SwapConn replacement.
		ctx := context.Background()
		deviceB := testutil.NewFakeDevice(testutil.WithDeviceID(sharedUUID))
		if err := deviceB.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("deviceB.Connect: %v", err)
		}
		defer deviceB.Close()

		// Wait for device B to be registered (the device entry stays under the same UUID).
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, sharedUUID)
			return d != nil && d.IsConnected
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device B in API: %v", err)
		}

		// Read from connA until we get the close frame (server may send data before closing).
		connA.SetReadDeadline(time.Now().Add(30 * time.Second))
		var closeErr *websocket.CloseError
		for {
			_, _, readErr := connA.ReadMessage()
			if readErr != nil {
				if !errors.As(readErr, &closeErr) {
					t.Fatalf("expected websocket.CloseError, got %T: %v", readErr, readErr)
				}
				break
			}
		}
		if closeErr.Code != websocket.CloseGoingAway {
			t.Errorf("close code = %d, want %d (GoingAway)", closeErr.Code, websocket.CloseGoingAway)
		}

		// Verify device B is still visible in the API as connected.
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, sharedUUID)
		if d == nil {
			t.Fatal("device not found in API after replacement")
		}
		if !d.IsConnected {
			t.Error("device IsConnected = false after replacement, want true")
		}
	})

	t.Run("DeviceDisconnect", func(t *testing.T) {
		env := startTestEnv(t)
		ctx := context.Background()

		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)

		// Wait for device to appear in API with 1 worker.
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device with worker in API: %v", err)
		}

		// Capture goroutine baseline after env is stable.
		baselineGoroutines := runtime.NumGoroutine()

		// Forcibly close worker then device.
		worker.Close()
		device.Close()

		// Wait for device to show as disconnected in API.
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && !d.IsConnected && d.WorkerCount == 0
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for device disconnect in API: %v", err)
		}

		// Verify no goroutine leak: count should be within tolerance of 5.
		err = testutil.WaitForCondition(func() bool {
			current := runtime.NumGoroutine()
			return current <= baselineGoroutines+5
		}, 2*time.Second)
		if err != nil {
			t.Errorf("goroutine leak detected: baseline=%d, current=%d (tolerance=5)",
				baselineGoroutines, runtime.NumGoroutine())
		}
	})

	t.Run("ControllerDisconnect", func(t *testing.T) {
		env := startTestEnv(t)
		ctx := context.Background()

		// Set up device with worker so controller can be assigned.
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device.Close()
		defer worker.Close()

		// Wait for device+worker to appear in API.
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device not in API: %v", err)
		}

		// Connect controller (gets assigned to the worker).
		controller := testutil.NewFakeController()
		if err := controller.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("controller.Connect: %v", err)
		}

		// Wait for controller to appear in API.
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("controller not in API: %v", err)
		}

		// Capture goroutine baseline.
		goroutinesBefore := runtime.NumGoroutine()

		// Forcibly close the controller connection.
		controller.Close()

		// Verify controller disappears from API.
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == 0
		}, waitTimeout)
		if err != nil {
			t.Fatalf("controller should disappear from API: %v", err)
		}

		// Verify device still exists and is connected after controller disconnect.
		// Note: the worker may also be deregistered as part of the controller disconnect
		// cascade (controller close triggers worker close in the proxy path), so we only
		// check that the device itself remains connected and no workers are in use.
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerInUseCount == 0
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device should remain connected after controller disconnect: %v", err)
		}

		// Goroutine leak check (tolerance 5).
		tolerance := 5
		err = testutil.WaitForCondition(func() bool {
			return runtime.NumGoroutine() <= goroutinesBefore+tolerance
		}, 2*time.Second)
		if err != nil {
			goroutinesAfter := runtime.NumGoroutine()
			t.Errorf("possible goroutine leak: before=%d after=%d (tolerance=%d)",
				goroutinesBefore, goroutinesAfter, tolerance)
		}
	})

	t.Run("MalformedMessage", func(t *testing.T) {
		env := startTestEnv(t)
		deviceID := "malformed-test-device"

		// Establish raw websocket connection to device control.
		conn := dialRawDevice(t, env.DeviceAddr)
		defer conn.Close()

		// Send valid init message to enter the message processing loop.
		sendRawDeviceInit(t, conn, deviceID)

		// Wait for device to appear in API (confirms init succeeded).
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			return findDeviceInList(status.Devices, deviceID) != nil
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device did not register after init: %v", err)
		}

		// Send malformed (non-JSON) message -- triggers JSON decode error in
		// processWebsocketMessage (deviceconn.go), causing Run() to return and
		// the deferred deviceConn.Close(StatusNormalClosure) in device_handler.go.
		if err := conn.WriteMessage(websocket.TextMessage, []byte("this is not valid json{{{{")); err != nil {
			t.Fatalf("send malformed: %v", err)
		}

		// Read from the connection in a loop until we get a close frame.
		// The server may send data messages before closing.
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		var closeErr *websocket.CloseError
		for {
			_, _, readErr := conn.ReadMessage()
			if readErr != nil {
				if !errors.As(readErr, &closeErr) {
					// Connection closed without a clean close frame.
					t.Logf("connection closed without CloseError (got: %v); verifying device removed from API", readErr)
				}
				break
			}
		}

		if closeErr != nil {
			// The device handler closes with StatusNormalClosure after Run() returns.
			// Accept either NormalClosure (1000) or InternalServerError (1011).
			if closeErr.Code != websocket.CloseNormalClosure && closeErr.Code != 1011 {
				t.Errorf("close code = %d, want 1000 or 1011", closeErr.Code)
			}
		}

		// Verify device is removed from API after malformed message causes disconnect.
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, deviceID)
			return d == nil || !d.IsConnected
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device should be removed from API after malformed message: %v", err)
		}
	})

	t.Run("WorkerDisconnectClosesController", func(t *testing.T) {
		env := startTestEnv(t)
		ctx := context.Background()

		// Set up device with worker.
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device.Close()

		// Wait for device+worker to appear in API.
		err := testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device+worker not in API: %v", err)
		}

		// Connect a raw controller so we can read the close frame.
		rawCtrlConn := dialRawController(t, env.ControllerAddr)
		defer rawCtrlConn.Close()

		// Wait for controller to appear in API (confirms assignment to worker).
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("controller not in API: %v", err)
		}

		// Forcibly close the worker connection.
		// This triggers manager.go worker deregistration which closes the assigned
		// controller with CLOSE_CODE_MITM_WORKER_DISCONNECTED (3000).
		worker.Close()

		// Read from the raw controller connection in a loop until we get a close frame.
		// The server may send data messages (e.g., login response) before closing.
		rawCtrlConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		var closeErr *websocket.CloseError
		for {
			_, _, readErr := rawCtrlConn.ReadMessage()
			if readErr != nil {
				if !errors.As(readErr, &closeErr) {
					t.Fatalf("expected websocket.CloseError, got: %T: %v", readErr, readErr)
				}
				break
			}
		}

		expectedCode := 3000 // CLOSE_CODE_MITM_WORKER_DISCONNECTED
		if closeErr.Code != expectedCode {
			t.Errorf("controller close code = %d, want %d (CLOSE_CODE_MITM_WORKER_DISCONNECTED)", closeErr.Code, expectedCode)
		}

		// Verify controller disappears from API.
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == 0
		}, waitTimeout)
		if err != nil {
			t.Fatalf("controller should disappear from API after worker disconnect: %v", err)
		}
	})
}
