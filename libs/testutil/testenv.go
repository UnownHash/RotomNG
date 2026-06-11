package testutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
)

// TestEnv wraps a full RotomNG App lifecycle for integration testing.
// It manages ephemeral port allocation, app startup, readiness detection,
// and clean shutdown with resource cleanup.
type TestEnv struct {
	App            *app.App
	Config         *config.Config
	DeviceAddr     string
	ControllerAddr string
	HTTPAddr       string

	cleanup        func()
	listeners      []net.Listener
	runDone        chan struct{}
	reloadConfigFn func() (*config.Config, error)
}

// NewTestEnv creates a new TestEnv with config created via NewTestConfig.
// It accepts both Option (for config.Config) and TestEnvOption (for TestEnv
// behavior like ReloadConfig). It does NOT start the app; call Start() to launch.
func NewTestEnv(opts ...any) (*TestEnv, error) {
	var configOpts []Option
	var envOpts []TestEnvOption

	for _, opt := range opts {
		switch o := opt.(type) {
		case Option:
			configOpts = append(configOpts, o)
		case TestEnvOption:
			envOpts = append(envOpts, o)
		}
	}

	cfg, cleanup, err := NewTestConfig(configOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating test config: %w", err)
	}

	te := &TestEnv{
		Config:  cfg,
		cleanup: cleanup,
	}

	for _, opt := range envOpts {
		opt(te)
	}

	return te, nil
}

// Start pre-creates listeners on ephemeral ports, injects them into the config,
// creates the App, initializes it, and launches Run() in a goroutine.
func (te *TestEnv) Start() error {
	// Pre-create 3 listeners on ephemeral ports
	var lc net.ListenConfig

	deviceLn, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("creating device listener: %w", err)
	}

	controllerLn, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		deviceLn.Close()
		return fmt.Errorf("creating controller listener: %w", err)
	}

	httpLn, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		deviceLn.Close()
		controllerLn.Close()
		return fmt.Errorf("creating http listener: %w", err)
	}

	te.listeners = []net.Listener{deviceLn, controllerLn, httpLn}

	// Store resolved addresses
	te.DeviceAddr = deviceLn.Addr().String()
	te.ControllerAddr = controllerLn.Addr().String()
	te.HTTPAddr = httpLn.Addr().String()

	// Inject listeners into config
	te.Config.DeviceListener.Listener = deviceLn
	te.Config.DeviceListener.Address = te.DeviceAddr
	te.Config.ControllerListener.Listener = controllerLn
	te.Config.ControllerListener.Address = te.ControllerAddr
	te.Config.HTTPListener.Listener = httpLn
	te.Config.HTTPListener.Address = te.HTTPAddr

	// Create FlagConfig for test usage
	reloadFn := te.reloadConfigFn
	if reloadFn == nil {
		reloadFn = func() (*config.Config, error) {
			return nil, errors.New("no reload config function configured")
		}
	}
	flagCfg := app.FlagConfig{
		UIDev:        true,
		ReloadConfig: reloadFn,
	}

	// Create and initialize the App
	a, err := app.NewApp(te.Config, flagCfg)
	if err != nil {
		te.closeListeners()
		return fmt.Errorf("creating app: %w", err)
	}
	te.App = a

	if err := te.App.Init(); err != nil {
		te.closeListeners()
		return fmt.Errorf("initializing app: %w", err)
	}

	// Launch Run() in a goroutine
	te.runDone = make(chan struct{})
	go func() {
		defer close(te.runDone)
		te.App.Run()
	}()

	return nil
}

// WaitReady blocks until all three listener ports accept TCP connections
// or the timeout expires.
func (te *TestEnv) WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for _, addr := range []string{te.DeviceAddr, te.ControllerAddr, te.HTTPAddr} {
		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for %s to be ready", addr)
			}

			dialer := net.Dialer{Timeout: 100 * time.Millisecond}
			conn, err := dialer.DialContext(context.Background(), "tcp", addr)
			if err == nil {
				conn.Close()
				break
			}

			time.Sleep(10 * time.Millisecond)
		}
	}

	return nil
}

// Stop triggers graceful shutdown via Cancel(), waits for Run() to complete,
// closes the logger (freeing its pipe writer goroutine), and cleans up temp directories.
func (te *TestEnv) Stop() error {
	if te.App != nil {
		te.App.Cancel()
	}

	if te.runDone != nil {
		<-te.runDone
	}

	// Close idle HTTP connections from http.DefaultClient to prevent
	// leaked persistConn goroutines from accumulating across test iterations.
	http.DefaultClient.CloseIdleConnections()

	if te.cleanup != nil {
		te.cleanup()
	}

	return nil
}

// closeListeners closes all pre-created listeners. Used only in error paths
// during Start() -- in the happy path, App.shutdown() closes them.
func (te *TestEnv) closeListeners() {
	for _, ln := range te.listeners {
		ln.Close()
	}
}
