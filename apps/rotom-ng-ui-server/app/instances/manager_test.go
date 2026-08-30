package instances

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func testSettings(instances ...InstanceConfig) Settings {
	return Settings{
		Instances: instances,
		Interval:  time.Hour,
		Timeout:   time.Second,
	}
}

func newTestManager(t *testing.T, settings Settings) *Manager {
	t.Helper()
	manager, err := NewManager(ManagerConfig{Logger: testLogger(), UserAgent: "test-agent"}, settings)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return manager
}

// configServer stands in for a rotom-ng instance's /api/config endpoint.
type configServer struct {
	server *httptest.Server
	// requests counts config probes, so a test can tell a cached answer from a
	// fresh one.
	requests atomic.Int64
	// gotSecret records the secret header of the most recent probe.
	gotSecret atomic.Pointer[string]
	// down makes the endpoint fail, standing in for an instance going away.
	down atomic.Bool
	// instanceName is the name reported in the config body; empty omits the
	// field entirely, as rotom-ng does when no instance name is set.
	instanceName atomic.Pointer[string]
}

func newConfigServer(t *testing.T, instanceName string) *configServer {
	t.Helper()
	cs := &configServer{}
	cs.instanceName.Store(&instanceName)
	cs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.requests.Add(1)
		secret := r.Header.Get("X-Rotom-Secret")
		cs.gotSecret.Store(&secret)

		if r.URL.Path != configPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if cs.down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		config := map[string]any{"version": "1.2.3", "jobs": map[string]any{"enable": true}}
		if name := *cs.instanceName.Load(); name != "" {
			config["instance"] = name
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "config": config})
	}))
	t.Cleanup(cs.server.Close)
	return cs
}

func (cs *configServer) url() string { return cs.server.URL }

func TestProbeMarksInstanceReachableAndCachesConfig(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: upstream.url(), APISecret: "sekrit"}))

	manager.probeAll(t.Context())

	states := manager.Snapshot()
	if len(states) != 1 {
		t.Fatalf("Snapshot returned %d states, want 1", len(states))
	}
	state := states[0]
	if !state.Reachable {
		t.Error("instance is not reachable after a successful probe")
	}
	if state.Instance != "east" {
		t.Errorf("Instance = %q, want %q", state.Instance, "east")
	}
	if state.URL != upstream.url() {
		t.Errorf("URL = %q, want %q", state.URL, upstream.url())
	}

	// The config is passed through verbatim so the UI reads exactly what it
	// would read from that instance directly.
	var config map[string]any
	if err := json.Unmarshal(state.Config, &config); err != nil {
		t.Fatalf("cached config is not valid JSON: %v", err)
	}
	if config["version"] != "1.2.3" {
		t.Errorf("cached config = %v, want version 1.2.3", config)
	}

	if got := *upstream.gotSecret.Load(); got != "sekrit" {
		t.Errorf("upstream saw secret %q, want %q", got, "sekrit")
	}
}

func TestProbeOmittedInstanceNameIsEmpty(t *testing.T) {
	upstream := newConfigServer(t, "")
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: upstream.url()}))

	manager.probeAll(t.Context())

	state := manager.Snapshot()[0]
	if !state.Reachable {
		t.Fatal("instance is not reachable")
	}
	if state.Instance != "" {
		t.Errorf("Instance = %q, want empty", state.Instance)
	}
	if got := *upstream.gotSecret.Load(); got != "" {
		t.Errorf("upstream saw secret %q, want none sent", got)
	}
}

func TestProbeFailureKeepsLastConfigButClearsReachable(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: upstream.url()}))

	manager.probeAll(t.Context())
	upstream.down.Store(true)
	manager.probeAll(t.Context())

	state := manager.Snapshot()[0]
	if state.Reachable {
		t.Error("instance is still reachable after a failed probe")
	}
	// Keeping the last config means the UI's feature gating does not thrash
	// while an instance is briefly down.
	if len(state.Config) == 0 {
		t.Error("cached config was dropped on failure, want it retained")
	}
	if state.Instance != "east" {
		t.Errorf("Instance = %q, want it retained as %q", state.Instance, "east")
	}
}

