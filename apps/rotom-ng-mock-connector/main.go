// Command rotom-ng-mock-connector connects fake devices, workers, and
// controllers to a running RotomNG instance using the real wire protocol.
//
// It is meant for local development and load/smoke testing: instead of mocking
// data inside the UI, it drives RotomNG with genuine WebSocket connections so
// the API and UI light up with live devices, workers, controllers, request
// stats, and proxied RPC traffic.
//
// Usage:
//
//	rotom-ng-mock-connector [flags]      run the simulated fleet
//	rotom-ng-mock-connector gen-compose  write a docker-compose.yml + config
//
// Run with -h for the full flag list.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Subcommand dispatch. The default (no subcommand) runs the fleet.
	if len(os.Args) > 1 && os.Args[1] == "gen-compose" {
		if err := runGenCompose(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "gen-compose failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runFleet(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runFleet parses the run flags and drives the simulated fleet until interrupted.
func runFleet(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)

	cfg := Config{}
	fs.IntVar(&cfg.Devices, "devices", 1, "number of devices to simulate")
	fs.IntVar(&cfg.WorkersPerDevice, "workers", 1, "number of MITM workers per device")
	fs.IntVar(&cfg.Controllers, "controllers", 1, "number of controllers to simulate")
	fs.StringVar(&cfg.DeviceEndpoint, "device-endpoint", "ws://localhost:7070",
		"base WebSocket endpoint for devices and workers (RotomNG device_listener)")
	fs.StringVar(&cfg.ControllerEndpoint, "controller-endpoint", "ws://localhost:7071",
		"WebSocket endpoint for controllers (RotomNG controller_listener)")
	fs.StringVar(&cfg.DeviceSecret, "device-secret", "", "X-Rotom-Secret for device/worker connections")
	fs.StringVar(&cfg.ControllerSecret, "controller-secret", "", "X-Rotom-Secret for controller connections")
	fs.StringVar(&cfg.Origin, "origin", "mock", "origin name reported by devices/workers")
	fs.StringVar(&cfg.IDPrefix, "id-prefix", "mock", "prefix for generated device/worker/controller IDs")
	fs.StringVar(&cfg.Version, "version", "1.0.0-mock", "version string reported by devices/workers")
	fs.IntVar(&cfg.VersionCode, "version-code", 1000, "numeric version code reported by workers")
	fs.IntVar(&cfg.Weight, "weight", 5, "controller weight (1-10) requested at registration")
	fs.DurationVar(&cfg.RPCInterval, "rpc-interval", 30*time.Second,
		"how often each controller sends an RPC request (must be < controller read timeout of 5m)")
	fs.DurationVar(&cfg.StartupDelay, "startup-delay", 2*time.Second,
		"delay between bringing up devices/workers and starting controllers")
	fs.DurationVar(&cfg.ReconnectDelay, "reconnect-delay", 3*time.Second,
		"delay before reconnecting a dropped connection")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "enable debug logging")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := NewConnector(cfg, logger)
	c.Run(ctx)

	logger.Info("shutdown complete")
	return nil
}
