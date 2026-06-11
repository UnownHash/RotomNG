package stats

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatsCollectorIntegration(t *testing.T) {
	// Create a new stats collector
	sc := NewPromStatsCollector("test")

	// Get the metrics handler
	handler := sc.GetMetricsHandler()

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()

	// Serve the metrics
	handler.ServeHTTP(w, req)

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	t.Logf("Metrics output:\n%s", body)

	// Increment some interface methods to test
	sc.IncrDevicesConnected("test")
	sc.IncrDevicesTotal("test")
	sc.IncrControllerAccepts()
	sc.IncrControllerAcceptFails()

	// Create another test request
	req2 := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w2 := httptest.NewRecorder()

	// Serve the metrics again
	handler.ServeHTTP(w2, req2)

	body2 := w2.Body.String()
	t.Logf("Metrics output after increment:\n%s", body2)

	// Check if the counter values are correct
	if !strings.Contains(body2, "test_devices_connected") {
		t.Error("test_devices_connected metric not found after increment")
	}
	if !strings.Contains(body2, "test_devices_total") {
		t.Error("test_devices_total metric not found after increment")
	}
	if !strings.Contains(body2, "test_controller_accepts_total") {
		t.Error("test_controller_accepts_total metric not found after increment")
	}
}

func TestStatsCollectorDevicesTotalGauge(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Test incrementing devices total for different origins
	sc.IncrDevicesTotal("origin1")
	sc.IncrDevicesTotal("origin1")
	sc.IncrDevicesTotal("origin2")

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("Devices total metrics:\n%s", body)

	// Check if devices_total metrics are present with correct values
	if !strings.Contains(body, `test_devices_total{origin="origin1"} 2`) {
		t.Error("test_devices_total metric for origin1 not correct")
	}
	if !strings.Contains(body, `test_devices_total{origin="origin2"} 1`) {
		t.Error("test_devices_total metric for origin2 not correct")
	}

	// Test decrementing with count
	sc.DecrDevicesTotal("origin1", 1)

	req2 := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	body2 := w2.Body.String()
	t.Logf("Devices total metrics after decrement:\n%s", body2)

	if !strings.Contains(body2, `test_devices_total{origin="origin1"} 1`) {
		t.Error("test_devices_total metric for origin1 not decremented correctly")
	}
}

func TestStatsCollectorDevicesConnectedGauge(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Test incrementing devices connected for different origins
	sc.IncrDevicesConnected("origin1")
	sc.IncrDevicesConnected("origin1")
	sc.IncrDevicesConnected("origin2")

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("Devices connected metrics:\n%s", body)

	// Check if devices_connected metrics are present with correct values
	if !strings.Contains(body, `test_devices_connected{origin="origin1"} 2`) {
		t.Error("test_devices_connected metric for origin1 not correct")
	}
	if !strings.Contains(body, `test_devices_connected{origin="origin2"} 1`) {
		t.Error("test_devices_connected metric for origin2 not correct")
	}

	// Test decrementing
	sc.DecrDevicesConnected("origin1")

	req2 := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	body2 := w2.Body.String()
	t.Logf("Devices connected metrics after decrement:\n%s", body2)

	if !strings.Contains(body2, `test_devices_connected{origin="origin1"} 1`) {
		t.Error("test_devices_connected metric for origin1 not decremented correctly")
	}
}

func TestStatsCollectorAllMetricsPresent(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Add some data to interface metrics only
	sc.IncrDevicesTotal("test")
	sc.IncrDevicesConnected("test")
	sc.IncrControllerAccepts()
	sc.IncrControllerAcceptFails()

	// Add some memory metrics
	sc.SetDeviceMemoryFree("test", 1024000)
	sc.SetDeviceMemoryMITM("test", 512000)
	sc.SetDeviceMemoryStart("test", 256000)

	// Add some device command metrics
	sc.IncrDeviceCommandExecuted("test", "getMemoryUsage")
	sc.IncrDeviceCommandSuccess("test", "getMemoryUsage")
	sc.IncrDeviceCommandError("test", "getScreenSize")

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("All metrics output:\n%s", body)

	// Check that interface metric types are present in the output
	expectedMetrics := []string{
		"test_devices_total",
		"test_devices_connected",
		"test_device_memory_free",
		"test_device_memory_mitm",
		"test_device_memory_start",
		"test_device_commands_executed_total",
		"test_device_commands_success_total",
		"test_device_commands_error_total",
		"test_controller_accepts_total",
		"test_controller_accept_fails_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Metric %s not found in output", metric)
		}
	}
}

