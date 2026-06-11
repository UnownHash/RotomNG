package testutil

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestEnv_StartStop(t *testing.T) {
	cfg, cleanup, err := NewTestConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	te, err := NewTestEnv()
	if err != nil {
		t.Fatal(err)
	}

	if err := te.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if err := te.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady() failed: %v", err)
	}

	if err := te.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// cfg is used only to suppress unused variable if needed
	_ = cfg
}

func TestEnv_WaitReady_AllPorts(t *testing.T) {
	te, err := NewTestEnv()
	if err != nil {
		t.Fatal(err)
	}

	if err := te.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() {
		if err := te.Stop(); err != nil {
			t.Errorf("Stop() failed: %v", err)
		}
	}()

	if err := te.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady() failed: %v", err)
	}

	// Verify all three ports accept connections
	for _, addr := range []string{te.DeviceAddr, te.ControllerAddr, te.HTTPAddr} {
		conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err != nil {
			t.Fatalf("failed to dial %s: %v", addr, err)
		}
		conn.Close()
	}
}

func TestEnv_Stop_Cleanup(t *testing.T) {
	te, err := NewTestEnv()
	if err != nil {
		t.Fatal(err)
	}

	if err := te.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if err := te.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady() failed: %v", err)
	}

	// Capture addresses and the temp dir path before stop
	deviceAddr := te.DeviceAddr
	controllerAddr := te.ControllerAddr
	httpAddr := te.HTTPAddr

	// Derive temp dir from the jobs path in config (created by NewTestConfig)
	tmpDir := te.Config.Jobs.Path

	if err := te.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// Verify temp dir no longer exists
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("expected temp dir %s to be removed after Stop(), but it still exists", tmpDir)
	}

	// Verify connections are refused
	for _, addr := range []string{deviceAddr, controllerAddr, httpAddr} {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			t.Fatalf("expected connection refused to %s after Stop(), but dial succeeded", addr)
		}
	}
}

func TestEnv_SequentialCycles(t *testing.T) {
	for i := range 2 {
		te, err := NewTestEnv()
		if err != nil {
			t.Fatalf("cycle %d: NewTestEnv() failed: %v", i, err)
		}

		if err := te.Start(); err != nil {
			t.Fatalf("cycle %d: Start() failed: %v", i, err)
		}

		if err := te.WaitReady(5 * time.Second); err != nil {
			t.Fatalf("cycle %d: WaitReady() failed: %v", i, err)
		}

		if err := te.Stop(); err != nil {
			t.Fatalf("cycle %d: Stop() failed: %v", i, err)
		}
	}
}

func TestEnv_WaitReady_Timeout(t *testing.T) {
	te, err := NewTestEnv()
	if err != nil {
		t.Fatal(err)
	}

	// Start the app so we have addresses to check, but use an impossibly short timeout
	if err := te.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer func() {
		// Stop even if WaitReady fails
		te.Stop()
	}()

	// Use a nanosecond timeout -- impossible for servers to be ready
	err = te.WaitReady(1 * time.Nanosecond)
	if err == nil {
		t.Fatal("expected WaitReady to return non-nil error with 1ns timeout")
	}
}
