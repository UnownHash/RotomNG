package app_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/logging"

	uiapp "github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app"
	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/config"
)

// waitTimeout bounds the polls below. Conditions normally settle in a few
// milliseconds; this is only a safety net against a wedged test.
const waitTimeout = 10 * time.Second

var testHTTPClient = &http.Client{
	Transport: &http.Transport{DisableKeepAlives: true},
}

// fakeInstance stands in for a rotom-ng server: it answers /api/config the way
// one does, and records what it is asked for.
type fakeInstance struct {
	server *httptest.Server
	// instanceName is reported as the config's `instance`; empty omits it.
	instanceName string
	// secret, when set, is required on requests -- as an instance with an api
	// secret configured would require.
	secret string
	down   atomic.Bool
	// lastStatusSecret records the secret header seen on /api/status, the
	// endpoint the tests proxy.
	lastStatusSecret atomic.Pointer[string]
}

func newFakeInstance(t *testing.T, name, secret string) *fakeInstance {
	t.Helper()
	instance := &fakeInstance{instanceName: name, secret: secret}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if !instance.authorize(w, r) {
			return
		}
		if instance.down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		config := map[string]any{
			"version": "9.9.9",
			"sha":     "deadbeef",
			"tuning":  map[string]any{"profiling": false},
			"jobs":    map[string]any{"enable": true, "path": "./jobs"},
		}
		if instance.instanceName != "" {
			config["instance"] = instance.instanceName
		}
		writeJSON(w, map[string]any{"status": "ok", "config": config})
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		seen := r.Header.Get("X-Rotom-Secret")
		instance.lastStatusSecret.Store(&seen)
		if !instance.authorize(w, r) {
			return
		}
		writeJSON(w, map[string]any{"devices": []any{}, "controllers": []any{}, "from": instance.instanceName})
	})
	instance.server = httptest.NewServer(mux)
	t.Cleanup(instance.server.Close)
	return instance
}

