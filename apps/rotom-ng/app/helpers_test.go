package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/testutil"
)

// waitTimeout is the standard timeout for WaitForCondition in integration tests.
// Set generously (10s) to avoid flakes under -race -count 50 where the race
// detector and high goroutine counts cause scheduling delays. Normal runs
// satisfy conditions in <100ms; this timeout is only a safety net.
const waitTimeout = 10 * time.Second

// testHTTPClient is a shared HTTP client for test helpers. Keep-alives are
// disabled and idle connections set to zero to prevent leaked transport
// goroutines when servers are torn down between test iterations.
var testHTTPClient = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives:   true,
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: 0,
	},
}

// --- Response structs mirroring all JSON fields from libs/api/api.go ---
// These are independent test structs (not imported from libs/api) to catch
// field drift between the API and test expectations (per D-08).

type apiCommonStats struct {
	MessageLastReceivedAtMs int64 `json:"message_last_received_at_ms"`
	MessagesReceived        int64 `json:"messages_received"`
	BytesReceived           int64 `json:"bytes_received"`
	MessageLastSentAtMs     int64 `json:"message_last_sent_at_ms"`
	MessagesSent            int64 `json:"messages_sent"`
	BytesSent               int64 `json:"bytes_sent"`
}

type apiDeviceMemory struct {
	Free  int64 `json:"free"`
	Mitm  int64 `json:"mitm"`
	Start int64 `json:"start"`
}

type apiDeviceSession struct {
	apiCommonStats

	ConnectedAtMs int64 `json:"connected_at_ms"`
}

type apiDevice struct {
	apiCommonStats

	ID                       string            `json:"id"`
	Origin                   string            `json:"origin"`
	Version                  string            `json:"version"`
	PublicIP                 string            `json:"public_ip"`
	WorkerCount              int               `json:"worker_count"`
	WorkerInUseCount         int               `json:"worker_in_use_count"`
	WorkerInUsePercent       float64           `json:"worker_in_use_percent"`
	WorkerInUseWeight        int               `json:"worker_in_use_weight"`
	WorkerInUseWeightPercent float64           `json:"worker_in_use_weight_percent"`
	WorkerMaxWeight          int               `json:"worker_max_weight"`
	LastConnectedAtMs        int64             `json:"last_connected_at_ms"`
	LastSeenAtMs             int64             `json:"last_seen_at_ms"`
	Enabled                  bool              `json:"enabled"`
	IsConnected              bool              `json:"is_connected"`
	CanBeUsed                bool              `json:"can_be_used"`
	LastMemory               *apiDeviceMemory  `json:"last_memory,omitempty"`
	Session                  *apiDeviceSession `json:"session,omitempty"`
	Workers                  []apiWorker       `json:"workers,omitempty"`
	IsInUse                  bool              `json:"is_in_use"`
}

type apiTimeWindowedStats struct {
	RequestsRateOver30Seconds float64 `json:"requests_rate_over_30_seconds"`
	RequestsRateOver1Min      float64 `json:"requests_rate_over_1_min"`
	RequestsRateOver5Min      float64 `json:"requests_rate_over_5_min"`
	RequestsRateOver15Min     float64 `json:"requests_rate_over_15_min"`
	RequestMsAvgOver30Seconds float64 `json:"request_ms_avg_over_30_seconds"`
	RequestMsAvgOver1Min      float64 `json:"request_ms_avg_over_1_min"`
	RequestMsAvgOver5Min      float64 `json:"request_ms_avg_over_5_min"`
	RequestMsAvgOver15Min     float64 `json:"request_ms_avg_over_15_min"`
}

type apiWorkerSession struct {
	apiCommonStats

	ConnectedAtMs int64          `json:"connected_at_ms"`
	Controller    *apiController `json:"controller,omitempty"`
}

type apiWorker struct {
	apiCommonStats

	ID                string                `json:"id"`
	DeviceID          string                `json:"device_id"`
	Origin            string                `json:"origin"`
	VersionCode       int32                 `json:"version_code"`
	VersionName       string                `json:"version_name"`
	UserAgent         string                `json:"user_agent"`
	LastConnectedAtMs int64                 `json:"last_connected_at_ms"`
	LastSeenAtMs      int64                 `json:"last_seen_at_ms"`
	IsConnected       bool                  `json:"is_connected"`
	IsInUse           bool                  `json:"is_in_use"`
	Platform          string                `json:"platform"`
	Weight            *int                  `json:"weight,omitempty"`
	CanBeUsed         bool                  `json:"can_be_used"`
	Session           apiWorkerSession      `json:"session"`
	TimeWindowedStats *apiTimeWindowedStats `json:"time_windowed_stats,omitempty"`
}

type apiController struct {
	apiCommonStats

	ID                string  `json:"id"`
	UUID              string  `json:"uuid"`
	UserAgent         string  `json:"user_agent"`
	Weight            int     `json:"weight"`
	ProtoMajorVersion int     `json:"proto_major_version"`
	ProtoMinorVersion int     `json:"proto_minor_version"`
	WorkerID          *string `json:"worker_id,omitempty"`
	AccountUsername   string  `json:"account_username"`
	AccountSource     string  `json:"account_source"`
	ConnectedAtMs     int64   `json:"connected_at_ms"`
}

// --- Wrapper structs for each endpoint's response shape ---

