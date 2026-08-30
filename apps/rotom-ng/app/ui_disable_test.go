package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/testutil"
)

// getPath fetches an arbitrary path and returns the status and body.
func getPath(t *testing.T, httpAddr, path string) (int, string) {
	t.Helper()
	resp, err := testHTTPClient.Get("http://" + httpAddr + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	return resp.StatusCode, string(body[:n])
}

// TestUI_DisabledWithholdsUIButNotAPI covers http_listener.disable_ui: the
// REST API keeps working while every browser-facing path is refused.
func TestUI_DisabledWithholdsUIButNotAPI(t *testing.T) {
	env, err := testutil.NewTestEnv(testutil.WithDisableUI(true))
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	status, body := getPath(t, env.HTTPAddr, "/")
	if status != http.StatusNotFound {
		t.Errorf("GET / status = %d, want 404 (body %s)", status, body)
	}
	if !strings.Contains(body, "disabled") {
		t.Errorf("GET / body = %q, want it to say the UI is disabled", body)
	}

	// The API is the whole point of running this way.
	if status, body := getPath(t, env.HTTPAddr, "/api/status"); status != http.StatusOK {
		t.Errorf("GET /api/status = %d, want 200 (body %s)", status, body)
	}

	// Reported back so an operator can confirm the setting took.
	status, body = getPath(t, env.HTTPAddr, "/api/config")
	if status != http.StatusOK {
		t.Fatalf("GET /api/config = %d (body %s)", status, body)
	}
	var reply struct {
		Config struct {
			HTTPListener struct {
				DisableUI bool `json:"disable_ui"`
			} `json:"http_listener"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(body), &reply); err != nil {
		t.Fatalf("decode /api/config: %v (body %s)", err, body)
	}
	if !reply.Config.HTTPListener.DisableUI {
		t.Errorf("/api/config did not report disable_ui: %s", body)
	}
}

// TestUI_DisableIsHotReloadable is the property that matters most: the routes
// are registered once at startup, so turning the UI off has to work through a
// config reload rather than needing a restart.
func TestUI_DisableIsHotReloadable(t *testing.T) {
	// The reload callback hands back a config whose disable_ui flips when the
	// test says so, standing in for an edited config file.
	var disabled bool
	reload := func() (*config.Config, error) {
		cfg, cleanup, err := testutil.NewTestConfig(testutil.WithDisableUI(disabled))
		if err != nil {
			return nil, err
		}
		t.Cleanup(cleanup)
		return cfg, nil
	}

	env, err := testutil.NewTestEnv(testutil.WithReloadConfig(reload))
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })

	// Started with the UI on. The test env runs in UI dev mode, so a UI path
	// is proxied at the (absent) dev server and fails as a bad gateway -- which
	// is still proof the request was not refused outright.
	if status, body := getPath(t, env.HTTPAddr, "/"); status == http.StatusNotFound {
		t.Fatalf("GET / = 404 before disabling; want the request to reach the UI path (body %s)", body)
	}

	disabled = true
	if status, body := reloadConfig(t, env.HTTPAddr); status != http.StatusOK {
		t.Fatalf("reload = %d (body %s)", status, body)
	}

	status, body := getPath(t, env.HTTPAddr, "/")
	if status != http.StatusNotFound {
		t.Errorf("GET / after reload = %d, want 404 (body %s)", status, body)
	}
	if !strings.Contains(body, "disabled") {
		t.Errorf("body = %q, want it to say the UI is disabled", body)
	}

	// And back off again, without a restart.
	disabled = false
	if status, body := reloadConfig(t, env.HTTPAddr); status != http.StatusOK {
		t.Fatalf("second reload = %d (body %s)", status, body)
	}
	if status, body := getPath(t, env.HTTPAddr, "/"); status == http.StatusNotFound {
		t.Errorf("GET / after re-enabling = 404, want the UI served again (body %s)", body)
	}
}

// reloadConfig issues PUT /api/config/reload.
func reloadConfig(t *testing.T, httpAddr string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut,
		"http://"+httpAddr+"/api/config/reload", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/config/reload: %v", err)
	}
	defer resp.Body.Close()
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	return resp.StatusCode, string(body[:n])
}
