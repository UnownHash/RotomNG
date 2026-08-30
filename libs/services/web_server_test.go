package services

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// testUIFS stands in for the built UI bundle. WebServer looks for a directory
// named "static" at the root of the FS, so the fixture has to live under that
// name in this package -- see libs/services/static.
//
//go:embed static
var testUIFS embed.FS

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// allowingAuth accepts or rejects on demand and implements RequestAuthorizer,
// as the real auth.Middleware does.
type allowingAuth struct {
	allow bool
	// handlerCalls counts in-chain middleware invocations, distinguishing the
	// registered-route path from the NoRoute path.
	handlerCalls atomic.Int64
}

func (a *allowingAuth) Handler(c *gin.Context) {
	a.handlerCalls.Add(1)
	if !a.allow {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

// sessionAuth additionally registers unauthenticated session routes.
type sessionAuth struct {
	allowingAuth

	setupCalls atomic.Int64
}

func (s *sessionAuth) SetupSessionRoutes(group *gin.RouterGroup, _ *slog.Logger) {
	s.setupCalls.Add(1)
	group.GET("/auth/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

// uiDir writes a stand-in for the built UI bundle on disk.
func uiDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>ui</html>"), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.txt"), []byte("asset"), 0o600); err != nil {
		t.Fatalf("write asset.txt: %v", err)
	}
	return dir
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

func doGet(t *testing.T, engine *gin.Engine, path string) (int, string) {
	t.Helper()
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response.Code, response.Body.String()
}

// --- API fallback ---

// TestAPIMissWithoutFallbackIs404 pins the default: an unknown /api path is a
// JSON 404, not the SPA's index.html. Serving HTML there would make a typo in
// a client's URL look like a successful page load.
func TestAPIMissWithoutFallbackIs404(t *testing.T) {
	engine, err := newTestWebServer(t, WebServerConfig{UIPath: uiDir(t)})
	if err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	status, body := doGet(t, engine, "/api/nope")
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body %s)", status, body)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, body)
	}
	if decoded["error"] != "resource does not exist" {
		t.Errorf("error = %v, want %q", decoded["error"], "resource does not exist")
	}
}

// --- UI serving ---

func TestUIIsServedFromDisk(t *testing.T) {
	engine, err := newTestWebServer(t, WebServerConfig{UIPath: uiDir(t)})
	if err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	t.Run("static asset", func(t *testing.T) {
		status, body := doGet(t, engine, "/asset.txt")
		if status != http.StatusOK || body != "asset" {
			t.Errorf("status = %d, body = %q; want the asset", status, body)
		}
	})

	t.Run("unknown path falls back to the SPA index", func(t *testing.T) {
		// Client-side routes have no file behind them; serving index.html is
		// what lets a deep link load rather than 404.
		status, body := doGet(t, engine, "/devices/some-id")
		if status != http.StatusOK || !strings.Contains(body, "<html>ui</html>") {
			t.Errorf("status = %d, body = %q; want index.html", status, body)
		}
	})
}

func TestUIIsServedFromEmbeddedFS(t *testing.T) {
	engine, err := newTestWebServer(t, WebServerConfig{UIFS: &testUIFS})
	if err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	status, body := doGet(t, engine, "/asset.txt")
	if status != http.StatusOK || !strings.Contains(body, "fixture asset") {
		t.Errorf("status = %d, body = %q; want the embedded asset", status, body)
	}

	status, body = doGet(t, engine, "/deep/link")
	if status != http.StatusOK || !strings.Contains(body, "embedded ui fixture") {
		t.Errorf("status = %d, body = %q; want the embedded index", status, body)
	}
}

// TestSetupRoutesRejectsNoUISource covers the one unusable-UI case that is
// reachable. The sibling branch -- static.EmbedFolder failing -- is not: it
// only errors on a path fs.ValidPath rejects, and the path passed is the
// hardcoded literal "static", so a missing directory yields an empty FS rather
// than an error.
func TestSetupRoutesRejectsNoUISource(t *testing.T) {
	_, err := newTestWebServer(t, WebServerConfig{})
	if err == nil {
		t.Fatal("SetupRoutes succeeded with no UI at all, want an error")
	}
	if !strings.Contains(err.Error(), "no embedded UI") {
		t.Errorf("error = %v, want it to name the missing UI", err)
	}
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

// --- Route wiring ---

// TestSessionRoutesAreRegisteredUnauthenticated covers why session endpoints
// get their own group: they are how a browser obtains a credential, so gating
// them behind the credential would lock the UI out permanently.
func TestSessionRoutesAreRegisteredUnauthenticated(t *testing.T) {
	auth := &sessionAuth{allowingAuth: allowingAuth{allow: false}}
	engine, err := newTestWebServer(t, WebServerConfig{
		UIPath:         uiDir(t),
		AuthMiddleware: auth,
		SetupAPIRoutes: func(group *gin.RouterGroup) {
			group.GET("/guarded", func(c *gin.Context) { c.Status(http.StatusOK) })
		},
	})
	if err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	if auth.setupCalls.Load() != 1 {
		t.Errorf("SetupSessionRoutes called %d times, want 1", auth.setupCalls.Load())
	}

	if status, body := doGet(t, engine, "/api/auth/me"); status != http.StatusOK {
		t.Errorf("session route status = %d, want 200 (body %s)", status, body)
	}
	if status, _ := doGet(t, engine, "/api/guarded"); status != http.StatusUnauthorized {
		t.Errorf("guarded route status = %d, want 401", status)
	}
}

func TestAPIRoutesAreGuardedByAuth(t *testing.T) {
	auth := &allowingAuth{allow: true}
	engine, err := newTestWebServer(t, WebServerConfig{
		UIPath:         uiDir(t),
		AuthMiddleware: auth,
		SetupAPIRoutes: func(group *gin.RouterGroup) {
			group.GET("/thing", func(c *gin.Context) { c.Status(http.StatusOK) })
		},
	})
	if err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	if status, _ := doGet(t, engine, "/api/thing"); status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if auth.handlerCalls.Load() != 1 {
		t.Errorf("auth middleware ran %d times, want 1", auth.handlerCalls.Load())
	}

	// The UI is public: gating it would mean the login form itself never loads.
	if status, _ := doGet(t, engine, "/"); status != http.StatusOK {
		t.Errorf("ui status = %d, want 200", status)
	}
	if auth.handlerCalls.Load() != 1 {
		t.Error("auth middleware ran for a UI request")
	}
}

func TestNewWebServerPropagatesSetupErrors(t *testing.T) {
	// A WebServer with no UI source cannot install its routes, and that has to
	// surface as a constructor error rather than a server that 404s everything.
	_, err := NewWebServer(context.Background(), testLogger(), WebServerConfig{
		Address:        "127.0.0.1:0",
		SetupAPIRoutes: func(*gin.RouterGroup) {},
	})
	if err == nil {
		t.Fatal("NewWebServer succeeded with no UI source, want an error")
	}
}

func TestNewWebServerBuildsAServer(t *testing.T) {
	server, err := NewWebServer(context.Background(), testLogger(), WebServerConfig{
		Address:        "127.0.0.1:0",
		UIPath:         uiDir(t),
		SetupAPIRoutes: func(*gin.RouterGroup) {},
	})
	if err != nil {
		t.Fatalf("NewWebServer: %v", err)
	}
	if server.HTTPServer == nil {
		t.Error("WebServer has no underlying HTTPServer")
	}
}
