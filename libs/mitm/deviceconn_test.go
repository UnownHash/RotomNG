package mitm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/ws"
)

// --- mock DeviceWSConn ---

type mockDeviceWSConn struct {
	mu      sync.Mutex
	written []any // messages passed to WriteJSONAsync
	stats   ws.ConnStats
	readErr error
	readMsg any // for ReadJSON

	readerFunc func(ctx context.Context) (io.Reader, error)
	closeCode  ws.StatusCode
	closeText  string
	closeCalls int
}

func (m *mockDeviceWSConn) GetStats() ws.ConnStats {
	return m.stats
}

func (m *mockDeviceWSConn) Close(code ws.StatusCode, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCode = code
	m.closeText = text
	m.closeCalls++
	return nil
}

func (m *mockDeviceWSConn) Reader(ctx context.Context) (ws.Reader, error) {
	if m.readerFunc != nil {
		r, err := m.readerFunc(ctx)
		if err != nil {
			return nil, err
		}
		return &mockWSReader{Reader: r}, nil
	}
	return nil, errors.New("no reader configured")
}

func (m *mockDeviceWSConn) ReadJSON(_ context.Context, objptr any) error {
	if m.readErr != nil {
		return m.readErr
	}
	data, err := json.Marshal(m.readMsg)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, objptr)
}

func (m *mockDeviceWSConn) WriteJSONAsync(_ context.Context, obj any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, obj)
	return nil
}

// --- mock WSReader ---

type mockWSReader struct {
	io.Reader

	done bool
}

func (m *mockWSReader) Bytes() []byte {
	if br, ok := m.Reader.(*bytes.Reader); ok {
		buf := make([]byte, br.Len())
		n, _ := br.Read(buf)
		// Reset reader position
		br.Seek(0, io.SeekStart)
		return buf[:n]
	}
	return nil
}

func (m *mockWSReader) MessageType() ws.MessageType { return ws.MessageText }
func (m *mockWSReader) Len() int                    { return 0 }
func (m *mockWSReader) Done()                       { m.done = true }

// --- mock DeviceStatsCollector ---

type mockDeviceStatsCollector struct {
	mu               sync.Mutex
	memoryFree       map[string]float64
	memoryMitm       map[string]float64
	memoryStart      map[string]float64
	commandsExecuted map[string]map[string]int
	commandsSuccess  map[string]map[string]int
	commandsError    map[string]map[string]int
}

func newMockDeviceStatsCollector() *mockDeviceStatsCollector {
	return &mockDeviceStatsCollector{
		memoryFree:       make(map[string]float64),
		memoryMitm:       make(map[string]float64),
		memoryStart:      make(map[string]float64),
		commandsExecuted: make(map[string]map[string]int),
		commandsSuccess:  make(map[string]map[string]int),
		commandsError:    make(map[string]map[string]int),
	}
}

func (m *mockDeviceStatsCollector) SetDeviceMemoryFree(origin string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memoryFree[origin] = value
}

func (m *mockDeviceStatsCollector) SetDeviceMemoryMITM(origin string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memoryMitm[origin] = value
}

func (m *mockDeviceStatsCollector) SetDeviceMemoryStart(origin string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memoryStart[origin] = value
}

func (m *mockDeviceStatsCollector) IncrDeviceCommandExecuted(origin string, command string) {
	m.incrMap(m.commandsExecuted, origin, command)
}

func (m *mockDeviceStatsCollector) IncrDeviceCommandSuccess(origin string, command string) {
	m.incrMap(m.commandsSuccess, origin, command)
}

func (m *mockDeviceStatsCollector) IncrDeviceCommandError(origin string, command string) {
	m.incrMap(m.commandsError, origin, command)
}

func (m *mockDeviceStatsCollector) incrMap(target map[string]map[string]int, origin, command string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := target[origin]; !ok {
		target[origin] = make(map[string]int)
	}
	target[origin][command]++
}

// --- Tests ---

func TestNewDeviceConn(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()

	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	if dc.ID() != "dev1" {
		t.Errorf("Id() = %q, want %q", dc.ID(), "dev1")
	}
	if dc.Origin() != "origin1" {
		t.Errorf("Origin() = %q, want %q", dc.Origin(), "origin1")
	}
	if dc.Version() != "1.0" {
		t.Errorf("Version() = %q, want %q", dc.Version(), "1.0")
	}
	if dc.PublicIP() != "1.2.3.4" {
		t.Errorf("PublicIP() = %q, want %q", dc.PublicIP(), "1.2.3.4")
	}
	if dc.CanRunCommands() {
		t.Error("CanRunCommands() should be false before Run()")
	}
}

