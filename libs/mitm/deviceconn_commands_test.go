package mitm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

// simulateCommandReply waits for a command to be registered in the DeviceConn's
// messages map and sends a reply with the given status and body.
// The returned cancel function must be deferred to prevent goroutine leaks.
func simulateCommandReply(dc *DeviceConn, status int, body any) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(2 * time.Millisecond)
			dc.messagesMu.RLock()
			var msgID int64
			found := false
			for id := range dc.messages {
				msgID = id
				found = true
				break
			}
			dc.messagesMu.RUnlock()
			if found {
				bodyBytes, _ := json.Marshal(body) //nolint:errchkjson // test helper
				dc.processCommandReply(context.Background(), DeviceCommandReply{
					ID:     msgID,
					Status: status,
					Body:   json.RawMessage(bodyBytes),
				})
				return
			}
		}
	}()
	return cancel
}

// simulateCommandReplyWithRawBody is like simulateCommandReply but sends a raw JSON body.
func simulateCommandReplyWithRawBody(dc *DeviceConn, status int, rawBody json.RawMessage) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			time.Sleep(2 * time.Millisecond)
			dc.messagesMu.RLock()
			var msgID int64
			found := false
			for id := range dc.messages {
				msgID = id
				found = true
				break
			}
			dc.messagesMu.RUnlock()
			if found {
				dc.processCommandReply(context.Background(), DeviceCommandReply{
					ID:     msgID,
					Status: status,
					Body:   rawBody,
				})
				return
			}
		}
	}()
	return cancel
}

func newCommandableDeviceConn() (*DeviceConn, *mockDeviceStatsCollector) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)
	dc.canRunCommands.Store(true)
	return dc, sc
}

// --- GetMemoryUsage ---