func TestSnapshotBeforeFirstProbe(t *testing.T) {
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: "http://never:7072"}))

	states := manager.Snapshot()
	if len(states) != 1 {
		t.Fatalf("Snapshot returned %d states, want 1", len(states))
	}
	if states[0].Reachable {
		t.Error("instance is reachable before any probe ran")
	}
	if states[0].Config != nil {
		t.Errorf("Config = %s, want nil until first contact", states[0].Config)
	}

	// An entry with no config yet must serialise without a config key at all,
	// so the UI can tell "not contacted" from "contacted, empty config".
	encoded, err := json.Marshal(states[0])
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if want := `{"instance":"","url":"http://never:7072","reachable":false}`; string(encoded) != want {
		t.Errorf("state JSON = %s, want %s", encoded, want)
	}
}

func TestSnapshotPreservesConfigOrder(t *testing.T) {
	manager := newTestManager(t, testSettings(
		InstanceConfig{BaseURL: "http://c:7072"},
		InstanceConfig{BaseURL: "http://a:7072"},
		InstanceConfig{BaseURL: "http://b:7072"},
	))

	states := manager.Snapshot()
	want := []string{"http://c:7072", "http://a:7072", "http://b:7072"}
	for idx, url := range want {
		if states[idx].URL != url {
			t.Errorf("states[%d].URL = %q, want %q", idx, states[idx].URL, url)
		}
	}
}

func TestResolve(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, testSettings(
		InstanceConfig{BaseURL: "http://down:7072", APISecret: "down-secret"},
		InstanceConfig{BaseURL: upstream.url(), APISecret: "up-secret"},
	))
	manager.probeAll(t.Context())

	t.Run("by base url", func(t *testing.T) {
		target, err := manager.Resolve(upstream.url())
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if target.BaseURL != upstream.url() || target.APISecret != "up-secret" {
			t.Errorf("target = %+v", target)
		}
	})

	t.Run("by instance name", func(t *testing.T) {
		target, err := manager.Resolve("east")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if target.BaseURL != upstream.url() {
			t.Errorf("target = %+v, want %q", target, upstream.url())
		}
	})

	t.Run("unreachable instance still resolves", func(t *testing.T) {
		// Resolving is not gated on reachability: a request that names a
		// downed instance should fail against that instance, with its own
		// error, rather than be silently answered by a different one.
		target, err := manager.Resolve("http://down:7072")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if target.BaseURL != "http://down:7072" {
			t.Errorf("target = %+v", target)
		}
	})

	t.Run("empty key picks first reachable", func(t *testing.T) {
		target, err := manager.Resolve("")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if target.BaseURL != upstream.url() {
			t.Errorf("target = %+v, want the reachable instance %q", target, upstream.url())
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		_, err := manager.Resolve("http://nope:7072")
		if !IsErrInstanceNotFound(err) {
			t.Errorf("error = %v, want instance-not-found", err)
		}
	})
}

func TestResolveWithNothingUsable(t *testing.T) {
	t.Run("no instances configured", func(t *testing.T) {
		manager := newTestManager(t, testSettings())
		if _, err := manager.Resolve(""); !IsErrNoInstances(err) {
			t.Errorf("error = %v, want no-instances", err)
		}
		if _, err := manager.Resolve("anything"); !IsErrNoInstances(err) {
			t.Errorf("error = %v, want no-instances", err)
		}
	})

	t.Run("none reachable", func(t *testing.T) {
		manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: "http://down:7072"}))
		if _, err := manager.Resolve(""); !IsErrNoInstanceReachable(err) {
			t.Errorf("error = %v, want no-instance-reachable", err)
		}
	})
}

