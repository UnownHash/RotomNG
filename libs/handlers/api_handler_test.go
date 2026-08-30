package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/api"
	"github.com/UnownHash/RotomNG/libs/connections"
	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/selector"
)

type testAPIHandler = APIHandler[*fakeController, *fakeWorker]

// apiTestEnv is an APIHandler wired to a real ConnectionManager, selector, and
// jobs manager, served through a real gin router on the routes the app
// registers.
type apiTestEnv struct {
	handler *testAPIHandler
	manager *connections.ConnectionManager[*fakeController, *fakeWorker]
	jobs    *jobs.Manager
	router  *gin.Engine
	cfg     *APIHandlerConfig[*fakeController, *fakeWorker]
}

type apiEnvOption func(*apiEnvOptions)

type apiEnvOptions struct {
	settings APIHandlerSettings
	// jobFiles are written into the jobs directory before the manager loads.
	jobFiles map[string]string
	// omitJobsManager leaves the handler with no jobs manager at all, which is
	// how the app wires it when jobs are off.
	omitJobsManager bool
}

func withSettings(s APIHandlerSettings) apiEnvOption {
	return func(o *apiEnvOptions) { o.settings = s }
}

func withJobFile(name, body string) apiEnvOption {
	return func(o *apiEnvOptions) {
		if o.jobFiles == nil {
			o.jobFiles = map[string]string{}
		}
		o.jobFiles[name] = body
	}
}

func withoutJobsManager() apiEnvOption {
	return func(o *apiEnvOptions) { o.omitJobsManager = true }
}

func newAPITestEnv(t *testing.T, opts ...apiEnvOption) *apiTestEnv {
	t.Helper()

	options := apiEnvOptions{settings: APIHandlerSettings{JobsEnabled: true}}
	for _, opt := range opts {
		opt(&options)
	}

	logger := slog.New(slog.DiscardHandler)

	jobsPath := t.TempDir()
	for name, body := range options.jobFiles {
		if err := os.WriteFile(filepath.Join(jobsPath, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write job file %s: %v", name, err)
		}
	}

	jobsManagerConfig := jobs.ManagerConfig{Logger: logger}
	if err := jobsManagerConfig.Init(jobs.ManagerSettings{JobsPath: jobsPath}); err != nil {
		t.Fatalf("init jobs manager config: %v", err)
	}
	jobsManager := jobs.NewManager(jobsManagerConfig)
	if err := jobsManager.Reload(); err != nil {
		t.Fatalf("load jobs: %v", err)
	}

	var selectorConfig selector.Config
	if err := selectorConfig.Init(selector.Settings{}); err != nil {
		t.Fatalf("init selector config: %v", err)
	}

	managerConfig := connections.ConnectionManagerConfig[*fakeController, *fakeWorker]{
		Logger:         logger,
		StatsCollector: noopConnStats{},
		JobsRunner:     jobsManager,
		WorkerSelector: selector.NewBalancedSelector[*fakeWorker](selectorConfig),
		NewController: func(
			_ connections.ControllerWSConn,
			id string,
			_ *protos.MitmRequest,
			_ connections.MITMWorker,
			weight int,
			_ string,
			_ bool,
			_, _ int,
		) *fakeController {
			return newFakeController(id, id+"-worker", weight)
		},
		UserAgent: "test",
	}
	if err := managerConfig.Init(connections.ConnectionManagerSettings{}); err != nil {
		t.Fatalf("init connection manager config: %v", err)
	}
	manager := connections.NewConnectionManager(managerConfig)

	handlerConfig := &APIHandlerConfig[*fakeController, *fakeWorker]{
		Logger:            logger,
		ConnectionManager: manager,
		APIConverter: api.NewConverter[
			*connections.Device[*fakeWorker], *fakeWorker, *fakeController,
		](),
	}
	if !options.omitJobsManager {
		handlerConfig.JobsManager = jobsManager
	}
	if err := handlerConfig.Init(options.settings); err != nil {
		t.Fatalf("init api handler config: %v", err)
	}

	handler := NewAPIHandler(t.Context(), *handlerConfig)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler.SetupAPIRoutes(router.Group("/api"))

	t.Cleanup(manager.Wait)

	return &apiTestEnv{
		handler: handler,
		manager: manager,
		jobs:    jobsManager,
		router:  router,
		cfg:     handlerConfig,
	}
}

// do issues a request against the handler's routes.
func (e *apiTestEnv) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	e.router.ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

// addWorker registers a worker, which is also what brings its device into
// existence in the manager.
func (e *apiTestEnv) addWorker(t *testing.T, workerID, deviceID string) *fakeWorker {
	t.Helper()
	worker := newFakeWorker(workerID, deviceID, "origin-"+deviceID)
	if err := e.manager.RegisterWorker(t.Context(), worker); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	return worker
}

// addController registers a controller through the v1 handshake, the only way
// in short of reaching into the manager's internals.
func (e *apiTestEnv) addController(t *testing.T, id string) *fakeController {
	t.Helper()

	// A controller is only issued a worker the selector says is available, so
	// one has to exist first. Registering a worker for an unknown device marks
	// that device unselectable until its control connection arrives, and
	// EnableDevice only reaches the selector on a state *change* -- so the
	// device has to be toggled to stand in for the control connection this
	// test does not open.
	deviceID := id + "-device"
	e.addWorker(t, id+"-worker", deviceID)
	if _, err := e.manager.DisableDevice(deviceID); err != nil {
		t.Fatalf("disable device: %v", err)
	}
	if _, err := e.manager.EnableDevice(deviceID); err != nil {
		t.Fatalf("enable device: %v", err)
	}

	loginRequest := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{WorkerId: id},
		},
	}
	payload, err := proto.Marshal(loginRequest)
	if err != nil {
		t.Fatalf("marshal login request: %v", err)
	}

	controller, err := e.manager.RegisterControllerConnectionV1(
		t.Context(), &fakeControllerWSConn{firstMessage: payload}, 10, "test-agent",
	)
	if err != nil {
		t.Fatalf("register controller: %v", err)
	}
	return controller
}

func decodeBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", body, err)
	}
	return decoded
}

func assertErrorBody(t *testing.T, body, wantSubstring string) {
	t.Helper()
	decoded := decodeBody(t, body)
	if decoded["status"] != valStatusError {
		t.Errorf("status = %v, want %q (body %s)", decoded["status"], valStatusError, body)
	}
	message, _ := decoded[fieldError].(string)
	if !strings.Contains(message, wantSubstring) {
		t.Errorf("error = %q, want it to contain %q", message, wantSubstring)
	}
}

// --- Device actions ---

// TestDeviceActionOnUnknownDevice pins the error mapping: a device the manager
// has never heard of is a 404, not a 500, for every action that takes one.
func TestDeviceActionOnUnknownDevice(t *testing.T) {
	env := newAPITestEnv(t)

	for _, action := range []string{"restart", "reboot", "logcat", "enable", "disable", "disconnect", "delete"} {
		t.Run(action, func(t *testing.T) {
			status, body := env.do(t, http.MethodPut, "/api/device/ghost/action/"+action, "")
			if status != http.StatusNotFound {
				t.Errorf("status = %d, want 404 (body %s)", status, body)
			}
			assertErrorBody(t, body, "not found")
		})
	}
}

// TestDeviceActionRejectsAllDevicesWildcard covers the "_" guard: only delete
// is defined across every device, so the rest must refuse rather than silently
// act on one device or none.
func TestDeviceActionRejectsAllDevicesWildcard(t *testing.T) {
	env := newAPITestEnv(t)

	for _, action := range []string{"restart", "reboot", "logcat", "enable", "disable", "disconnect"} {
		t.Run(action, func(t *testing.T) {
			status, body := env.do(t, http.MethodPut, "/api/device/_/action/"+action, "")
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", status, body)
			}
			assertErrorBody(t, body, msgActionNotAllDevices)
		})
	}
}