type statusAPIResponse struct {
	Devices     []apiDevice     `json:"devices"`
	Controllers []apiController `json:"controllers"`
}

type devicesAPIResponse struct {
	Devices []apiDevice `json:"devices"`
}

type deviceAPIResponse struct {
	Device apiDevice `json:"device"`
}

type controllersAPIResponse struct {
	Controllers []apiController `json:"controllers"`
}

type controllerAPIResponse struct {
	Controller apiController `json:"controller"`
}

// --- Shared test environment helper ---

// startTestEnv creates, starts, and waits for a TestEnv to be ready.
// It registers cleanup to stop the env when the test finishes.
func startTestEnv(t *testing.T) *testutil.TestEnv {
	t.Helper()
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
	return env
}

// --- Per-endpoint GET helpers ---

// getStatus calls GET /api/status and returns the parsed response.
func getStatus(t *testing.T, httpAddr string) statusAPIResponse {
	t.Helper()
	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/status", httpAddr))
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/status: status %d", resp.StatusCode)
	}
	var result statusAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode /api/status: %v", err)
	}
	return result
}

// getDevices calls GET /api/device and returns the parsed response.
// If includeWorkers is true, appends ?include_workers=true.
func getDevices(t *testing.T, httpAddr string, includeWorkers bool) devicesAPIResponse {
	t.Helper()
	url := fmt.Sprintf("http://%s/api/device", httpAddr)
	if includeWorkers {
		url += "?include_workers=true"
	}
	resp, err := testHTTPClient.Get(url)
	if err != nil {
		t.Fatalf("GET /api/device: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/device: status %d", resp.StatusCode)
	}
	var result devicesAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode /api/device: %v", err)
	}
	return result
}

// getDeviceByID calls GET /api/device/:deviceId and returns the parsed
// response along with the HTTP status code (to support 404 testing).
func getDeviceByID(t *testing.T, httpAddr, deviceID string, includeWorkers bool) (deviceAPIResponse, int) {
	t.Helper()
	url := fmt.Sprintf("http://%s/api/device/%s", httpAddr, deviceID)
	if includeWorkers {
		url += "?include_workers=true"
	}
	resp, err := testHTTPClient.Get(url)
	if err != nil {
		t.Fatalf("GET /api/device/%s: %v", deviceID, err)
	}
	defer resp.Body.Close()
	var result deviceAPIResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode /api/device/%s: %v", deviceID, err)
		}
	}
	return result, resp.StatusCode
}

// getControllers calls GET /api/controller and returns the parsed response.
func getControllers(t *testing.T, httpAddr string) controllersAPIResponse {
	t.Helper()
	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/controller", httpAddr))
	if err != nil {
		t.Fatalf("GET /api/controller: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/controller: status %d", resp.StatusCode)
	}
	var result controllersAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode /api/controller: %v", err)
	}
	return result
}

// getControllerByUUID calls GET /api/controller/:uuid and returns the parsed
// response along with the HTTP status code (to support 404 testing).
func getControllerByUUID(t *testing.T, httpAddr, uuid string) (controllerAPIResponse, int) {
	t.Helper()
	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/controller/%s", httpAddr, uuid))
	if err != nil {
		t.Fatalf("GET /api/controller/%s: %v", uuid, err)
	}
	defer resp.Body.Close()
	var result controllerAPIResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("decode /api/controller/%s: %v", uuid, err)
		}
	}
	return result, resp.StatusCode
}

// --- Job instance response structs ---

type apiJobInstance struct {
	ID           uint64 `json:"id,omitzero"`
	JobID        string `json:"job_id"`
	DeviceID     string `json:"device_id"`
	StartedAtMs  int64  `json:"started_at_ms"`
	FinishedAtMs int64  `json:"finished_at_ms,omitzero"`
	Status       string `json:"status"`
	Result       string `json:"result,omitempty"`
}

type jobInstancesResponse struct {
	Instances []apiJobInstance `json:"instances"`
}

// getJobInstances calls GET /api/job-instance and returns the parsed response.
func getJobInstances(t *testing.T, httpAddr string) jobInstancesResponse {
	t.Helper()
	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/job-instance", httpAddr))
	if err != nil {
		t.Fatalf("GET /api/job-instance: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/job-instance: status %d", resp.StatusCode)
	}
	var result jobInstancesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode /api/job-instance: %v", err)
	}
	return result
}

// --- Utility helpers ---

// findDeviceInList returns the apiDevice matching the given deviceID, or nil if not found.
func findDeviceInList(devices []apiDevice, deviceID string) *apiDevice {
	for i := range devices {
		if devices[i].ID == deviceID {
			return &devices[i]
		}
	}
	return nil
}

// connectDeviceWithWorker creates a FakeDevice and FakeWorker, connects both,
// and returns them. It fatals on connection errors.
func connectDeviceWithWorker(ctx context.Context, t *testing.T, deviceAddr string, opts ...testutil.DeviceOption) (*testutil.FakeDevice, *testutil.FakeWorker) {
	t.Helper()
	device := testutil.NewFakeDevice(opts...)
	if err := device.Connect(ctx, deviceAddr); err != nil {
		t.Fatalf("device.Connect: %v", err)
	}
	worker := device.NewWorker()
	if err := worker.Connect(ctx, deviceAddr); err != nil {
		device.Close()
		t.Fatalf("worker.Connect: %v", err)
	}
	return device, worker
}