func TestNewDeviceConn_NilStatsCollector(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}

	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, nil, lastMemory)

	// Should use NoOpStatsCollector when nil is passed
	if dc.statsCollector == nil {
		t.Error("statsCollector should not be nil")
	}
}

func TestDeviceConn_ProcessCommandReply(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	// Register a pending message
	replyChan := make(chan DeviceCommandReply, 1)
	dc.messagesMu.Lock()
	dc.messages[42] = replyChan
	dc.messagesMu.Unlock()

	reply := DeviceCommandReply{ID: 42, Status: 200, Body: json.RawMessage(`{"ok":true}`)}
	dc.processCommandReply(context.Background(), reply)

	select {
	case got := <-replyChan:
		if got.ID != 42 {
			t.Errorf("reply Id = %d, want 42", got.ID)
		}
		if got.Status != 200 {
			t.Errorf("reply Status = %d, want 200", got.Status)
		}
	default:
		t.Error("expected reply on channel")
	}
}

func TestDeviceConn_ProcessCommandReply_UnknownId(_ *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	reply := DeviceCommandReply{ID: 999, Status: 200}
	dc.processCommandReply(context.Background(), reply)
}

func TestDeviceConn_ExecuteCommand(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	// Simulate a reply coming in via processCommandReply (the real code path)
	go func() {
		// Wait for the message to be registered
		for {
			time.Sleep(5 * time.Millisecond)
			dc.messagesMu.RLock()
			_, ok := dc.messages[1]
			dc.messagesMu.RUnlock()
			if ok {
				break
			}
		}
		// Use processCommandReply which handles channel lifecycle correctly
		dc.processCommandReply(context.Background(), DeviceCommandReply{ID: 1, Status: 200, Body: json.RawMessage(`{}`)})
	}()

	reply, err := dc.executeCommand(context.Background(), "test", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}
	if reply.Status != 200 {
		t.Errorf("reply.Status = %d, want 200", reply.Status)
	}
}

func TestDeviceConn_ExecuteCommand_Timeout(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	_, err := dc.executeCommand(context.Background(), "test", nil, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error should mention timeout, got: %v", err)
	}
}