// TestDeviceCommandsOnDisconnectedDevice covers the second error class: the
// device is known but has no control connection, which is a 400 rather than a
// 404 because the operator's request was well-formed.
func TestDeviceCommandsOnDisconnectedDevice(t *testing.T) {
	env := newAPITestEnv(t)
	env.addWorker(t, "worker-1", "device-1")

	for _, action := range []string{"restart", "reboot", "logcat", "disconnect"} {
		t.Run(action, func(t *testing.T) {
			status, body := env.do(t, http.MethodPut, "/api/device/device-1/action/"+action, "")
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", status, body)
			}
			assertErrorBody(t, body, "not connected")
		})
	}
}

func TestDeviceEnableDisable(t *testing.T) {
	env := newAPITestEnv(t)
	env.addWorker(t, "worker-1", "device-1")

	status, body := env.do(t, http.MethodPut, "/api/device/device-1/action/enable", "")
	if status != http.StatusOK {
		t.Fatalf("enable: status = %d, want 200 (body %s)", status, body)
	}
	decoded := decodeBody(t, body)
	device, ok := decoded[fieldDevice].(map[string]any)
	if !ok {
		t.Fatalf("enable reply has no device object: %s", body)
	}
	// The reply carries the device's new state, so a UI need not re-poll.
	if device["enabled"] != true {
		t.Errorf("enabled = %v, want true (body %s)", device["enabled"], body)
	}

	status, body = env.do(t, http.MethodPut, "/api/device/device-1/action/disable", "")
	if status != http.StatusOK {
		t.Fatalf("disable: status = %d, want 200 (body %s)", status, body)
	}
	device, _ = decodeBody(t, body)[fieldDevice].(map[string]any)
	if device["enabled"] != false {
		t.Errorf("enabled = %v, want false (body %s)", device["enabled"], body)
	}
}

func TestDeviceDelete(t *testing.T) {
	t.Run("device with workers is refused", func(t *testing.T) {
		env := newAPITestEnv(t)
		env.addWorker(t, "worker-1", "device-1")

		status, body := env.do(t, http.MethodPut, "/api/device/device-1/action/delete", "")
		if status != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (body %s)", status, body)
		}
		assertErrorBody(t, body, "workers")
	})

	t.Run("unconnected device is removed", func(t *testing.T) {
		env := newAPITestEnv(t)
		worker := env.addWorker(t, "worker-1", "device-1")
		// Closing the worker deregisters it, leaving the device unreferenced.
		_ = worker.Close(0, "")

		status, body := env.do(t, http.MethodPut, "/api/device/device-1/action/delete", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		if env.manager.GetDeviceByID("device-1") != nil {
			t.Error("device is still known after a successful delete")
		}
	})

	t.Run("wildcard removes every dead device", func(t *testing.T) {
		env := newAPITestEnv(t)
		first := env.addWorker(t, "worker-1", "device-1")
		second := env.addWorker(t, "worker-2", "device-2")
		_ = first.Close(0, "")
		_ = second.Close(0, "")

		status, body := env.do(t, http.MethodPut, "/api/device/_/action/delete", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		if count := decodeBody(t, body)["devices_count"]; count != float64(2) {
			t.Errorf("devices_count = %v, want 2 (body %s)", count, body)
		}
	})
}

func TestDeviceActionUnknownAction(t *testing.T) {
	env := newAPITestEnv(t)

	status, body := env.do(t, http.MethodPut, "/api/device/device-1/action/explode", "")
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body %s)", status, body)
	}
	assertErrorBody(t, body, "Action not found")
}

// --- Controller actions ---

