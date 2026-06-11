package testutil

import (
	"context"
	"testing"
	"time"
)

func TestFakeWorker_Options(t *testing.T) {
	// Test that NewFakeWorker with no options has sensible defaults
	fw := NewFakeWorker()
	if fw.WorkerID() == "" {
		t.Fatal("expected non-empty default workerID")
	}
	// deviceID defaults to empty (must be set via option or NewWorker helper)
	if fw.DeviceID() != "" {
		t.Fatalf("expected empty default deviceID, got %q", fw.DeviceID())
	}

	// Test WithWorkerID
	fw2 := NewFakeWorker(WithWorkerID("w-1"))
	if fw2.WorkerID() != "w-1" {
		t.Fatalf("expected workerID 'w-1', got %q", fw2.WorkerID())
	}

	// Test WithWorkerDeviceID
	fw3 := NewFakeWorker(WithWorkerDeviceID("d-1"))
	if fw3.DeviceID() != "d-1" {
		t.Fatalf("expected deviceID 'd-1', got %q", fw3.DeviceID())
	}

	// Test Connected() is false before Connect
	if fw.Connected() {
		t.Fatal("expected Connected() to be false before Connect()")
	}
}

func TestFakeWorker_Connect(t *testing.T) {
	ctx := context.Background()

	te, err := NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}

	if err := te.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer te.Stop()

	if err := te.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	// Must connect a FakeDevice first (worker references device_id)
	fd := NewFakeDevice(WithDeviceID("test-device-for-worker"))
	if err := fd.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect device: %v", err)
	}
	defer fd.Close()

	// Connect a FakeWorker under that device using NewWorker helper
	fw := fd.NewWorker()
	if err := fw.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect worker: %v", err)
	}
	defer fw.Close()

	if !fw.Connected() {
		t.Fatal("expected Connected() to be true")
	}
	if fw.DeviceID() != "test-device-for-worker" {
		t.Fatalf("expected worker deviceID to match device, got %q", fw.DeviceID())
	}
	if fw.WorkerID() == "" {
		t.Fatal("expected non-empty workerID from NewWorker()")
	}

	// Close cleanly
	if err := fw.Close(); err != nil {
		t.Fatalf("close worker: %v", err)
	}
	if fw.Connected() {
		t.Fatal("expected Connected() to be false after Close()")
	}
}
