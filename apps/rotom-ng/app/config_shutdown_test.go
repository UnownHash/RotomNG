package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fasthttp/websocket"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/testutil"
)

// TestMetricsEndpoint validates that GET /api/metrics returns 200 with
// prometheus metrics text when prometheus is enabled.
func TestMetricsEndpoint(t *testing.T) {
	env, err := testutil.NewTestEnv(
		testutil.WithPrometheus(true),
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

	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/metrics", env.HTTPAddr))
	if err != nil {
		t.Fatalf("GET /api/metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/metrics status = %d, want 200", resp.StatusCode)
	}

	// Prometheus output always includes Go runtime metrics with "go_" prefix.
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])
	if len(bodyStr) == 0 {
		t.Fatal("metrics response body is empty")
	}
	if !strings.Contains(bodyStr, "go_") {
		t.Errorf("metrics response does not contain 'go_' prefix; got: %s", bodyStr[:min(200, len(bodyStr))])
	}
}

// TestMetricsWithConnections validates that /api/metrics includes gin HTTP stats
// and app-specific metrics with the correct namespace when devices are connected.
func TestMetricsWithConnections(t *testing.T) {
	const namespace = "test_ns"

	env, err := testutil.NewTestEnv(
		testutil.WithPrometheus(true),
		testutil.WithPrometheusNamespace(namespace),
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

	// Connect a device + worker so connection metrics are populated.
	ctx := context.Background()
	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
	defer device.Close()
	defer worker.Close()

	// Wait for device and worker to appear in the API.
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		d := findDeviceInList(status.Devices, device.DeviceID())
		return d != nil && d.IsConnected && d.WorkerCount > 0
	}, waitTimeout)
	if err != nil {
		t.Fatalf("device/worker not in API: %v", err)
	}

	// Make a few API calls to generate gin metrics.
	for range 3 {
		testHTTPClient.Get(fmt.Sprintf("http://%s/api/status", env.HTTPAddr))
	}

	// Fetch metrics.
	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/metrics", env.HTTPAddr))
	if err != nil {
		t.Fatalf("GET /api/metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/metrics status = %d, want 200", resp.StatusCode)
	}

	body := make([]byte, 64*1024)
	n, _ := resp.Body.Read(body)
	metricsBody := string(body[:n])

	if len(metricsBody) == 0 {
		t.Fatal("metrics response body is empty")
	}

	// Gin HTTP stats should be present (subsystem "gin").
	ginPrefix := namespace + "_gin_"
	if !strings.Contains(metricsBody, ginPrefix) {
		t.Errorf("metrics output missing gin stats with prefix %q", ginPrefix)
	}

	// App-specific metrics should be present.
	appMetrics := []string{
		namespace + "_app_startups_total",
		namespace + "_devices_connected",
		namespace + "_workers_connected",
		namespace + "_device_registrations_total",
		namespace + "_worker_registrations_total",
	}
	for _, metric := range appMetrics {
		if !strings.Contains(metricsBody, metric) {
			t.Errorf("metrics output missing %q", metric)
		}
	}

	// Go runtime and process metrics should be namespaced.
	if !strings.Contains(metricsBody, namespace+"_go_") {
		t.Errorf("go runtime metrics missing namespace prefix %q", namespace+"_go_")
	}
	if !strings.Contains(metricsBody, namespace+"_process_") {
		t.Errorf("process metrics missing namespace prefix %q", namespace+"_process_")
	}

	// Every non-comment, non-empty line with a metric name should use the namespace.
	// This ensures no metric leaks without the configured namespace.
	for line := range strings.SplitSeq(metricsBody, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		if !strings.HasPrefix(line, namespace+"_") {
			t.Errorf("metric line does not start with namespace %q: %s", namespace+"_", truncate(line, 120))
		}
	}
}

