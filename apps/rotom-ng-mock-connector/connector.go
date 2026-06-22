package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Config holds all options for a simulated fleet run.
type Config struct {
	Devices            int
	WorkersPerDevice   int
	Controllers        int
	DeviceEndpoint     string
	ControllerEndpoint string
	DeviceSecret       string
	ControllerSecret   string
	Origin             string
	IDPrefix           string
	Version            string
	VersionCode        int
	Weight             int
	RPCInterval        time.Duration
	StartupDelay       time.Duration
	ReconnectDelay     time.Duration
	Verbose            bool
}

// Validate checks the configuration for obviously invalid combinations.
func (cfg Config) Validate() error {
	if cfg.Devices < 0 || cfg.WorkersPerDevice < 0 || cfg.Controllers < 0 {
		return errors.New("devices, workers, and controllers must be >= 0")
	}
	if cfg.Devices == 0 && cfg.Controllers == 0 {
		return errors.New("nothing to do: set -devices and/or -controllers")
	}
	// Each controller claims exactly one worker, so when this connector is
	// supplying the workers (devices > 0) there must be enough of them. When
	// devices == 0 the workers come from a separate/existing instance, so this
	// constraint does not apply.
	if cfg.Devices > 0 && cfg.Controllers > cfg.totalWorkers() {
		return fmt.Errorf(
			"not enough workers: %d controllers need at most %d workers (devices*workers = %d*%d); "+
				"reduce -controllers or increase -devices/-workers",
			cfg.Controllers, cfg.totalWorkers(), cfg.Devices, cfg.WorkersPerDevice,
		)
	}
	if !strings.HasPrefix(cfg.DeviceEndpoint, "ws://") && !strings.HasPrefix(cfg.DeviceEndpoint, "wss://") {
		return fmt.Errorf("device-endpoint must start with ws:// or wss:// (got %q)", cfg.DeviceEndpoint)
	}
	if !strings.HasPrefix(cfg.ControllerEndpoint, "ws://") && !strings.HasPrefix(cfg.ControllerEndpoint, "wss://") {
		return fmt.Errorf("controller-endpoint must start with ws:// or wss:// (got %q)", cfg.ControllerEndpoint)
	}
	if cfg.RPCInterval <= 0 {
		return errors.New("rpc-interval must be > 0")
	}
	// Weight is clamped to [1,10] by RotomNG; reject obviously bogus values and
	// keep it within int32 for the protobuf field.
	if cfg.Weight < 1 || cfg.Weight > 10 {
		return errors.New("weight must be between 1 and 10")
	}
	if cfg.VersionCode < 0 || cfg.VersionCode > 1_000_000_000 {
		return errors.New("version-code must be between 0 and 1000000000")
	}
	return nil
}

// totalWorkers returns the number of worker connections the fleet will create.
func (cfg Config) totalWorkers() int {
	return cfg.Devices * cfg.WorkersPerDevice
}

// Connector orchestrates the simulated fleet of devices, workers, and controllers.
type Connector struct {
	cfg    Config
	logger *slog.Logger
}

// NewConnector creates a Connector for the given configuration.
func NewConnector(cfg Config, logger *slog.Logger) *Connector {
	return &Connector{cfg: cfg, logger: logger}
}

// Run starts every simulated connection and blocks until ctx is cancelled.
//
// Devices and their workers are started first so that RotomNG has workers
// available to assign before the controllers attempt to register. Each
// connection self-heals: if it drops it reconnects after ReconnectDelay until
// the context is cancelled.
func (c *Connector) Run(ctx context.Context) {
	c.logger.Info("starting mock fleet",
		slog.Int("devices", c.cfg.Devices),
		slog.Int("workers_per_device", c.cfg.WorkersPerDevice),
		slog.Int("controllers", c.cfg.Controllers),
		slog.String("device_endpoint", c.cfg.DeviceEndpoint),
		slog.String("controller_endpoint", c.cfg.ControllerEndpoint),
	)

	var wg sync.WaitGroup

	for d := range c.cfg.Devices {
		deviceID := fmt.Sprintf("%s-device-%03d", c.cfg.IDPrefix, d)
		device := newDevice(c.cfg, c.logger, deviceID)
		c.supervise(ctx, &wg, "device/"+deviceID, device.run)

		for w := range c.cfg.WorkersPerDevice {
			workerID := fmt.Sprintf("%s-worker-%03d", deviceID, w)
			worker := newWorker(c.cfg, c.logger, deviceID, workerID)
			c.supervise(ctx, &wg, "worker/"+workerID, worker.run)
		}
	}

	// Give devices and workers a moment to register before controllers try to
	// claim a worker, avoiding a burst of NO_WORKERS_AVAILABLE responses.
	if c.cfg.Controllers > 0 && c.cfg.Devices > 0 {
		c.sleep(ctx, c.cfg.StartupDelay)
	}

	for ctrl := range c.cfg.Controllers {
		controllerID := fmt.Sprintf("%s-controller-%03d", c.cfg.IDPrefix, ctrl)
		controller := newController(c.cfg, c.logger, controllerID)
		c.supervise(ctx, &wg, "controller/"+controllerID, controller.run)
	}

	<-ctx.Done()
	c.logger.Info("shutdown signal received; waiting for connections to close")
	wg.Wait()
}

// supervise runs fn in a goroutine, restarting it after ReconnectDelay whenever
// it returns, until ctx is cancelled.
func (c *Connector) supervise(ctx context.Context, wg *sync.WaitGroup, name string, fn func(context.Context) error) {
	wg.Go(func() {
		for ctx.Err() == nil {
			err := fn(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				c.logger.Debug("connection ended, will reconnect",
					slog.String("conn", name),
					slog.String("error", err.Error()),
					slog.Duration("retry_in", c.cfg.ReconnectDelay),
				)
			}
			c.sleep(ctx, c.cfg.ReconnectDelay)
		}
	})
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func (c *Connector) sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
