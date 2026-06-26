package testutil

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestControllerPingLoopNoRace guards against the data races that occurred when
// the controller handler's managed ping loop ran concurrently with connection
// registration and teardown:
//
//   - the ping loop set up the websocket read deadline / pong handler while
//     registration was still reading from (and possibly closing) the same conn;
//   - the ping loop's wg.Go raced the wg.Wait performed by Close when the
//     controller was closed from the connection manager.
//
// Both surface only under -race. Connecting many controllers concurrently —
// some succeeding (worker available, ping loop starts, then closes) and some
// failing (no worker, registration closes the conn) — maximizes the overlap.
func TestControllerPingLoopNoRace(t *testing.T) {
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

	// A single device+worker means a few controllers register successfully
	// (exercising the ping loop and the manager-driven Close) while the rest hit
	// the no-worker path (exercising the registration Close).
	fd := NewFakeDevice(WithDeviceID("ping-race-device"))
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

	var wg sync.WaitGroup
	for i := range 100 {
		version := V1
		if i%2 == 0 {
			version = V2
		}
		wg.Add(1)
		go func(v ProtocolVersion) {
			defer wg.Done()
			fc := NewFakeController(WithProtocolVersion(v))
			if err := fc.Connect(ctx, te.ControllerAddr); err == nil {
				fc.Close()
			}
		}(version)
	}
	wg.Wait()
}
