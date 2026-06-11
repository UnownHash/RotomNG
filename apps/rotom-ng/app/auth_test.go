package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/testutil"
)

const (
	deviceTestSecret     = "device-test-secret"
	controllerTestSecret = "controller-test-secret"
	httpTestSecret       = "http-test-secret"
)

// startTestEnvWithAuth creates a TestEnv with distinct per-listener secrets,
// starts it, waits for readiness, and registers cleanup.
func startTestEnvWithAuth(t *testing.T) *testutil.TestEnv {
	t.Helper()
	env, err := testutil.NewTestEnv(
		testutil.WithDeviceSecret(deviceTestSecret),
		testutil.WithControllerSecret(controllerTestSecret),
		testutil.WithHTTPSecret(httpTestSecret),
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
	return env
}

// makeAuthRequest sends an HTTP GET to the given URL with an optional
// X-Rotom-Secret header. If secret is non-empty, the header is set.
// Returns the *http.Response; the caller must close the body.
func makeAuthRequest(t *testing.T, url string, secret string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if secret != "" {
		req.Header.Set("X-Rotom-Secret", secret)
	}
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP GET %s: %v", url, err)
	}
	return resp
}

// TestAuth_Device verifies that the device listener enforces shared secret
// authentication: valid secrets connect, invalid and missing secrets are rejected.
func TestAuth_Device(t *testing.T) {
	env := startTestEnvWithAuth(t)

	t.Run("valid_secret", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		device := testutil.NewFakeDevice(testutil.WithDeviceAuthSecret(deviceTestSecret))
		if err := device.Connect(ctx, env.DeviceAddr); err != nil {
			t.Fatalf("expected successful connection with valid secret, got: %v", err)
		}
		defer device.Close()
	})

	t.Run("invalid_secret", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		device := testutil.NewFakeDevice(testutil.WithDeviceAuthSecret("wrong-secret"))
		err := device.Connect(ctx, env.DeviceAddr)
		if err == nil {
			device.Close()
			t.Fatal("expected connection to be rejected with invalid secret, but it succeeded")
		}
	})

	t.Run("missing_secret", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		device := testutil.NewFakeDevice()
		err := device.Connect(ctx, env.DeviceAddr)
		if err == nil {
			device.Close()
			t.Fatal("expected connection to be rejected with missing secret, but it succeeded")
		}
	})
}

// TestAuth_HTTPApi verifies that the HTTP API enforces shared secret
// authentication on /api endpoints.
func TestAuth_HTTPApi(t *testing.T) {
	env := startTestEnvWithAuth(t)

	t.Run("valid_secret", func(t *testing.T) {
		resp := makeAuthRequest(t, fmt.Sprintf("http://%s/api/status", env.HTTPAddr), httpTestSecret)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid_secret", func(t *testing.T) {
		resp := makeAuthRequest(t, fmt.Sprintf("http://%s/api/status", env.HTTPAddr), "wrong-secret")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
		// Verify that the response body does not contain API data (middleware aborted before handler).
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		if strings.Contains(string(body), "devices") {
			t.Fatal("response body should not contain API data when auth fails")
		}
	})

	t.Run("missing_secret", func(t *testing.T) {
		resp := makeAuthRequest(t, fmt.Sprintf("http://%s/api/status", env.HTTPAddr), "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", resp.StatusCode)
		}
	})
}

// TestAuth_Controller verifies that the controller listener enforces shared
// secret authentication: valid secrets connect, invalid and missing secrets
// are rejected at the HTTP level (401) before WebSocket upgrade.
func TestAuth_Controller(t *testing.T) {
	env := startTestEnvWithAuth(t)

	t.Run("valid_secret", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Controller V2 protocol requires a device+worker to be available for assignment.
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr, testutil.WithDeviceAuthSecret(deviceTestSecret))
		defer device.Close()
		defer worker.Close()

		// Wait for device+worker to be registered before connecting controller.
		err := testutil.WaitForCondition(func() bool {
			resp := makeAuthRequest(t, fmt.Sprintf("http://%s/api/status", env.HTTPAddr), httpTestSecret)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false
			}
			var result statusAPIResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return false
			}
			d := findDeviceInList(result.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device+worker not registered: %v", err)
		}

		controller := testutil.NewFakeController(testutil.WithAuthSecret(controllerTestSecret))
		if err := controller.Connect(ctx, env.ControllerAddr); err != nil {
			t.Fatalf("expected successful connection with valid secret, got: %v", err)
		}
		defer controller.Close()
	})

	t.Run("invalid_secret", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		controller := testutil.NewFakeController(testutil.WithAuthSecret("wrong-secret"))
		err := controller.Connect(ctx, env.ControllerAddr)
		if err == nil {
			controller.Close()
			t.Fatal("expected connection to be rejected with invalid secret, but it succeeded")
		}
	})

	t.Run("missing_secret", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		controller := testutil.NewFakeController()
		err := controller.Connect(ctx, env.ControllerAddr)
		if err == nil {
			controller.Close()
			t.Fatal("expected connection to be rejected with missing secret, but it succeeded")
		}
	})
}
