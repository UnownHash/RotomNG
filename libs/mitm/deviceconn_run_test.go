package mitm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// runCtx returns a context with a discard logger for use in Run tests.
func runCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logging.NewDiscardLogger()
	ctx = logging.ContextWithLogger(ctx, logger)
	return ctx, cancel
}

func TestDeviceConn_Run_ProcessesMessages(t *testing.T) {
	msgChan := make(chan io.Reader, 5)

	wsConn := &mockDeviceWSConn{
		readerFunc: func(ctx context.Context) (io.Reader, error) {
			select {
			case r := <-msgChan:
				return r, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	monitorCfg := DeviceMonitorConfig{}
	if err := monitorCfg.Init(DeviceMonitorSettings{Interval: time.Hour}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx, cancel := runCtx()

	done := make(chan struct{})
	go func() {
		dc.Run(ctx, monitorCfg)
		close(done)
	}()

	// Wait for Run to start and set canRunCommands
	time.Sleep(50 * time.Millisecond)
	if !dc.CanRunCommands() {
		t.Error("CanRunCommands should be true while Run is active")
	}

	// Register a reply channel matching the ID we'll send
	const replyID int64 = 9999
	replyChan := make(chan DeviceCommandReply, 1)
	dc.messagesMu.Lock()
	dc.messages[replyID] = replyChan
	dc.messagesMu.Unlock()

	// Send a command reply message through the websocket
	reply := DeviceCommandReply{ID: replyID, Status: 200, Body: json.RawMessage(`{"ok":true}`)}
	replyBytes, _ := json.Marshal(reply) //nolint:errchkjson // test data
	msgChan <- bytes.NewReader(replyBytes)

	// Wait for reply
	select {
	case got := <-replyChan:
		if got.Status != 200 {
			t.Errorf("reply.Status = %d, want 200", got.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reply")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}

	if dc.CanRunCommands() {
		t.Error("CanRunCommands should be false after Run exits")
	}
}

func TestDeviceConn_Run_ReadError(t *testing.T) {
	wsConn := &mockDeviceWSConn{
		readerFunc: func(_ context.Context) (io.Reader, error) {
			return nil, errors.New("ws read error")
		},
	}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	monitorCfg := DeviceMonitorConfig{}
	monitorCfg.Init(DeviceMonitorSettings{Interval: 0})

	ctx, cancel := runCtx()
	defer cancel()

	done := make(chan struct{})
	go func() {
		dc.Run(ctx, monitorCfg)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after read error")
	}
}

func TestDeviceConn_Run_InvalidJSON(t *testing.T) {
	callCount := 0
	wsConn := &mockDeviceWSConn{
		readerFunc: func(ctx context.Context) (io.Reader, error) {
			callCount++
			if callCount == 1 {
				return bytes.NewReader([]byte("not json")), nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	monitorCfg := DeviceMonitorConfig{}
	monitorCfg.Init(DeviceMonitorSettings{Interval: 0})

	ctx, cancel := runCtx()
	defer cancel()

	done := make(chan struct{})
	go func() {
		dc.Run(ctx, monitorCfg)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after invalid JSON")
	}
}

func TestDeviceConn_CheckDevice_Success(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)
	dc.canRunCommands.Store(true)

	memResp := DeviceMemory{Free: 1000, Mitm: 500, Start: 2000}
	defer simulateCommandReply(dc, 200, memResp)()

	ctx, cancel := runCtx()
	defer cancel()
	dc.checkDevice(ctx, DeviceMonitorSettings{})

	stored := dc.lastMemory.Load()
	if stored == nil || stored.Free != 1000 {
		t.Error("checkDevice should update lastMemory on success")
	}
}

func TestDeviceConn_CheckDevice_Error(_ *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	ctx, cancel := runCtx()
	defer cancel()
	dc.checkDevice(ctx, DeviceMonitorSettings{})
	// Should not panic
}

func TestDeviceConn_RunMonitor_ContextCancel(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	monitorCfg := DeviceMonitorConfig{}
	monitorCfg.Init(DeviceMonitorSettings{Interval: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		dc.runMonitor(ctx, monitorCfg)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runMonitor did not exit after cancel")
	}
}

func TestDeviceConn_RunMonitor_SettingsNotify(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	monitorCfg := DeviceMonitorConfig{}
	monitorCfg.Init(DeviceMonitorSettings{Interval: time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		dc.runMonitor(ctx, monitorCfg)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	monitorCfg.PutSettings(DeviceMonitorSettings{Interval: time.Hour})
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runMonitor did not exit")
	}
}

func TestDeviceConn_RunMonitor_ZeroInterval(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	monitorCfg := DeviceMonitorConfig{}
	monitorCfg.Init(DeviceMonitorSettings{Interval: 0})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		dc.runMonitor(ctx, monitorCfg)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runMonitor did not exit")
	}
}

// --- Device.WebsocketStats with active connection ---

func TestDevice_WebsocketStats_WithConn(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	wsConn := &mockDeviceWSConn{
		stats: ws.ConnStats{
			MessagesReceived: 10,
			BytesReceived:    1000,
		},
	}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev-1", "origin-1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	d.SwapConn(dc)

	session, total := d.WebsocketStats()
	if session.MessagesReceived != 10 {
		t.Errorf("session.MessagesReceived = %d, want 10", session.MessagesReceived)
	}
	if total.MessagesReceived != 10 {
		t.Errorf("total.MessagesReceived = %d, want 10", total.MessagesReceived)
	}
}
