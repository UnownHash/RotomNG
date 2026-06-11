package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/protos"
)

func TestFakeController_Options(t *testing.T) {
	// Test 1: NewFakeController with no options has sensible defaults
	fc := NewFakeController()
	if fc.ControllerID() == "" {
		t.Fatal("expected non-empty default controllerID")
	}
	if fc.Connected() {
		t.Fatal("expected Connected() to be false before Connect()")
	}

	// Test 2: WithControllerID sets the controller ID
	fc2 := NewFakeController(WithControllerID("custom-id"))
	if fc2.ControllerID() != "custom-id" {
		t.Fatalf("expected controllerID 'custom-id', got %q", fc2.ControllerID())
	}

	// Test 3: WithWeight sets the weight
	fc3 := NewFakeController(WithWeight(5))
	if fc3.cfg.weight != 5 {
		t.Fatalf("expected weight 5, got %d", fc3.cfg.weight)
	}

	// Test 4: WithProtocolVersion sets V1 protocol
	fc4 := NewFakeController(WithProtocolVersion(V1))
	if fc4.cfg.protocolVersion != V1 {
		t.Fatalf("expected V1 protocol, got %d", fc4.cfg.protocolVersion)
	}

	// Test 5: WithAuthSecret sets the auth secret
	fc5 := NewFakeController(WithAuthSecret("secret"))
	if fc5.cfg.authSecret != "secret" {
		t.Fatalf("expected auth secret 'secret', got %q", fc5.cfg.authSecret)
	}
}

func TestFakeController_V2_Connect(t *testing.T) {
	// Test 6: FakeController V2 Connect succeeds with FakeDevice+FakeWorker
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

	// Connect a device and worker first
	fd := NewFakeDevice(WithDeviceID("ctrl-test-device"))
	if err := fd.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect device: %v", err)
	}
	defer fd.Close()

	fw := fd.NewWorker()
	if err := fw.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect worker: %v", err)
	}
	defer fw.Close()

	// Give the server a moment to register the worker
	time.Sleep(100 * time.Millisecond)

	// Connect a V2 controller
	fc := NewFakeController(WithControllerID("v2-ctrl"))
	if err := fc.Connect(ctx, te.ControllerAddr); err != nil {
		t.Fatalf("V2 connect: %v", err)
	}
	defer fc.Close()

	if !fc.Connected() {
		t.Fatal("expected Connected() to be true after V2 Connect()")
	}
}

func TestFakeController_V2_NoWorkers(t *testing.T) {
	// Test 7: FakeController V2 Connect returns error when no workers available
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

	// No device/worker connected -- should fail
	fc := NewFakeController(WithControllerID("v2-ctrl-noworker"))
	err = fc.Connect(ctx, te.ControllerAddr)
	if err == nil {
		fc.Close()
		t.Fatal("expected error when no workers available, got nil")
	}
}

func TestFakeController_V1_Connect(t *testing.T) {
	// Test 8: FakeController V1 Connect succeeds with FakeDevice+FakeWorker
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

	// Connect a device and worker first
	fd := NewFakeDevice(WithDeviceID("v1-ctrl-test-device"))
	if err := fd.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect device: %v", err)
	}
	defer fd.Close()

	fw := fd.NewWorker()
	if err := fw.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect worker: %v", err)
	}
	defer fw.Close()

	time.Sleep(100 * time.Millisecond)

	// Connect a V1 controller
	fc := NewFakeController(WithControllerID("v1-ctrl"), WithProtocolVersion(V1))
	if err := fc.Connect(ctx, te.ControllerAddr); err != nil {
		t.Fatalf("V1 connect: %v", err)
	}
	defer fc.Close()

	if !fc.Connected() {
		t.Fatal("expected Connected() to be true after V1 Connect()")
	}
}

func TestFakeController_ConnectedLifecycle(t *testing.T) {
	// Test 9: Connected() returns true after Connect and false after Close
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

	fd := NewFakeDevice(WithDeviceID("lifecycle-ctrl-device"))
	if err := fd.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect device: %v", err)
	}
	defer fd.Close()

	fw := fd.NewWorker()
	if err := fw.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect worker: %v", err)
	}
	defer fw.Close()

	time.Sleep(100 * time.Millisecond)

	fc := NewFakeController(WithControllerID("lifecycle-ctrl"))
	if fc.Connected() {
		t.Fatal("expected Connected() false before Connect()")
	}

	if err := fc.Connect(ctx, te.ControllerAddr); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if !fc.Connected() {
		t.Fatal("expected Connected() true after Connect()")
	}

	if err := fc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if fc.Connected() {
		t.Fatal("expected Connected() false after Close()")
	}
}

func TestFakeController_SendRequest(t *testing.T) {
	// Test 10: SendRequest sends MitmRequest and receives matching MitmResponse
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

	fd := NewFakeDevice(WithDeviceID("sendreq-ctrl-device"))
	if err := fd.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect device: %v", err)
	}
	defer fd.Close()

	fw := fd.NewWorker()
	if err := fw.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("connect worker: %v", err)
	}
	defer fw.Close()

	time.Sleep(100 * time.Millisecond)

	fc := NewFakeController(WithControllerID("sendreq-ctrl"))
	if err := fc.Connect(ctx, te.ControllerAddr); err != nil {
		t.Fatalf("connect controller: %v", err)
	}
	defer fc.Close()

	// Send a request -- the FakeWorker's defaultRequestHandler returns SUCCESS
	req := &protos.MitmRequest{
		Method: protos.MitmRequest_RPC_REQUEST,
	}
	resp, err := fc.SendRequest(req)
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Status != protos.MitmResponse_SUCCESS {
		t.Fatalf("expected SUCCESS status, got %v", resp.Status)
	}
	if resp.Id != req.Id {
		t.Fatalf("expected response ID %d to match request ID %d", resp.Id, req.Id)
	}
}