func TestDeviceConn_GetMemoryUsage_Success(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	memResp := DeviceMemory{Free: 1000, Mitm: 500, Start: 2000}
	defer simulateCommandReply(dc, 200, memResp)()

	mem, err := dc.GetMemoryUsage(context.Background())
	if err != nil {
		t.Fatalf("GetMemoryUsage() error = %v", err)
	}
	if mem.Free != 1000 {
		t.Errorf("Free = %d, want 1000", mem.Free)
	}
	if mem.Mitm != 500 {
		t.Errorf("Mitm = %d, want 500", mem.Mitm)
	}
	if mem.Start != 2000 {
		t.Errorf("Start = %d, want 2000", mem.Start)
	}
	if mem.Time.IsZero() {
		t.Error("Time should be set")
	}

	// Verify last memory was stored
	stored := dc.lastMemory.Load()
	if stored == nil || stored.Free != 1000 {
		t.Error("lastMemory should be updated")
	}

	// Verify stats
	sc.mu.Lock()
	if sc.commandsExecuted["origin1"]["getMemoryUsage"] != 1 {
		t.Error("expected command execution to be tracked")
	}
	if sc.commandsSuccess["origin1"]["getMemoryUsage"] != 1 {
		t.Error("expected command success to be tracked")
	}
	if sc.memoryFree["origin1"] != 1000 {
		t.Error("expected memory free gauge to be set")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_GetMemoryUsage_Non200(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	defer simulateCommandReply(dc, 500, nil)()

	_, err := dc.GetMemoryUsage(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}

	sc.mu.Lock()
	if sc.commandsError["origin1"]["getMemoryUsage"] != 1 {
		t.Error("expected command error to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_GetMemoryUsage_BadJSON(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	defer simulateCommandReplyWithRawBody(dc, 200, json.RawMessage(`"not an object"`))()

	_, err := dc.GetMemoryUsage(context.Background())
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}

	sc.mu.Lock()
	if sc.commandsError["origin1"]["getMemoryUsage"] != 1 {
		t.Error("expected command error to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_GetMemoryUsage_CommandError(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	// Don't simulate any reply - let it timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := dc.GetMemoryUsage(ctx)
	if err == nil {
		t.Fatal("expected error")
	}

	sc.mu.Lock()
	if sc.commandsError["origin1"]["getMemoryUsage"] != 1 {
		t.Error("expected command error to be tracked")
	}
	sc.mu.Unlock()
}

// --- GetScreenSize ---

func TestDeviceConn_GetScreenSize_Success(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	defer simulateCommandReply(dc, 200, ScreenSize{Width: 1080, Height: 1920})()

	ss, err := dc.GetScreenSize(context.Background())
	if err != nil {
		t.Fatalf("GetScreenSize() error = %v", err)
	}
	if ss.Width != 1080 || ss.Height != 1920 {
		t.Errorf("ScreenSize = %dx%d, want 1080x1920", ss.Width, ss.Height)
	}

	sc.mu.Lock()
	if sc.commandsSuccess["origin1"]["getScreenSize"] != 1 {
		t.Error("expected command success to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_GetScreenSize_Non200(t *testing.T) {
	dc, _ := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 500, nil)()

	_, err := dc.GetScreenSize(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestDeviceConn_GetScreenSize_BadJSON(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	defer simulateCommandReplyWithRawBody(dc, 200, json.RawMessage(`"not valid"`))()

	_, err := dc.GetScreenSize(context.Background())
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestDeviceConn_GetScreenSize_CommandError(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := dc.GetScreenSize(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- RestartApp ---

func TestDeviceConn_RestartApp_Success(t *testing.T) {
	dc, sc := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 200, nil)()

	err := dc.RestartApp(context.Background())
	if err != nil {
		t.Fatalf("RestartApp() error = %v", err)
	}

	sc.mu.Lock()
	if sc.commandsSuccess["origin1"]["restartApp"] != 1 {
		t.Error("expected command success to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_RestartApp_Non200(t *testing.T) {
	dc, _ := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 500, nil)()

	err := dc.RestartApp(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestDeviceConn_RestartApp_CommandError(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := dc.RestartApp(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Reboot ---

func TestDeviceConn_Reboot_Success(t *testing.T) {
	dc, sc := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 200, nil)()

	err := dc.Reboot(context.Background())
	if err != nil {
		t.Fatalf("Reboot() error = %v", err)
	}

	sc.mu.Lock()
	if sc.commandsSuccess["origin1"]["reboot"] != 1 {
		t.Error("expected command success to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_Reboot_Non200(t *testing.T) {
	dc, _ := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 500, nil)()

	err := dc.Reboot(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestDeviceConn_Reboot_CommandError(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := dc.Reboot(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- GetLogcat ---

func TestDeviceConn_GetLogcat_Success(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	zipData := []byte("fake zip content")
	encoded := base64.StdEncoding.EncodeToString(zipData)
	defer simulateCommandReply(dc, 200, LogcatResponse{ZipData: encoded})()

	data, err := dc.GetLogcat(context.Background())
	if err != nil {
		t.Fatalf("GetLogcat() error = %v", err)
	}
	if string(data) != "fake zip content" {
		t.Errorf("data = %q, want %q", string(data), "fake zip content")
	}

	sc.mu.Lock()
	if sc.commandsSuccess["origin1"]["getLogcat"] != 1 {
		t.Error("expected command success to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_GetLogcat_Non200(t *testing.T) {
	dc, _ := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 500, nil)()

	_, err := dc.GetLogcat(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestDeviceConn_GetLogcat_BadJSON(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	defer simulateCommandReplyWithRawBody(dc, 200, json.RawMessage(`"not a logcat response"`))()

	_, err := dc.GetLogcat(context.Background())
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestDeviceConn_GetLogcat_BadBase64(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	defer simulateCommandReply(dc, 200, LogcatResponse{ZipData: "!!!not-base64!!!"})()

	_, err := dc.GetLogcat(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDeviceConn_GetLogcat_CommandError(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := dc.GetLogcat(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- RunJob ---

func TestDeviceConn_RunJob_Success(t *testing.T) {
	dc, sc := newCommandableDeviceConn()

	defer simulateCommandReply(dc, 200, map[string]string{"commandResult": "ok"})()

	resp, err := dc.RunJob(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("RunJob() error = %v", err)
	}
	if resp.CommandResult != "ok" {
		t.Errorf("CommandResult = %q, want %q", resp.CommandResult, "ok")
	}

	sc.mu.Lock()
	if sc.commandsSuccess["origin1"]["runJob"] != 1 {
		t.Error("expected command success to be tracked")
	}
	sc.mu.Unlock()
}

func TestDeviceConn_RunJob_Non200(t *testing.T) {
	dc, _ := newCommandableDeviceConn()
	defer simulateCommandReply(dc, 500, nil)()

	_, err := dc.RunJob(context.Background(), "cmd")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestDeviceConn_RunJob_BadJSON(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	defer simulateCommandReplyWithRawBody(dc, 200, json.RawMessage(`"not a job response"`))()

	_, err := dc.RunJob(context.Background(), "cmd")
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestDeviceConn_RunJob_CommandError(t *testing.T) {
	dc, _ := newCommandableDeviceConn()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := dc.RunJob(ctx, "cmd")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ExecuteCommand write error ---

func TestDeviceConn_ExecuteCommand_WriteError(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	// Override WriteJSONAsync to return error
	origWriteJSONAsync := wsConn.WriteJSONAsync
	_ = origWriteJSONAsync
	// We need a custom wsConn that errors on write
	errWSConn := &mockDeviceWSConnWithWriteErr{
		mockDeviceWSConn: mockDeviceWSConn{},
	}
	dc.wsConn = errWSConn

	dc.canRunCommands.Store(true)
	_, err := dc.executeCommand(context.Background(), "test", nil, time.Second)
	if err == nil {
		t.Fatal("expected error on write failure")
	}
}

type mockDeviceWSConnWithWriteErr struct {
	mockDeviceWSConn
}

func (m *mockDeviceWSConnWithWriteErr) WriteJSONAsync(_ context.Context, _ any) error {
	return errDeviceCommandsUnavailable // just need any error
}

// --- FlexibleString ---

func TestFlexibleString_UnmarshalJSON_Number(t *testing.T) {
	var s FlexibleString
	err := s.UnmarshalJSON([]byte("42"))
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if string(s) != "42" {
		t.Errorf("got %q, want %q", string(s), "42")
	}
}

func TestFlexibleString_UnmarshalJSON_String(t *testing.T) {
	var s FlexibleString
	err := s.UnmarshalJSON([]byte(`"hello"`))
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if string(s) != "hello" {
		t.Errorf("got %q, want %q", string(s), "hello")
	}
}

func TestFlexibleString_UnmarshalJSON_Empty(t *testing.T) {
	var s FlexibleString
	err := s.UnmarshalJSON([]byte{})
	if err == nil {
		t.Fatal("expected error for empty bytes")
	}
}

func TestFlexibleString_UnmarshalJSON_InvalidNumber(t *testing.T) {
	var s FlexibleString
	err := s.UnmarshalJSON([]byte("abc"))
	if err == nil {
		t.Fatal("expected error for invalid number")
	}
}

func TestFlexibleString_UnmarshalJSON_InvalidString(t *testing.T) {
	var s FlexibleString
	err := s.UnmarshalJSON([]byte(`"unterminated`))
	if err == nil {
		t.Fatal("expected error for invalid string")
	}
}