func TestSetSettingsPreservesSurvivingInstances(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: upstream.url(), APISecret: "old"}))
	manager.probeAll(t.Context())

	// A reload that adds an instance and rotates a secret must not blank out
	// the instance it left in place.
	manager.SetSettings(testSettings(
		InstanceConfig{BaseURL: upstream.url(), APISecret: "rotated"},
		InstanceConfig{BaseURL: "http://new:7072"},
	))

	states := manager.Snapshot()
	if len(states) != 2 {
		t.Fatalf("Snapshot returned %d states, want 2", len(states))
	}
	if !states[0].Reachable || states[0].Instance != "east" || len(states[0].Config) == 0 {
		t.Errorf("surviving instance lost its cached state: %+v", states[0])
	}
	if states[1].Reachable {
		t.Error("newly added instance is reachable before being probed")
	}

	target, err := manager.Resolve(upstream.url())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target.APISecret != "rotated" {
		t.Errorf("APISecret = %q, want the rotated value", target.APISecret)
	}
}

func TestSetSettingsDropsRemovedInstances(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: upstream.url()}))
	manager.probeAll(t.Context())

	manager.SetSettings(testSettings(InstanceConfig{BaseURL: "http://other:7072"}))

	if states := manager.Snapshot(); len(states) != 1 || states[0].URL != "http://other:7072" {
		t.Errorf("Snapshot = %+v, want only the remaining instance", states)
	}
	if _, err := manager.Resolve(upstream.url()); !IsErrInstanceNotFound(err) {
		t.Errorf("error = %v, want the removed instance to be unknown", err)
	}
}

func TestChangedSignalsReachabilityTransitions(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: upstream.url()}))

	changed := manager.Changed()
	manager.probeAll(t.Context())
	select {
	case <-changed:
	default:
		t.Fatal("Changed was not signalled when the instance became reachable")
	}

	// A probe that changes nothing must not signal, or a waiter would spin.
	changed = manager.Changed()
	manager.probeAll(t.Context())
	select {
	case <-changed:
		t.Error("Changed was signalled by a probe that changed nothing")
	default:
	}
}

func TestRunProbesUntilContextCancelled(t *testing.T) {
	upstream := newConfigServer(t, "east")
	manager := newTestManager(t, Settings{
		Instances: []InstanceConfig{{BaseURL: upstream.url()}},
		Interval:  5 * time.Millisecond,
		Timeout:   time.Second,
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.Run(ctx)
	}()

	// Wait for the first probe to land rather than sleeping a fixed amount.
	deadline := time.After(5 * time.Second)
	for upstream.requests.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("Run never probed the instance")
		case <-time.After(time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

func TestSettingsValidate(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{name: "ok", settings: testSettings(InstanceConfig{BaseURL: "http://a:7072"})},
		{name: "ok with no instances", settings: testSettings()},
		{name: "zero interval", settings: Settings{Timeout: time.Second}, wantErr: true},
		{name: "zero timeout", settings: Settings{Interval: time.Second}, wantErr: true},
		{
			name:     "empty base url",
			settings: testSettings(InstanceConfig{}),
			wantErr:  true,
		},
		{
			name: "duplicate base url",
			settings: testSettings(
				InstanceConfig{BaseURL: "http://a:7072"},
				InstanceConfig{BaseURL: "http://a:7072"},
			),
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.settings.Validate()
			if (err != nil) != test.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestFetchConfigRejectsBadResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "non-200", body: `{"status":"ok","config":{}}`, code: http.StatusUnauthorized},
		{name: "not json", body: `not json`, code: http.StatusOK},
		{name: "no config object", body: `{"status":"ok"}`, code: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.code)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			manager := newTestManager(t, testSettings(InstanceConfig{BaseURL: server.URL}))
			manager.probeAll(t.Context())

			if manager.Snapshot()[0].Reachable {
				t.Error("instance counted as reachable despite an unusable config response")
			}
		})
	}
}