// TestMetricsNamespaceChange validates that different namespace configs produce
// different metric prefixes in /api/metrics output.
func TestMetricsNamespaceChange(t *testing.T) {
	namespaces := []string{"alpha_ns", "beta_ns"}

	for _, ns := range namespaces {
		t.Run(ns, func(t *testing.T) {
			env, err := testutil.NewTestEnv(
				testutil.WithPrometheus(true),
				testutil.WithPrometheusNamespace(ns),
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

			resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/metrics", env.HTTPAddr))
			if err != nil {
				t.Fatalf("GET /api/metrics: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /api/metrics status = %d, want 200", resp.StatusCode)
			}

			body := make([]byte, 64*1024)
			n, _ := resp.Body.Read(body)
			metricsBody := string(body[:n])

			// Verify every metric line uses this namespace.
			for line := range strings.SplitSeq(metricsBody, "\n") {
				if line == "" || line[0] == '#' {
					continue
				}
				if !strings.HasPrefix(line, ns+"_") {
					t.Errorf("metric line does not start with namespace %q: %s", ns+"_", truncate(line, 120))
				}
			}

			// Verify the other namespace is NOT present.
			otherNS := namespaces[0]
			if otherNS == ns {
				otherNS = namespaces[1]
			}
			if strings.Contains(metricsBody, otherNS+"_") {
				t.Errorf("metrics output contains wrong namespace %q when configured with %q", otherNS, ns)
			}
		})
	}
}

// truncate returns s truncated to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestConfigAndShutdown validates config reload via SIGHUP (CFG-01),
// graceful shutdown drain timeout (CFG-02), and shutdown close frames (CFG-03).
// All subtests run sequentially (no t.Parallel) to avoid SIGHUP interference.
func TestConfigAndShutdown(t *testing.T) {
	t.Run("CFG-01_SIGHUPReload", func(t *testing.T) {
		newHTTPSecret := "reloaded-http-secret-cfg01"

		// Create reload callback that returns config with new HTTP secret.
		// Must create a FRESH config with SetDefaults() called (via NewTestConfig).
		reloadFn := func() (*config.Config, error) {
			cfg, _, err := testutil.NewTestConfig(
				testutil.WithHTTPSecret(newHTTPSecret),
			)
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

		// Before reload: HTTP API should be accessible without auth (no secret configured).
		resp, err := testHTTPClient.Get("http://" + env.HTTPAddr + "/api/status")
		if err != nil {
			t.Fatalf("pre-reload GET /api/status: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("pre-reload status = %d, want 200", resp.StatusCode)
		}

		// Send SIGHUP to trigger reload.
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatalf("sending SIGHUP: %v", err)
		}

		// Wait for reload to propagate: unauthenticated request should now return 401.
		err = testutil.WaitForCondition(func() bool {
			r, e := testHTTPClient.Get("http://" + env.HTTPAddr + "/api/status")
			if e != nil {
				return false
			}
			r.Body.Close()
			return r.StatusCode == http.StatusUnauthorized
		}, waitTimeout)
		if err != nil {
			t.Fatalf("waiting for auth to take effect after SIGHUP: %v", err)
		}

		// Verify new secret works: authenticated request should return 200.
		req, err := http.NewRequest(http.MethodGet, "http://"+env.HTTPAddr+"/api/status", nil)
		if err != nil {
			t.Fatalf("creating request: %v", err)
		}
		req.Header.Set("X-Rotom-Secret", newHTTPSecret)
		authedResp, err := testHTTPClient.Do(req)
		if err != nil {
			t.Fatalf("authenticated GET /api/status: %v", err)
		}
		authedResp.Body.Close()
		if authedResp.StatusCode != http.StatusOK {
			t.Errorf("authenticated status = %d, want 200", authedResp.StatusCode)
		}
	})

	t.Run("CFG-01_SIGHUPReloadError", func(t *testing.T) {
		// ReloadConfig returns an error -- app should continue with original config.
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

		// Send SIGHUP -- reload should fail but app continues running.
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatalf("sending SIGHUP: %v", err)
		}

		// Give a moment for the reload to process.
		time.Sleep(200 * time.Millisecond)

		// API should still work (app didn't crash).
		resp, err := testHTTPClient.Get("http://" + env.HTTPAddr + "/api/status")
		if err != nil {
			t.Fatalf("GET /api/status after failed reload: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status after failed reload = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("CFG-02_DrainTimeout", func(t *testing.T) {
		shutdownTimeout := 2 * time.Second

		env, err := testutil.NewTestEnv(
			testutil.WithShutdownTimeout(shutdownTimeout),
		)
		if err != nil {
			t.Fatalf("NewTestEnv: %v", err)
		}
		if err := env.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := env.WaitReady(5 * time.Second); err != nil {
			t.Fatalf("WaitReady: %v", err)
		}

		// Connect a device+worker to create active connections.
		ctx := context.Background()
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device.Close()
		defer worker.Close()

		// Wait for device to appear in API.
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected
		}, waitTimeout)
		if err != nil {
			t.Fatalf("device not in API: %v", err)
		}

		// Capture goroutine baseline.
		baselineGoroutines := runtime.NumGoroutine()

		// Measure shutdown duration.
		start := time.Now()
		env.Stop() // Cancel + wait for Run() to complete
		elapsed := time.Since(start)

		// Shutdown must complete within ShutdownTimeout + tolerance.
		tolerance := 1 * time.Second
		if elapsed > shutdownTimeout+tolerance {
			t.Errorf("shutdown took %v, want <= %v (timeout=%v + tolerance=%v)",
				elapsed, shutdownTimeout+tolerance, shutdownTimeout, tolerance)
		}

		// Goroutine leak check: after Stop(), give goroutines a moment to wind down.
		time.Sleep(100 * time.Millisecond)
		currentGoroutines := runtime.NumGoroutine()
		goroutineTolerance := 5
		if currentGoroutines > baselineGoroutines+goroutineTolerance {
			t.Errorf("possible goroutine leak: baseline=%d, current=%d (tolerance=%d)",
				baselineGoroutines, currentGoroutines, goroutineTolerance)
		}
	})

	t.Run("CFG-03_ShutdownCloseFrames", func(t *testing.T) {
		env, err := testutil.NewTestEnv(
			testutil.WithShutdownTimeout(2 * time.Second),
		)
		if err != nil {
			t.Fatalf("NewTestEnv: %v", err)
		}
		if err := env.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := env.WaitReady(5 * time.Second); err != nil {
			t.Fatalf("WaitReady: %v", err)
		}

		// Connect raw device for close frame capture.
		rawDevice := dialRawDevice(t, env.DeviceAddr)
		sendRawDeviceInit(t, rawDevice, "shutdown-close-test-device")
		defer rawDevice.Close()

		// Connect a real device+worker so controller can be assigned.
		ctx := context.Background()
		device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr)
		defer device.Close()
		defer worker.Close()

		// Wait for both devices to appear and the real device+worker to be ready.
		err = testutil.WaitForCondition(func() bool {
			status := getStatus(t, env.HTTPAddr)
			if len(status.Devices) < 2 {
				return false
			}
			d := findDeviceInList(status.Devices, device.DeviceID())
			return d != nil && d.IsConnected && d.WorkerCount == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("devices not in API: %v", err)
		}

		// Connect raw controller for close frame capture.
		rawCtrl := dialRawController(t, env.ControllerAddr)
		defer rawCtrl.Close()

		// Wait for controller to appear in API.
		err = testutil.WaitForCondition(func() bool {
			ctrls := getControllers(t, env.HTTPAddr)
			return len(ctrls.Controllers) == 1
		}, waitTimeout)
		if err != nil {
			t.Fatalf("controller not in API: %v", err)
		}

		// Set read deadlines so the read loops don't hang forever if no close
		// frame arrives (the server may tear down TCP before sending a frame).
		readDeadline := time.Now().Add(5 * time.Second)
		rawCtrl.SetReadDeadline(readDeadline)
		rawDevice.SetReadDeadline(readDeadline)

		// Trigger graceful shutdown.
		env.App.Cancel()

		// Read close frame from controller.
		// Controller handler sends StatusGoingAway (1001) on shutdown.
		rawCtrl.SetReadDeadline(time.Now().Add(10 * time.Second))
		var ctrlCloseErr *websocket.CloseError
		for {
			_, _, readErr := rawCtrl.ReadMessage()
			if readErr != nil {
				errors.As(readErr, &ctrlCloseErr)
				break
			}
		}
		if ctrlCloseErr != nil {
			// During shutdown, controller may receive GoingAway (1001) from the
			// HTTP server or MitmWorkerDisconnected (3000) if the device/worker
			// tears down first. Both are valid shutdown close codes.
			if ctrlCloseErr.Code != websocket.CloseGoingAway &&
				ctrlCloseErr.Code != 3000 {
				t.Errorf("controller close code = %d, want 1001 (GoingAway) or 3000 (MitmWorkerDisconnected)",
					ctrlCloseErr.Code)
			}
		} else {
			t.Log("controller connection closed without clean close frame (acceptable)")
		}

		// Read close frame from device.
		// Per research: accept either CloseError or raw connection error.
		rawDevice.SetReadDeadline(time.Now().Add(10 * time.Second))
		var deviceCloseErr *websocket.CloseError
		for {
			_, _, readErr := rawDevice.ReadMessage()
			if readErr != nil {
				errors.As(readErr, &deviceCloseErr)
				break
			}
		}
		if deviceCloseErr != nil {
			// Accept NormalClosure (1000) or GoingAway (1001).
			if deviceCloseErr.Code != websocket.CloseNormalClosure &&
				deviceCloseErr.Code != websocket.CloseGoingAway {
				t.Errorf("device close code = %d, want 1000 or 1001",
					deviceCloseErr.Code)
			}
		} else {
			t.Log("device connection closed without clean close frame (acceptable during shutdown)")
		}

		// Wait for Stop to complete (drain must finish).
		env.Stop()
	})
}