func (f *fakeInstance) authorize(w http.ResponseWriter, r *http.Request) bool {
	if f.secret != "" && r.Header.Get("X-Rotom-Secret") != f.secret {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (f *fakeInstance) url() string { return f.server.URL }

func writeJSON(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	encoded, err := json.Marshal(body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(encoded)
}

// testConfig builds a valid admin config wired to ephemeral ports and a temp
// log file, so tests neither collide on ports nor spam the console.
func testConfig(t *testing.T, instances ...config.Instance) *config.Config {
	t.Helper()
	cfg := &config.Config{
		HTTPListener: &config.HTTPListener{Address: "127.0.0.1:0"},
		Instances:    instances,
		InstanceMonitor: &config.InstanceMonitor{
			Interval: 20 * time.Millisecond,
			Timeout:  2 * time.Second,
		},
		Logging: &logging.Config{
			Level:        "debug",
			NoConsoleLog: true,
			File:         &logging.FileConfig{Path: filepath.Join(t.TempDir(), "test.log")},
		},
	}
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("test config is invalid: %v", err)
	}
	return cfg
}

// uiDir creates a stand-in for the built UI bundle.
func uiDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>rotom ui</html>"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	return dir
}

// startApp starts the service on an ephemeral port and returns its address.
func startApp(t *testing.T, cfg *config.Config, reload func() (*config.Config, error)) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cfg.HTTPListener.Listener = listener
	cfg.HTTPListener.Address = listener.Addr().String()

	if reload == nil {
		reload = func() (*config.Config, error) { return cfg, nil }
	}

	app, err := uiapp.NewApp(cfg, uiapp.FlagConfig{
		UIPath:       uiDir(t),
		ReloadConfig: reload,
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if err := app.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		app.Run()
	}()
	t.Cleanup(func() {
		app.Cancel()
		select {
		case <-done:
		case <-time.After(waitTimeout):
			t.Error("app did not shut down")
		}
	})

	addr := listener.Addr().String()
	waitFor(t, "server to accept connections", func() bool {
		response, err := testHTTPClient.Get("http://" + addr + "/api/config")
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return true
	})
	return addr
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// configReply mirrors the JSON this service returns from /api/config. It is
// declared here rather than imported so a change to the wire format shows up
// as a test failure.
type configReply struct {
	Status string `json:"status"`
	Config struct {
		Version   string `json:"version"`
		SHA       string `json:"sha"`
		Instance  string `json:"instance"`
		Instances *[]struct {
			Instance  string          `json:"instance"`
			URL       string          `json:"url"`
			Reachable bool            `json:"reachable"`
			Config    json.RawMessage `json:"config"`
		} `json:"instances"`
	} `json:"config"`
}

func getConfig(t *testing.T, addr string) configReply {
	t.Helper()
	response, body := request(t, http.MethodGet, "http://"+addr+"/api/config", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/config: status %d, body %s", response.StatusCode, body)
	}
	var reply configReply
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("decode /api/config: %v (body %s)", err, body)
	}
	return reply
}

func request(t *testing.T, method, url string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	response, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response, body
}

// TestConfigAlwaysReportsInstances pins the contract the UI keys off: this
// service always sends an "instances" list, even an empty one, and rotom-ng
// never does. That presence test is how the UI knows which mode it is in.
func TestConfigAlwaysReportsInstances(t *testing.T) {
	addr := startApp(t, testConfig(t), nil)

	response, body := request(t, http.MethodGet, "http://"+addr+"/api/config", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d, body %s", response.StatusCode, body)
	}

	// Checked against the raw JSON: an omitted key and a null both decode to a
	// nil slice, and only one of those is acceptable.
	var raw struct {
		Config map[string]json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	instances, ok := raw.Config["instances"]
	if !ok {
		t.Fatalf("config has no instances key: %s", body)
	}
	if string(instances) != "[]" {
		t.Errorf("instances = %s, want an empty array", instances)
	}
}

func TestConfigReportsInstanceStateAndConfig(t *testing.T) {
	instance := newFakeInstance(t, "east", "instance-secret")
	addr := startApp(t, testConfig(t, config.Instance{
		BaseURL:   instance.url(),
		APISecret: "instance-secret",
	}), nil)

	var reply configReply
	waitFor(t, "the instance to be probed", func() bool {
		reply = getConfig(t, addr)
		return reply.Config.Instances != nil && len(*reply.Config.Instances) == 1 &&
			(*reply.Config.Instances)[0].Reachable
	})

	state := (*reply.Config.Instances)[0]
	if state.Instance != "east" {
		t.Errorf("instance = %q, want %q", state.Instance, "east")
	}
	if state.URL != instance.url() {
		t.Errorf("url = %q, want %q", state.URL, instance.url())
	}

	// The instance's own config rides along, which is what lets the UI gate
	// features (the Jobs tab here) on the instance the operator selected
	// rather than on this service.
	var instanceConfig struct {
		Instance string `json:"instance"`
		Jobs     struct {
			Enable bool `json:"enable"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(state.Config, &instanceConfig); err != nil {
		t.Fatalf("decode instance config: %v", err)
	}
	if !instanceConfig.Jobs.Enable {
		t.Errorf("instance config = %s, want jobs enabled", state.Config)
	}

	// The reply's top-level version is this service's own, not the instance's.
	if reply.Config.Version == "9.9.9" {
		t.Error("top-level version came from the instance, want this service's own")
	}
}

func TestConfigReportsUnreachableInstance(t *testing.T) {
	instance := newFakeInstance(t, "east", "")
	addr := startApp(t, testConfig(t, config.Instance{BaseURL: instance.url()}), nil)

	waitFor(t, "the instance to become reachable", func() bool {
		reply := getConfig(t, addr)
		return reply.Config.Instances != nil && (*reply.Config.Instances)[0].Reachable
	})

	instance.down.Store(true)

	waitFor(t, "the instance to become unreachable", func() bool {
		reply := getConfig(t, addr)
		return !(*reply.Config.Instances)[0].Reachable
	})

	// The last known config survives, so the UI's feature gating does not
	// thrash while an instance is briefly down.
	reply := getConfig(t, addr)
	if len((*reply.Config.Instances)[0].Config) == 0 {
		t.Error("config was dropped when the instance went away, want it retained")
	}
}

// TestUnknownAPIPathsAreProxied covers the design decision that makes this
// service forward-compatible: anything it does not serve itself goes upstream,
// so a new rotom-ng endpoint needs no change here.
func TestUnknownAPIPathsAreProxied(t *testing.T) {
	east := newFakeInstance(t, "east", "east-secret")
	west := newFakeInstance(t, "west", "")
	addr := startApp(t, testConfig(t,
		config.Instance{BaseURL: east.url(), APISecret: "east-secret"},
		config.Instance{BaseURL: west.url()},
	), nil)

	waitFor(t, "both instances to be reachable", func() bool {
		reply := getConfig(t, addr)
		if reply.Config.Instances == nil || len(*reply.Config.Instances) != 2 {
			return false
		}
		return (*reply.Config.Instances)[0].Reachable && (*reply.Config.Instances)[1].Reachable
	})

	t.Run("selected by url", func(t *testing.T) {
		response, body := request(t, http.MethodGet, "http://"+addr+"/api/status",
			map[string]string{"X-Rotom-Instance": west.url()})
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status %d, body %s", response.StatusCode, body)
		}
		assertFrom(t, body, "west")
	})

	t.Run("selected by name", func(t *testing.T) {
		response, body := request(t, http.MethodGet, "http://"+addr+"/api/status",
			map[string]string{"X-Rotom-Instance": "east"})
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status %d, body %s", response.StatusCode, body)
		}
		assertFrom(t, body, "east")
		// The instance's own secret is what authenticates the hop.
		if got := *east.lastStatusSecret.Load(); got != "east-secret" {
			t.Errorf("instance saw secret %q, want %q", got, "east-secret")
		}
	})

	t.Run("no selection falls back to the first reachable", func(t *testing.T) {
		response, body := request(t, http.MethodGet, "http://"+addr+"/api/status", nil)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status %d, body %s", response.StatusCode, body)
		}
		assertFrom(t, body, "east")
	})

	t.Run("unknown instance", func(t *testing.T) {
		response, _ := request(t, http.MethodGet, "http://"+addr+"/api/status",
			map[string]string{"X-Rotom-Instance": "nope"})
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", response.StatusCode)
		}
	})
}

func assertFrom(t *testing.T, body []byte, want string) {
	t.Helper()
	var reply struct {
		From string `json:"from"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("decode: %v (body %s)", err, body)
	}
	if reply.From != want {
		t.Errorf("answered by %q, want %q", reply.From, want)
	}
}

func TestProxyReportsWhenNothingIsReachable(t *testing.T) {
	addr := startApp(t, testConfig(t), nil)

	response, body := request(t, http.MethodGet, "http://"+addr+"/api/status", nil)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (body %s)", response.StatusCode, body)
	}
}

// TestAPISecretGuardsBothLocalAndProxiedRoutes matters because the proxied
// routes are reached through gin's NoRoute, outside the authenticated group;
// they have to be just as protected as the routes inside it.
func TestAPISecretGuardsBothLocalAndProxiedRoutes(t *testing.T) {
	instance := newFakeInstance(t, "east", "")
	cfg := testConfig(t, config.Instance{BaseURL: instance.url()})
	cfg.HTTPListener.Secret = "admin-secret"
	addr := startApp(t, cfg, nil)

	waitFor(t, "the instance to be reachable", func() bool {
		response, _ := request(t, http.MethodGet, "http://"+addr+"/api/config",
			map[string]string{"X-Rotom-Secret": "admin-secret"})
		return response.StatusCode == http.StatusOK
	})

	for _, path := range []string{"/api/config", "/api/status"} {
		t.Run("unauthenticated"+path, func(t *testing.T) {
			response, _ := request(t, http.MethodGet, "http://"+addr+path, nil)
			if response.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", response.StatusCode)
			}
		})

		t.Run("authenticated"+path, func(t *testing.T) {
			response, body := request(t, http.MethodGet, "http://"+addr+path,
				map[string]string{"X-Rotom-Secret": "admin-secret"})
			if response.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200 (body %s)", response.StatusCode, body)
			}
		})
	}

	// The session probe has to stay reachable without a credential, or the UI
	// could never present a login form.
	response, _ := request(t, http.MethodGet, "http://"+addr+"/api/auth/me", nil)
	if response.StatusCode != http.StatusOK {
		t.Errorf("GET /api/auth/me status = %d, want 200", response.StatusCode)
	}
}

