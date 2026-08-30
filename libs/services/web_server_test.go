package services

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// newTestWebServer builds a WebServer and returns the gin engine its routes
// were installed on, so requests can be driven without binding a port.
func newTestWebServer(t *testing.T, config WebServerConfig) (*gin.Engine, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	if config.SetupAPIRoutes == nil {
		config.SetupAPIRoutes = func(*gin.RouterGroup) {}
	}

	server := &WebServer{config: config, logger: testLogger()}
	engine := gin.New()
	if err := server.SetupRoutes(engine); err != nil {
		return nil, err
	}
	return engine, nil
}

// TestDevModeProxiesNonAPIPaths covers the branch taken by -ui-dev: everything
// that is not /api goes to the vite dev server instead of to disk.
//
// It runs against a real server because ReverseProxy takes a different path
// when the request context has no Done channel, which is what a synthetic
// httptest request has -- and that difference is part of why the broken proxy
// went unnoticed for so long.
func TestDevModeProxiesNonAPIPaths(t *testing.T) {
	engine, err := newTestWebServer(t, WebServerConfig{DevMode: true})
	if err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	server := httptest.NewServer(engine)
	defer server.Close()

	get := func(path string) (int, string) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		return response.StatusCode, string(body)
	}

	// API paths are answered here rather than forwarded: the dev server serves
	// the UI, and the API belongs to this process.
	if status, body := get("/api/unknown"); status != http.StatusNotFound {
		t.Errorf("api status = %d, want 404 (body %s)", status, body)
	}

	// The dev-server target is hardcoded, so standing in for it means binding
	// that exact port. Skipping when it is taken keeps the suite usable on a
	// machine that happens to be running the real dev server.
	listener, err := net.Listen("tcp", "localhost:4199")
	if err != nil {
		t.Skipf("cannot bind the dev server port to stand in for vite: %v", err)
	}

	var gotHost, gotPath atomic.Pointer[string]
	devServer := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, path := r.Host, r.URL.Path
			gotHost.Store(&host)
			gotPath.Store(&path)
			_, _ = io.WriteString(w, "vite")
		}),
	}
	go func() { _ = devServer.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = devServer.Shutdown(ctx)
	}()

	status, body := get("/some/page")
	if status != http.StatusOK || body != "vite" {
		t.Fatalf("page = %d %q, want the dev server's response", status, body)
	}
	if got := gotPath.Load(); got == nil || *got != "/some/page" {
		t.Errorf("dev server saw path %v, want /some/page", got)
	}
	// Host is rewritten to the target so the dev server sees a request for
	// itself rather than for the rotom listener.
	if got := gotHost.Load(); got == nil || *got != "localhost:4199" {
		t.Errorf("dev server saw Host %v, want localhost:4199", got)
	}
}

// TestSetupRoutesRejectsNoUISource covers the other way SetupRoutes can fail,
// so a change to it cannot pass on the dev-mode path alone.
func TestSetupRoutesRejectsNoUISource(t *testing.T) {
	_, err := newTestWebServer(t, WebServerConfig{})
	if err == nil {
		t.Fatal("SetupRoutes succeeded with no UI at all, want an error")
	}
	if !strings.Contains(err.Error(), "no embedded UI") {
		t.Errorf("error = %v, want it to name the missing UI", err)
	}
}
