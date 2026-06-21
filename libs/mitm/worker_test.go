package mitm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/tracking"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// --- mock WorkerWSConn ---

type mockWorkerWSConn struct {
	mu         sync.Mutex
	stats      ws.ConnStats
	closeCode  ws.StatusCode
	closeText  string
	closeCalls int
	writeErr   error

	readerChan chan ws.Reader
	readerErr  error
}

func (m *mockWorkerWSConn) GetStats() ws.ConnStats {
	return m.stats
}

func (m *mockWorkerWSConn) Close(code ws.StatusCode, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCode = code
	m.closeText = text
	m.closeCalls++
	return nil
}

func (m *mockWorkerWSConn) Reader(ctx context.Context) (ws.Reader, error) {
	if m.readerErr != nil {
		return nil, m.readerErr
	}
	if m.readerChan != nil {
		select {
		case r := <-m.readerChan:
			return r, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("no reader configured")
}

func (m *mockWorkerWSConn) WriteAsyncFromReader(_ context.Context, _ ws.Reader) error {
	return m.writeErr
}

func (m *mockWorkerWSConn) WriteAsync(_ context.Context, _ ws.MessageType, _ []byte) error {
	return m.writeErr
}

// --- mock WorkerStatsCollector ---

type mockWorkerStatsCollector struct {
	mu               sync.Mutex
	requests         map[string]int
	droppedResponses int
	responses        int
	rpcRequests      int
	workersInUse     map[string]int
}

func newMockWorkerStatsCollector() *mockWorkerStatsCollector {
	return &mockWorkerStatsCollector{
		requests:     make(map[string]int),
		workersInUse: make(map[string]int),
	}
}

func (m *mockWorkerStatsCollector) IncrWorkerRequests(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[method]++
}

func (m *mockWorkerStatsCollector) IncrWorkerDroppedResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.droppedResponses++
}

func (m *mockWorkerStatsCollector) IncrWorkerResponses(_ time.Duration, _ string, _ string, _ string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses++
}

func (m *mockWorkerStatsCollector) IncrRPCRequests(_ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rpcRequests++
}

func (m *mockWorkerStatsCollector) IncrWorkersInUse(origin string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workersInUse[origin]++
}

func (m *mockWorkerStatsCollector) DecrWorkersInUse(origin string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workersInUse[origin]--
}

// --- mock WelcomeMessage ---

type mockWelcomeMessage struct {
	workerID    string
	origin      string
	deviceID    string
	versionName string
	useragent   string
	versionCode int32
	platform    protos.WelcomeMessage_Platform
}

func (m *mockWelcomeMessage) GetWorkerId() string                         { return m.workerID }
func (m *mockWelcomeMessage) GetOrigin() string                           { return m.origin }
func (m *mockWelcomeMessage) GetDeviceId() string                         { return m.deviceID }
func (m *mockWelcomeMessage) GetVersionName() string                      { return m.versionName }
func (m *mockWelcomeMessage) GetUseragent() string                        { return m.useragent }
func (m *mockWelcomeMessage) GetVersionCode() int32                       { return m.versionCode }
func (m *mockWelcomeMessage) GetPlatform() protos.WelcomeMessage_Platform { return m.platform }

// --- Tests ---

func TestNewWorker(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{
		workerID:    "worker-1",
		origin:      "origin-1",
		deviceID:    "device-1",
		versionName: "v2.0",
		useragent:   "test-agent",
		versionCode: 42,
	}

	worker := NewWorker(wsConn, wm, sc)

	if worker.ID() != "worker-1" {
		t.Errorf("Id() = %q, want %q", worker.ID(), "worker-1")
	}
	if worker.Origin() != "origin-1" {
		t.Errorf("Origin() = %q, want %q", worker.Origin(), "origin-1")
	}
	if worker.DeviceID() != "device-1" {
		t.Errorf("DeviceId() = %q, want %q", worker.DeviceID(), "device-1")
	}
	if worker.VersionName() != "v2.0" {
		t.Errorf("VersionName() = %q, want %q", worker.VersionName(), "v2.0")
	}
	if worker.UserAgent() != "test-agent" {
		t.Errorf("UserAgent() = %q, want %q", worker.UserAgent(), "test-agent")
	}
	if worker.VersionCode() != 42 {
		t.Errorf("VersionCode() = %d, want %d", worker.VersionCode(), 42)
	}
}

func TestWorker_IsZero(t *testing.T) {
	var nilWorker *Worker
	if !nilWorker.IsZero() {
		t.Error("nil worker should be zero")
	}

	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)
	if worker.IsZero() {
		t.Error("non-nil worker should not be zero")
	}
}

func TestWorker_ID_Nil(t *testing.T) {
	var nilWorker *Worker
	if nilWorker.ID() != "" {
		t.Errorf("nil worker Id() = %q, want empty string", nilWorker.ID())
	}
}

func TestWorker_WebsocketStats(t *testing.T) {
	now := time.Now()
	wsConn := &mockWorkerWSConn{
		stats: ws.ConnStats{
			ConnectedAt:      now,
			MessagesReceived: 10,
			BytesReceived:    1000,
		},
	}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	worker.SetPreviousWSConnStats(ws.ConnStats{
		MessagesReceived: 5,
		BytesReceived:    500,
	})

	session, total := worker.WebsocketStats()
	if session.MessagesReceived != 10 {
		t.Errorf("session.MessagesReceived = %d, want 10", session.MessagesReceived)
	}
	if total.MessagesReceived != 15 {
		t.Errorf("total.MessagesReceived = %d, want 15", total.MessagesReceived)
	}
}