func TestStatsCollectorDeviceMemoryGauges(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Set memory values for different origins
	sc.SetDeviceMemoryFree("origin1", 1024000)
	sc.SetDeviceMemoryMITM("origin1", 512000)
	sc.SetDeviceMemoryStart("origin1", 256000)

	sc.SetDeviceMemoryFree("origin2", 2048000)
	sc.SetDeviceMemoryMITM("origin2", 1024000)
	sc.SetDeviceMemoryStart("origin2", 512000)

	// Get metrics output
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("Device memory metrics:\n%s", body)

	// Verify memory metrics are present
	expectedMetrics := []string{
		"test_device_memory_free{origin=\"origin1\"} 1.024e+06",
		"test_device_memory_mitm{origin=\"origin1\"} 512000",
		"test_device_memory_start{origin=\"origin1\"} 256000",
		"test_device_memory_free{origin=\"origin2\"} 2.048e+06",
		"test_device_memory_mitm{origin=\"origin2\"} 1.024e+06",
		"test_device_memory_start{origin=\"origin2\"} 512000",
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected metric not found: %s", expected)
		}
	}

	// Verify help text is present
	expectedHelp := []string{
		"# HELP test_device_memory_free Device free memory in bytes",
		"# HELP test_device_memory_mitm Device MITM memory usage in bytes",
		"# HELP test_device_memory_start Device start memory in bytes",
	}

	for _, expected := range expectedHelp {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected help text not found: %s", expected)
		}
	}

	// Test updating values
	sc.SetDeviceMemoryFree("origin1", 2048000)
	sc.SetDeviceMemoryMITM("origin1", 1024000)
	sc.SetDeviceMemoryStart("origin1", 512000)

	req2 := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	body2 := w2.Body.String()
	t.Logf("Device memory metrics after update:\n%s", body2)

	// Verify updated values
	if !strings.Contains(body2, "test_device_memory_free{origin=\"origin1\"} 2.048e+06") {
		t.Error("Device memory free metric not updated correctly")
	}
	if !strings.Contains(body2, "test_device_memory_mitm{origin=\"origin1\"} 1.024e+06") {
		t.Error("Device memory mitm metric not updated correctly")
	}
	if !strings.Contains(body2, "test_device_memory_start{origin=\"origin1\"} 512000") {
		t.Error("Device memory start metric not updated correctly")
	}
}

func TestStatsCollectorDeviceCommandMetrics(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Test device command metrics for different origins and commands
	sc.IncrDeviceCommandExecuted("origin1", "getMemoryUsage")
	sc.IncrDeviceCommandExecuted("origin1", "getScreenSize")
	sc.IncrDeviceCommandExecuted("origin2", "restartApp")

	sc.IncrDeviceCommandSuccess("origin1", "getMemoryUsage")
	sc.IncrDeviceCommandSuccess("origin2", "restartApp")

	sc.IncrDeviceCommandError("origin1", "getScreenSize")

	// Get metrics output
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("Device command metrics:\n%s", body)

	// Verify device command metrics are present
	expectedMetrics := []string{
		"test_device_commands_executed_total",
		"test_device_commands_success_total",
		"test_device_commands_error_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Device command metric %s not found in output", metric)
		}
	}

	// Verify specific metric values
	expectedValues := []string{
		`test_device_commands_executed_total{command="getMemoryUsage",origin="origin1"} 1`,
		`test_device_commands_executed_total{command="getScreenSize",origin="origin1"} 1`,
		`test_device_commands_executed_total{command="restartApp",origin="origin2"} 1`,
		`test_device_commands_success_total{command="getMemoryUsage",origin="origin1"} 1`,
		`test_device_commands_success_total{command="restartApp",origin="origin2"} 1`,
		`test_device_commands_error_total{command="getScreenSize",origin="origin1"} 1`,
	}

	for _, expected := range expectedValues {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected device command metric value not found: %s", expected)
		}
	}

	// Verify help text is present
	expectedHelp := []string{
		"# HELP test_device_commands_executed_total Total number of device commands executed",
		"# HELP test_device_commands_success_total Total number of successful device commands",
		"# HELP test_device_commands_error_total Total number of failed device commands",
	}

	for _, expected := range expectedHelp {
		if !strings.Contains(body, expected) {
			t.Errorf("Expected help text not found: %s", expected)
		}
	}

	// Test incrementing existing metrics
	sc.IncrDeviceCommandExecuted("origin1", "getMemoryUsage")
	sc.IncrDeviceCommandSuccess("origin1", "getMemoryUsage")

	req2 := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	body2 := w2.Body.String()
	t.Logf("Device command metrics after increment:\n%s", body2)

	// Verify incremented values
	if !strings.Contains(body2, `test_device_commands_executed_total{command="getMemoryUsage",origin="origin1"} 2`) {
		t.Error("Device command executed metric not incremented correctly")
	}
	if !strings.Contains(body2, `test_device_commands_success_total{command="getMemoryUsage",origin="origin1"} 2`) {
		t.Error("Device command success metric not incremented correctly")
	}
}