func TestControllerActions(t *testing.T) {
	t.Run("unknown controller", func(t *testing.T) {
		env := newAPITestEnv(t)
		for _, action := range []string{"disconnect", "reconnect"} {
			status, body := env.do(t, http.MethodPut, "/api/controller/nope/action/"+action, "")
			if status != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404 (body %s)", action, status, body)
			}
			assertErrorBody(t, body, "Controller not found")
		}
	})

	t.Run("disconnect closes the connection", func(t *testing.T) {
		env := newAPITestEnv(t)
		controller := env.addController(t, "ctrl-1")

		status, body := env.do(t, http.MethodPut,
			"/api/controller/"+controller.UUID()+"/action/disconnect", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		// The close runs in the manager's background goroutine.
		env.manager.Wait()
		if !controller.closed.Load() {
			t.Error("controller was not closed")
		}
	})

	t.Run("reconnect closes with the restart-session code", func(t *testing.T) {
		env := newAPITestEnv(t)
		controller := env.addController(t, "ctrl-2")

		status, body := env.do(t, http.MethodPut,
			"/api/controller/"+controller.UUID()+"/action/reconnect", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		env.manager.Wait()
		if !controller.closed.Load() {
			t.Error("controller was not closed")
		}
		// Disconnect and reconnect differ only in the close code the controller
		// sees, which is what tells it whether to come back.
		if code := controller.closeCode.Load(); code == 0 {
			t.Error("controller was closed with no status code")
		}
	})

	t.Run("unknown action", func(t *testing.T) {
		env := newAPITestEnv(t)
		status, body := env.do(t, http.MethodPut, "/api/controller/nope/action/explode", "")
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, "Action not found")
	})
}

// --- Jobs ---

const testJobFile = `{"id": "whoami", "description": "run whoami", "exec": "whoami"}`

func TestJobEndpointsWhenJobsDisabled(t *testing.T) {
	// Both ways of turning jobs off must behave the same: the setting off, and
	// the app not wiring a jobs manager at all.
	for name, env := range map[string]*apiTestEnv{
		"setting disabled": newAPITestEnv(t, withSettings(APIHandlerSettings{JobsEnabled: false})),
		"no jobs manager":  newAPITestEnv(t, withoutJobsManager()),
	} {
		t.Run(name, func(t *testing.T) {
			requests := []struct{ method, path string }{
				{http.MethodGet, "/api/job"},
				{http.MethodGet, "/api/job/whoami"},
				{http.MethodPut, "/api/job/-/reload"},
				{http.MethodPut, "/api/job/whoami/run"},
				{http.MethodGet, "/api/job-instance"},
				{http.MethodGet, "/api/job-instance/1"},
				{http.MethodPut, "/api/job-instance/1/clear"},
			}
			for _, request := range requests {
				status, body := env.do(t, request.method, request.path, "")
				if status != http.StatusNotFound {
					t.Errorf("%s %s: status = %d, want 404 (body %s)",
						request.method, request.path, status, body)
				}
				assertErrorBody(t, body, msgJobsNotEnabled)
			}
		})
	}
}

func TestGetJob(t *testing.T) {
	env := newAPITestEnv(t, withJobFile("whoami.json", testJobFile))

	t.Run("known job", func(t *testing.T) {
		status, body := env.do(t, http.MethodGet, "/api/job/whoami", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		job, ok := decodeBody(t, body)["job"].(map[string]any)
		if !ok || job["id"] != "whoami" {
			t.Errorf("job = %v, want id whoami (body %s)", job, body)
		}
	})

	t.Run("unknown job", func(t *testing.T) {
		status, body := env.do(t, http.MethodGet, "/api/job/nope", "")
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, "job not found")
	})
}

func TestReloadJobs(t *testing.T) {
	env := newAPITestEnv(t, withJobFile("whoami.json", testJobFile))

	t.Run("all jobs", func(t *testing.T) {
		status, body := env.do(t, http.MethodPut, "/api/job/-/reload", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
	})

	t.Run("single job is not supported", func(t *testing.T) {
		// Reloading one job would need the manager to reconcile a partial view
		// of the jobs directory; the endpoint says so rather than pretending.
		status, body := env.do(t, http.MethodPut, "/api/job/whoami/reload", "")
		if status != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (body %s)", status, body)
		}
		assertErrorBody(t, body, "not implemented")
	})
}