func TestDeviceConn_ExecuteCommand_ContextCanceled(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := dc.executeCommand(ctx, "test", nil, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestDeviceConn_GetMemoryUsage_CommandsUnavailable(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	// canRunCommands is false by default
	_, err := dc.GetMemoryUsage(context.Background())
	if !IsErrDeviceCommandsUnavailable(err) {
		t.Errorf("expected errDeviceCommandsUnavailable, got: %v", err)
	}
}

func TestDeviceConn_GetScreenSize_CommandsUnavailable(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	_, err := dc.GetScreenSize(context.Background())
	if !IsErrDeviceCommandsUnavailable(err) {
		t.Errorf("expected errDeviceCommandsUnavailable, got: %v", err)
	}
}

func TestDeviceConn_RestartApp_CommandsUnavailable(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	err := dc.RestartApp(context.Background())
	if !IsErrDeviceCommandsUnavailable(err) {
		t.Errorf("expected errDeviceCommandsUnavailable, got: %v", err)
	}
}

func TestDeviceConn_Reboot_CommandsUnavailable(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	err := dc.Reboot(context.Background())
	if !IsErrDeviceCommandsUnavailable(err) {
		t.Errorf("expected errDeviceCommandsUnavailable, got: %v", err)
	}
}

func TestDeviceConn_GetLogcat_CommandsUnavailable(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	_, err := dc.GetLogcat(context.Background())
	if !IsErrDeviceCommandsUnavailable(err) {
		t.Errorf("expected errDeviceCommandsUnavailable, got: %v", err)
	}
}

func TestDeviceConn_RunJob_CommandsUnavailable(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	_, err := dc.RunJob(context.Background(), "test-command")
	if !IsErrDeviceCommandsUnavailable(err) {
		t.Errorf("expected errDeviceCommandsUnavailable, got: %v", err)
	}
}

func TestDeviceConn_ReadInitMessage(t *testing.T) {
	initMsg := DeviceControlInitMessage{
		DeviceID: "dev1",
		Version:  "2.0",
		Origin:   "test-origin",
		PublicIP: "10.0.0.1",
	}
	wsConn := &mockDeviceWSConn{readMsg: initMsg}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	got, err := dc.ReadInitMessage(context.Background())
	if err != nil {
		t.Fatalf("ReadInitMessage() error = %v", err)
	}
	if got.DeviceID != "dev1" {
		t.Errorf("DeviceId = %q, want %q", got.DeviceID, "dev1")
	}
	if string(got.Version) != "2.0" {
		t.Errorf("Version = %q, want %q", got.Version, "2.0")
	}
}

func TestDeviceConn_ReadInitMessage_Error(t *testing.T) {
	wsConn := &mockDeviceWSConn{readErr: errors.New("read error")}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	_, err := dc.ReadInitMessage(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeviceConn_Close(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	closeCalled := false
	dc.SetCloseHandler(func() {
		closeCalled = true
	})

	err := dc.Close(ws.StatusNormalClosure, "done")
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !closeCalled {
		t.Error("close handler should have been called")
	}
	if wsConn.closeCode != ws.StatusNormalClosure {
		t.Errorf("close code = %d, want %d", wsConn.closeCode, ws.StatusNormalClosure)
	}
}

func TestDeviceConn_Close_OnlyOnce(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	callCount := 0
	dc.SetCloseHandler(func() {
		callCount++
	})

	dc.Close(ws.StatusNormalClosure, "done")
	dc.Close(ws.StatusNormalClosure, "done again")

	if callCount != 1 {
		t.Errorf("close handler called %d times, want 1", callCount)
	}
}

func TestDeviceConn_GetConnStats(t *testing.T) {
	now := time.Now()
	wsConn := &mockDeviceWSConn{
		stats: ws.ConnStats{
			ConnectedAt:      now,
			MessagesReceived: 10,
			BytesReceived:    1000,
			MessagesSent:     5,
			BytesSent:        500,
		},
	}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	stats := dc.GetConnStats()
	if stats.MessagesReceived != 10 {
		t.Errorf("MessagesReceived = %d, want 10", stats.MessagesReceived)
	}
	if stats.BytesSent != 500 {
		t.Errorf("BytesSent = %d, want 500", stats.BytesSent)
	}
}

func TestDeviceConn_ProcessWebsocketMessage(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	// Register a pending message
	replyChan := make(chan DeviceCommandReply, 1)
	dc.messagesMu.Lock()
	dc.messages[1] = replyChan
	dc.messagesMu.Unlock()

	msg := `{"id":1,"status":200,"body":{"result":"ok"}}`
	reader := bytes.NewReader([]byte(msg))

	err := dc.processWebsocketMessage(context.Background(), reader)
	if err != nil {
		t.Fatalf("processWebsocketMessage() error = %v", err)
	}

	select {
	case reply := <-replyChan:
		if reply.Status != 200 {
			t.Errorf("reply.Status = %d, want 200", reply.Status)
		}
	default:
		t.Error("expected reply on channel")
	}
}

func TestDeviceConn_ProcessWebsocketMessage_InvalidJSON(t *testing.T) {
	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev1", "origin1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	reader := bytes.NewReader([]byte("not json"))
	err := dc.processWebsocketMessage(context.Background(), reader)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestReadDeviceControlInitMessage(t *testing.T) {
	initMsg := DeviceControlInitMessage{
		DeviceID: "dev1",
		Version:  "3.0",
		Origin:   "test",
		PublicIP: "1.1.1.1",
	}
	wsConn := &mockDeviceWSConn{readMsg: initMsg}

	got, err := ReadDeviceControlInitMessage(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("ReadDeviceControlInitMessage() error = %v", err)
	}
	if got.DeviceID != "dev1" {
		t.Errorf("DeviceId = %q, want %q", got.DeviceID, "dev1")
	}
}

func TestReadDeviceControlInitMessage_Error(t *testing.T) {
	wsConn := &mockDeviceWSConn{readErr: errors.New("ws error")}

	_, err := ReadDeviceControlInitMessage(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeviceMonitorSettings_Validate(t *testing.T) {
	s := DeviceMonitorSettings{Interval: time.Minute}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestGetDeviceMonitorDefaultSettings(t *testing.T) {
	s := GetDeviceMonitorDefaultSettings()
	if s.Interval != DefaultDeviceMonitorInterval {
		t.Errorf("Interval = %v, want %v", s.Interval, DefaultDeviceMonitorInterval)
	}
}

func TestDeviceMonitorConfig_Init(t *testing.T) {
	cfg := &DeviceMonitorConfig{}
	err := cfg.Init(DeviceMonitorSettings{Interval: 30 * time.Second})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	s := cfg.GetSettings()
	if s.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want %v", s.Interval, 30*time.Second)
	}
}