func TestWorker_GetModeInfo_Unset(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	info := worker.GetModeInfo()
	if info.DisableStats != false {
		t.Errorf("DisableStats = %v, want false", info.DisableStats)
	}
	if info.Controller != nil {
		t.Error("Controller should be nil")
	}
}

func TestWorker_RequestManagement(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)
	worker.requestTracker = tracking.NewRequestTracker[uint32, RequestData]()

	// Store a request
	worker.requestTracker.Add(1, tracking.Request[RequestData]{StartTime: time.Now(), MethodName: "GET_MAP_OBJECTS"})
	worker.requestTracker.Add(2, tracking.Request[RequestData]{StartTime: time.Now(), MethodName: "ENCOUNTER"})

	// Verify get and delete
	req, ok := worker.requestTracker.Get(1)
	if !ok {
		t.Error("expected request 1 to be found")
	}
	if req.MethodName != "GET_MAP_OBJECTS" {
		t.Errorf("MethodName = %q, want %q", req.MethodName, "GET_MAP_OBJECTS")
	}

	// Should not find it again (Get removes the entry)
	_, ok = worker.requestTracker.Get(1)
	if ok {
		t.Error("request 1 should have been deleted")
	}

	// Get also removes, so request 2 should be found then gone
	_, ok = worker.requestTracker.Get(2)
	if !ok {
		t.Error("expected request 2 to be found")
	}
	_, ok = worker.requestTracker.Get(2)
	if ok {
		t.Error("request 2 should have been deleted")
	}
}

func TestWorker_DoneRequests(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)
	worker.requestTracker = tracking.NewRequestTracker[uint32, RequestData]()

	worker.requestTracker.Add(1, tracking.Request[RequestData]{StartTime: time.Now(), MethodName: "M1"})
	worker.requestTracker.Add(2, tracking.Request[RequestData]{StartTime: time.Now(), MethodName: "M2"})
	worker.requestTracker.Add(3, tracking.Request[RequestData]{StartTime: time.Now(), MethodName: "M3"})

	worker.requestTracker.Done(func(_ tracking.Request[RequestData]) {})

	for _, id := range []uint32{1, 2, 3} {
		if _, ok := worker.requestTracker.Get(id); ok {
			t.Errorf("request %d should have been flushed", id)
		}
	}
}

// --- mock Controller ---

type mockController struct {
	id         string
	closeCalls int
	isZero     bool
}

func (m *mockController) AccountInfo() protos.AccountInfo { return protos.AccountInfo{} }
func (m *mockController) Close(_ ws.StatusCode, _ string) error {
	m.closeCalls++
	return nil
}
func (m *mockController) ID() string             { return m.id }
func (m *mockController) IsZero() bool           { return m.isZero }
func (m *mockController) UserAgent() string      { return "test" }
func (m *mockController) UUID() string           { return "uuid" }
func (m *mockController) ProtoMajorVersion() int { return 1 }
func (m *mockController) ProtoMinorVersion() int { return 0 }
func (m *mockController) Reader(_ context.Context) (ws.Reader, error) {
	return nil, errors.New("not implemented")
}
func (m *mockController) WebsocketStats() ws.ConnStats { return ws.ConnStats{} }
func (m *mockController) Weight() int                  { return 1 }
func (m *mockController) WorkerID() string             { return "worker" }
func (m *mockController) WriteAsyncFromReader(_ context.Context, _ ws.Reader) error {
	return nil
}

func TestWorker_Close(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	// Set up a controller so Close doesn't panic on nil interface
	ctrl := &mockController{id: "ctrl-1", isZero: true}
	worker.modeInfo.Store(&WorkerModeInfo{
		DisableStats: true,
		Controller:   ctrl,
	})

	closeCalled := 0
	worker.SetCloseHandler(func() {
		closeCalled++
	})

	worker.Close(ws.StatusNormalClosure, "bye")
	worker.Close(ws.StatusNormalClosure, "bye again")

	if closeCalled != 1 {
		t.Errorf("close handler called %d times, want 1", closeCalled)
	}
	if wsConn.closeCalls != 2 {
		t.Errorf("wsConn.Close called %d times, want 2", wsConn.closeCalls)
	}
}

func TestWorker_Close_WithActiveController(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	ctrl := &mockController{id: "ctrl-1", isZero: false}
	worker.modeInfo.Store(&WorkerModeInfo{
		Controller: ctrl,
	})

	worker.Close(ws.StatusNormalClosure, "bye")

	if ctrl.closeCalls != 1 {
		t.Errorf("controller.Close called %d times, want 1", ctrl.closeCalls)
	}
}

func TestWorker_ReconnectSession(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	if worker.reconnectSession.Load() {
		t.Error("reconnectSession should be false initially")
	}

	worker.reconnectSession.Store(true)
	if !worker.reconnectSession.Load() {
		t.Error("reconnectSession should be true after Store(true)")
	}
}