func TestRunJob(t *testing.T) {
	env := newAPITestEnv(t, withJobFile("whoami.json", testJobFile))

	t.Run("unknown job", func(t *testing.T) {
		status, body := env.do(t, http.MethodPut, "/api/job/nope/run", `{"device_ids":["device-1"]}`)
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, "job not found")
	})

	t.Run("malformed body", func(t *testing.T) {
		status, body := env.do(t, http.MethodPut, "/api/job/whoami/run", `{"device_ids":`)
		if status != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (body %s)", status, body)
		}
		assertErrorBody(t, body, "failed to decode request")
	})

	t.Run("no device ids", func(t *testing.T) {
		status, body := env.do(t, http.MethodPut, "/api/job/whoami/run", `{"device_ids":[]}`)
		if status != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (body %s)", status, body)
		}
		assertErrorBody(t, body, "no 'device_ids'")
	})

	t.Run("device that cannot run it still yields an instance", func(t *testing.T) {
		// A job against an unreachable device is recorded as a failed instance
		// rather than an error reply: the operator asked for N runs and gets N
		// results, each with its own outcome.
		status, body := env.do(t, http.MethodPut, "/api/job/whoami/run",
			`{"device_ids":["device-1","device-2"]}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		instances, ok := decodeBody(t, body)["instances"].([]any)
		if !ok || len(instances) != 2 {
			t.Fatalf("instances = %v, want 2 (body %s)", instances, body)
		}
	})
}

func TestJobInstances(t *testing.T) {
	env := newAPITestEnv(t, withJobFile("whoami.json", testJobFile))
	instance := env.jobs.AddFailedJobInstance("whoami", "device-1", "nope")

	t.Run("known instance", func(t *testing.T) {
		status, body := env.do(t, http.MethodGet,
			"/api/job-instance/"+strconv.FormatUint(instance.ID, 10), "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
	})

	t.Run("unparseable id", func(t *testing.T) {
		status, body := env.do(t, http.MethodGet, "/api/job-instance/not-a-number", "")
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, msgJobInstanceNotFound)
	})

	t.Run("unknown id", func(t *testing.T) {
		status, body := env.do(t, http.MethodGet, "/api/job-instance/999999", "")
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, msgJobInstanceNotFound)
	})
}

func TestClearJobInstance(t *testing.T) {
	env := newAPITestEnv(t, withJobFile("whoami.json", testJobFile))

	t.Run("unparseable id", func(t *testing.T) {
		status, body := env.do(t, http.MethodPut, "/api/job-instance/not-a-number/clear", "")
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, msgJobInstanceNotFound)
	})

	t.Run("unknown id", func(t *testing.T) {
		status, body := env.do(t, http.MethodPut, "/api/job-instance/999999/clear", "")
		if status != http.StatusNotFound {
			t.Errorf("status = %d, want 404 (body %s)", status, body)
		}
		assertErrorBody(t, body, msgJobInstanceNotFound)
	})

	t.Run("single instance", func(t *testing.T) {
		instance := env.jobs.AddFailedJobInstance("whoami", "device-1", "nope")
		status, body := env.do(t, http.MethodPut,
			"/api/job-instance/"+strconv.FormatUint(instance.ID, 10)+"/clear", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		if _, found := env.jobs.GetJobInstanceByID(instance.ID); found {
			t.Error("instance still present after clear")
		}
	})

	t.Run("all instances", func(t *testing.T) {
		env.jobs.AddFailedJobInstance("whoami", "device-1", "nope")
		status, body := env.do(t, http.MethodPut, "/api/job-instance/-/clear", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", status, body)
		}
		if instances := env.jobs.GetJobInstances(); len(instances) != 0 {
			t.Errorf("%d instances remain after clearing all", len(instances))
		}
	})
}

// --- pprof ---

// TestPprofRoutesRespectTheProfilingSetting matters because these endpoints
// expose runtime internals: they must stay shut unless explicitly enabled.
func TestPprofRoutesRespectTheProfilingSetting(t *testing.T) {
	paths := []string{
		"/api/debug/pprof/",
		"/api/debug/pprof/cmdline",
		"/api/debug/pprof/symbol",
	}

	t.Run("disabled", func(t *testing.T) {
		env := newAPITestEnv(t, withSettings(APIHandlerSettings{ProfilingEnabled: false}))
		for _, path := range paths {
			status, body := env.do(t, http.MethodGet, path, "")
			if status != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404 (body %s)", path, status, body)
			}
			if !strings.Contains(body, msgProfilingDisabled) {
				t.Errorf("%s: body = %q, want it to mention profiling being off", path, body)
			}
		}
	})

	t.Run("enabled", func(t *testing.T) {
		env := newAPITestEnv(t, withSettings(APIHandlerSettings{ProfilingEnabled: true}))
		for _, path := range paths {
			status, _ := env.do(t, http.MethodGet, path, "")
			if status != http.StatusOK {
				t.Errorf("%s: status = %d, want 200", path, status)
			}
		}
	})
}

// TestPprofProfileAndTraceRespectTheSetting covers the two sampling handlers
// separately: enabled they block for the sample duration, so only the gated
// path is worth asserting here.
func TestPprofProfileAndTraceRespectTheSetting(t *testing.T) {
	env := newAPITestEnv(t, withSettings(APIHandlerSettings{ProfilingEnabled: false}))

	for _, path := range []string{"/api/debug/pprof/profile", "/api/debug/pprof/trace"} {
		status, body := env.do(t, http.MethodGet, path, "")
		if status != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 (body %s)", path, status, body)
		}
	}
}

func TestPprofProfileWhenEnabled(t *testing.T) {
	env := newAPITestEnv(t, withSettings(APIHandlerSettings{ProfilingEnabled: true}))

	// pprof parses seconds as an integer and falls back to 30 for anything <= 0,
	// so 1 is the shortest sample that does not stall the suite.
	status, body := env.do(t, http.MethodGet, "/api/debug/pprof/profile?seconds=1", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(body) == 0 {
		t.Error("profile body is empty, want pprof output")
	}
}

func TestPprofTraceWhenEnabled(t *testing.T) {
	env := newAPITestEnv(t, withSettings(APIHandlerSettings{ProfilingEnabled: true}))

	done := make(chan struct{})
	go func() {
		defer close(done)
		status, _ := env.do(t, http.MethodGet, "/api/debug/pprof/trace?seconds=0.01", "")
		if status != http.StatusOK {
			t.Errorf("status = %d, want 200", status)
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("trace handler did not return")
	}
}

// --- Settings plumbing ---

func TestSettingsReloadIsPickedUpLive(t *testing.T) {
	env := newAPITestEnv(t, withSettings(APIHandlerSettings{JobsEnabled: false}))

	status, _ := env.do(t, http.MethodGet, "/api/job", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 before the reload", status)
	}

	// A config reload swaps the settings container's contents underneath the
	// running handler; no restart, and no re-registered routes.
	if err := env.cfg.PutSettings(APIHandlerSettings{JobsEnabled: true}); err != nil {
		t.Fatalf("put settings: %v", err)
	}

	status, body := env.do(t, http.MethodGet, "/api/job", "")
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 after enabling jobs (body %s)", status, body)
	}
}

func TestNewAPIHandlerDefaultsGlobalRequestStats(t *testing.T) {
	// A nil collector would panic on the first /api/status; the constructor
	// substitutes an empty one instead.
	env := newAPITestEnv(t)
	if env.handler.globalRequestStats == nil {
		t.Fatal("globalRequestStats is nil")
	}

	status, body := env.do(t, http.MethodGet, "/api/status", "")
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 (body %s)", status, body)
	}
}

func TestAPIHandlerSettingsValidate(t *testing.T) {
	if err := (APIHandlerSettings{}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}