func TestUIIsServedForNonAPIPaths(t *testing.T) {
	addr := startApp(t, testConfig(t), nil)

	for _, path := range []string{"/", "/devices"} {
		response, body := request(t, http.MethodGet, "http://"+addr+path, nil)
		if response.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, response.StatusCode)
		}
		if string(body) != "<html>rotom ui</html>" {
			t.Errorf("GET %s body = %q, want the UI index", path, body)
		}
	}
}

func TestConfigReloadAppliesInstanceChanges(t *testing.T) {
	east := newFakeInstance(t, "east", "")
	west := newFakeInstance(t, "west", "")

	cfg := testConfig(t, config.Instance{BaseURL: east.url()})

	var reloaded atomic.Bool
	addr := startApp(t, cfg, func() (*config.Config, error) {
		if !reloaded.Load() {
			return cfg, nil
		}
		return testConfig(t,
			config.Instance{BaseURL: east.url()},
			config.Instance{BaseURL: west.url()},
		), nil
	})

	waitFor(t, "the first instance to be reachable", func() bool {
		reply := getConfig(t, addr)
		return reply.Config.Instances != nil && len(*reply.Config.Instances) == 1 &&
			(*reply.Config.Instances)[0].Reachable
	})

	reloaded.Store(true)
	response, body := request(t, http.MethodPut, "http://"+addr+"/api/config/reload", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reload status %d, body %s", response.StatusCode, body)
	}

	waitFor(t, "the added instance to be reachable", func() bool {
		reply := getConfig(t, addr)
		if reply.Config.Instances == nil || len(*reply.Config.Instances) != 2 {
			return false
		}
		return (*reply.Config.Instances)[1].Reachable
	})

	// The instance that survived the reload must not have been reset to
	// "never contacted" along the way.
	reply := getConfig(t, addr)
	if !(*reply.Config.Instances)[0].Reachable {
		t.Error("the surviving instance lost its reachable state across a reload")
	}

	response, body = request(t, http.MethodGet, "http://"+addr+"/api/status",
		map[string]string{"X-Rotom-Instance": west.url()})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("proxy to the added instance: status %d, body %s", response.StatusCode, body)
	}
	assertFrom(t, body, "west")
}

func TestInitRejectsMissingUI(t *testing.T) {
	cfg := testConfig(t)
	app, err := uiapp.NewApp(cfg, uiapp.FlagConfig{
		UIPath:       filepath.Join(t.TempDir(), "does-not-exist"),
		ReloadConfig: func() (*config.Config, error) { return cfg, nil },
	})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	err = app.Init()
	if err == nil {
		t.Fatal("Init succeeded with no UI files, want an error")
	}
	if want := "index.html"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want it to mention %s", err, want)
	}
}
