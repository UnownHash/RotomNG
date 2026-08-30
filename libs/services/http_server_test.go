package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// staticRoutes installs one route and records how many times it was installed.
type staticRoutes struct {
	err   error
	calls atomic.Int64
}

func (r *staticRoutes) SetupRoutes(engine *gin.Engine) error {
	r.calls.Add(1)
	if r.err != nil {
		return r.err
	}
	engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	engine.GET("/hang", func(c *gin.Context) {
		<-c.Request.Context().Done()
	})
	return nil
}

// countingRegistrar records that it was given the engine.
type countingRegistrar struct {
	calls atomic.Int64
}

func (r *countingRegistrar) RegisterGinEngine(*gin.Engine) { r.calls.Add(1) }

// runServer starts s and returns a function that shuts it down and waits.
func runServer(t *testing.T, server *HTTPServer) func(context.Context) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Run()
	}()
	return func(ctx context.Context) {
		server.Shutdown(ctx)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("Run did not return after Shutdown")
		}
	}
}

func TestHTTPServerServesOnAProvidedListener(t *testing.T) {
	gin.SetMode(gin.TestMode)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	routes := &staticRoutes{}
	registrar := &countingRegistrar{}
	auth := &allowingAuth{allow: true}

	server, err := NewHTTPServer(t.Context(), testLogger(), HTTPServerConfig{
		Address:         listener.Addr().String(),
		Listener:        listener,
		RoutesInstaller: routes,
		StatsRegistrar:  registrar,
		AuthMiddleware:  auth,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	if routes.calls.Load() != 1 {
		t.Errorf("SetupRoutes called %d times, want 1", routes.calls.Load())
	}
	if registrar.calls.Load() != 1 {
		t.Errorf("StatsRegistrar called %d times, want 1", registrar.calls.Load())
	}

	stop := runServer(t, server)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stop(ctx)
	}()

	response, err := http.Get("http://" + listener.Addr().String() + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
	// The engine-level auth middleware guards every route on these servers,
	// unlike WebServer's, which guards only /api.
	if auth.handlerCalls.Load() == 0 {
		t.Error("auth middleware did not run")
	}
}

func TestHTTPServerServesOnAnAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Claim an ephemeral port, then hand the address (not the listener) over,
	// which is the path taken when the config supplies only an address.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	server, err := NewHTTPServer(t.Context(), testLogger(), HTTPServerConfig{
		Address:         address,
		RoutesInstaller: &staticRoutes{},
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}

	stop := runServer(t, server)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stop(ctx)
	}()

	// The listen is asynchronous, so retry briefly rather than racing it.
	var response *http.Response
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err = http.Get("http://" + address + "/ping")
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", response.StatusCode)
	}
}

// TestHTTPServerRunReportsABindFailure covers the error path in Run: a port it
// cannot claim must return rather than block, so the app's Run goroutine
// cancels the context and the process exits instead of appearing healthy.
func TestHTTPServerRunReportsABindFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()

	server, err := NewHTTPServer(t.Context(), testLogger(), HTTPServerConfig{
		Address:         occupied.Addr().String(),
		RoutesInstaller: &staticRoutes{},
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Run()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run blocked on an address it could not bind")
	}
}

func TestNewHTTPServerPropagatesRouteErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	wantErr := errors.New("routes are broken")
	_, err := NewHTTPServer(t.Context(), testLogger(), HTTPServerConfig{
		Address:         "127.0.0.1:0",
		RoutesInstaller: &staticRoutes{err: wantErr},
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}

// TestHTTPServerShutdownTimesOut covers Shutdown's error branch: an in-flight
// request that outlives the deadline makes Shutdown return, and the server
// logs it rather than hanging the whole shutdown sequence.
func TestHTTPServerShutdownTimesOut(t *testing.T) {
	gin.SetMode(gin.TestMode)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server, err := NewHTTPServer(t.Context(), testLogger(), HTTPServerConfig{
		Address:         listener.Addr().String(),
		Listener:        listener,
		RoutesInstaller: &staticRoutes{},
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		server.Run()
	}()

	// Hold a request open so there is something for Shutdown to wait on.
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	inFlight := make(chan struct{})
	go func() {
		defer close(inFlight)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet,
			"http://"+listener.Addr().String()+"/hang", nil)
		if err != nil {
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
	}()

	// Give the hanging request time to be accepted before shutting down.
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	server.Shutdown(ctx)

	// Release the request and let the server finish.
	cancelRequest()
	<-inFlight

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)

	select {
	case <-runDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}
}

// --- Device and controller servers ---

type stubDeviceHandler struct{ calls atomic.Int64 }

func (h *stubDeviceHandler) HandleDeviceControl(c *gin.Context) {
	h.calls.Add(1)
	c.String(http.StatusOK, "control")
}

type stubWorkerHandler struct{ calls atomic.Int64 }

func (h *stubWorkerHandler) HandleWorker(c *gin.Context) {
	h.calls.Add(1)
	c.String(http.StatusOK, "worker")
}

type stubControllerHandler struct{ v1, v2 atomic.Int64 }

func (h *stubControllerHandler) HandleControllerV1(c *gin.Context) {
	h.v1.Add(1)
	c.String(http.StatusOK, "v1")
}

func (h *stubControllerHandler) HandleControllerV2(c *gin.Context) {
	h.v2.Add(1)
	c.String(http.StatusOK, "v2")
}

// TestDeviceServerRoutes pins the paths devices and workers connect on. They
// are baked into shipped device firmware, so a change here breaks fleets in
// the field rather than just a client.
func TestDeviceServerRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deviceHandler := &stubDeviceHandler{}
	workerHandler := &stubWorkerHandler{}

	server, err := NewDeviceServer(t.Context(), testLogger(), DeviceServerConfig{
		Address:       "127.0.0.1:0",
		DeviceHandler: deviceHandler,
		WorkerHandler: workerHandler,
	})
	if err != nil {
		t.Fatalf("NewDeviceServer: %v", err)
	}

	engine := gin.New()
	if err := server.SetupRoutes(engine); err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	if status, body := doGet(t, engine, "/control"); status != http.StatusOK || body != "control" {
		t.Errorf("/control = %d %q, want the device handler", status, body)
	}
	if status, body := doGet(t, engine, "/"); status != http.StatusOK || body != "worker" {
		t.Errorf("/ = %d %q, want the worker handler", status, body)
	}
}

func TestControllerServerRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &stubControllerHandler{}

	server, err := NewControllerServer(t.Context(), testLogger(), ControllerServerConfig{
		Address: "127.0.0.1:0",
	}, handler)
	if err != nil {
		t.Fatalf("NewControllerServer: %v", err)
	}

	engine := gin.New()
	if err := server.SetupRoutes(engine); err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}

	// The two protocol versions are distinguished by path, which is how an
	// older controller keeps working against a newer rotom-ng.
	if status, body := doGet(t, engine, "/"); status != http.StatusOK || body != "v1" {
		t.Errorf("/ = %d %q, want the v1 handler", status, body)
	}
	if status, body := doGet(t, engine, "/controller"); status != http.StatusOK || body != "v2" {
		t.Errorf("/controller = %d %q, want the v2 handler", status, body)
	}
}