func TestStatsCollectorWorkerMetrics(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Test worker request metrics
	sc.IncrWorkerRequests("POST")
	sc.IncrWorkerRequests("GET")
	sc.IncrWorkerDroppedResponses()
	sc.IncrWorkerResponses(100*time.Millisecond, "POST", "200", "")
	sc.IncrWorkerResponses(50*time.Millisecond, "GET", "404", "not found")

	// Test worker registration metrics
	sc.IncrWorkerRegistrationFails()
	sc.IncrWorkerRegistrations("origin1")
	sc.IncrWorkerRegistrations("origin2")

	// Test worker connection metrics
	sc.IncrWorkersConnected("origin1")
	sc.IncrWorkersInUse("origin1")
	sc.DecrWorkersConnected("origin2")
	sc.DecrWorkersInUse("origin2")

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("Worker metrics:\n%s", body)

	// Verify worker metrics are present
	expectedMetrics := []string{
		"test_worker_requests_total",
		"test_worker_dropped_responses_total",
		"test_worker_responses_total",
		"test_worker_response_duration_seconds",
		"test_worker_registration_fails_total",
		"test_worker_registrations_total",
		"test_workers_connected",
		"test_workers_in_use",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Worker metric %s not found in output", metric)
		}
	}
}

func TestStatsCollectorUncoveredMethods(t *testing.T) {
	sc := NewPromStatsCollector("test")

	// Exercise previously-uncovered counter/gauge methods
	sc.IncrDeviceRegistrationFails()
	sc.IncrDeviceRegistrations("test-origin")
	sc.IncrDeviceControlAccepts()
	sc.IncrDeviceControlAcceptFails()
	sc.IncrWorkerAccepts()
	sc.IncrWorkerAcceptFails()
	sc.IncrAppStartups("1.0.0")
	sc.IncrConfigReloads("1.0.0")

	// Serve metrics and verify the counters appear
	handler := sc.GetMetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	expectedMetrics := []string{
		"test_device_registration_fails_total",
		"test_device_registrations_total",
		"test_device_control_accepts_total",
		"test_device_control_accept_fails_total",
		"test_worker_accepts_total",
		"test_worker_accept_fails_total",
		"test_app_startups_total",
		"test_config_reloads_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Expected metric %s not found in output", metric)
		}
	}
}

func TestStatsCollectorControllerConnectionMetrics(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Test controller connection metrics
	sc.IncrControllerConnections("Mozilla/5.0")
	sc.IncrControllerConnections("Mozilla/5.0")
	sc.IncrControllerConnections("Chrome/91.0")
	sc.DecrControllerConnections("Mozilla/5.0")

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("Controller connection metrics:\n%s", body)

	// Verify controller connection metrics are present
	if !strings.Contains(body, "test_controller_connections") {
		t.Error("Controller connections metric not found")
	}

	// Check specific values
	if !strings.Contains(body, `test_controller_connections{user_agent="Mozilla/5.0"} 1`) {
		t.Error("Controller connections metric for Mozilla/5.0 not correct")
	}
	if !strings.Contains(body, `test_controller_connections{user_agent="Chrome/91.0"} 1`) {
		t.Error("Controller connections metric for Chrome/91.0 not correct")
	}
}

func TestStatsCollectorRpcMetrics(t *testing.T) {
	sc := NewPromStatsCollector("test")
	handler := sc.GetMetricsHandler()

	// Test RPC request metrics
	sc.IncrRPCRequests(100 * time.Millisecond)
	sc.IncrRPCRequests(50 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	t.Logf("RPC metrics:\n%s", body)

	// Verify RPC metrics are present
	expectedMetrics := []string{
		"test_rpc_requests_total",
		"test_rpc_request_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("RPC metric %s not found in output", metric)
		}
	}

	// Verify specific metric values
	if !strings.Contains(body, "test_rpc_requests_total 2") {
		t.Error("Expected test_rpc_requests_total 2 not found")
	}
}
