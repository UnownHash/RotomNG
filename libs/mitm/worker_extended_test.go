package mitm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/tracking"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// --- Simple getter tests ---

func TestWorker_Platform(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{
		workerID: "w1",
		platform: protos.WelcomeMessage_ANDROID,
	}
	worker := NewWorker(wsConn, wm, sc)

	if worker.Platform() != protos.WelcomeMessage_ANDROID {
		t.Errorf("Platform() = %v, want ANDROID", worker.Platform())
	}
}

func TestWorker_WriteAsync(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	err := worker.WriteAsync(context.Background(), ws.MessageBinary, []byte("test"))
	if err != nil {
		t.Fatalf("WriteAsync() error = %v", err)
	}
}

func TestWorker_WriteAsync_Error(t *testing.T) {
	wsConn := &mockWorkerWSConn{writeErr: errors.New("write failed")}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	err := worker.WriteAsync(context.Background(), ws.MessageBinary, []byte("test"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorker_Reader(t *testing.T) {
	readerChan := make(chan ws.Reader, 1)
	wsConn := &mockWorkerWSConn{readerChan: readerChan}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	expected := &mockWSReader2{data: []byte("hello")}
	readerChan <- expected

	got, err := worker.Reader(context.Background())
	if err != nil {
		t.Fatalf("Reader() error = %v", err)
	}
	if got != expected {
		t.Error("Reader() returned wrong reader")
	}
}

func TestWorker_Reader_Error(t *testing.T) {
	wsConn := &mockWorkerWSConn{readerErr: errors.New("read failed")}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	_, err := worker.Reader(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Time windowed stats ---

func TestWorker_GetRequestStats(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	s := worker.GetRequestStats()
	if len(s.Counts) != len(StatsWindows) {
		t.Fatalf("GetRequestStats().Counts returned %d windows, want %d", len(s.Counts), len(StatsWindows))
	}
	if len(s.Durations) != len(StatsWindows) {
		t.Fatalf("GetRequestStats().Durations returned %d windows, want %d", len(s.Durations), len(StatsWindows))
	}
	for i := range s.Counts {
		if s.Counts[i].Count() != 0 {
			t.Errorf("Counts window[%d] count = %d, want 0", i, s.Counts[i].Count())
		}
		if s.Durations[i].Count() != 0 {
			t.Errorf("Durations window[%d] count = %d, want 0", i, s.Durations[i].Count())
		}
	}
}

// --- getRequestMethodName ---

func TestWorker_GetRequestMethodName(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	req := &protos.MitmRequest{Method: protos.MitmRequest_LOGIN}
	name := worker.getRequestMethodName(req)
	if name == "" {
		t.Error("expected non-empty method name")
	}
}

// --- processResponseAndUpdateStats ---

func TestWorker_ProcessResponseAndUpdateStats(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	worker.requestTracker = tracking.NewRequestTracker[uint32, struct{}]()

	// Store a request first
	worker.requestTracker.Add(1, tracking.Request[struct{}]{StartTime: time.Now(), MethodName: "GET_MAP_OBJECTS"})

	// Create a response
	resp := &protos.MitmResponse{
		Id:     1,
		Status: protos.MitmResponse_SUCCESS,
	}
	respBytes, _ := proto.Marshal(resp)

	worker.processResponseAndUpdateStats(respBytes)

	// Verify stats were updated
	sc.mu.Lock()
	if sc.responses != 1 {
		t.Errorf("responses = %d, want 1", sc.responses)
	}
	sc.mu.Unlock()

	// Verify request was removed
	_, ok := worker.requestTracker.Get(1)
	if ok {
		t.Error("request should have been deleted after processing response")
	}
}

func TestWorker_ProcessResponseAndUpdateStats_RPC(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	worker.requestTracker = tracking.NewRequestTracker[uint32, struct{}]()
	worker.requestTracker.Add(1, tracking.Request[struct{}]{StartTime: time.Now(), MethodName: "RPC_REQUEST"})

	resp := &protos.MitmResponse{Id: 1, Status: protos.MitmResponse_SUCCESS}
	respBytes, _ := proto.Marshal(resp)

	worker.processResponseAndUpdateStats(respBytes)

	sc.mu.Lock()
	if sc.rpcRequests != 1 {
		t.Errorf("rpcRequests = %d, want 1", sc.rpcRequests)
	}
	sc.mu.Unlock()
}

func TestWorker_ProcessResponseAndUpdateStats_InvalidProto(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)
	worker.requestTracker = tracking.NewRequestTracker[uint32, struct{}]()

	// Should not panic on invalid protobuf
	worker.processResponseAndUpdateStats([]byte("not protobuf"))

	sc.mu.Lock()
	if sc.responses != 0 {
		t.Error("should not track stats for invalid protobuf")
	}
	sc.mu.Unlock()
}

func TestWorker_ProcessResponseAndUpdateStats_UnknownRequest(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)
	worker.requestTracker = tracking.NewRequestTracker[uint32, struct{}]()

	// Don't store any request - response should be for unknown ID
	resp := &protos.MitmResponse{Id: 999, Status: protos.MitmResponse_SUCCESS}
	respBytes, _ := proto.Marshal(resp)

	worker.processResponseAndUpdateStats(respBytes)

	sc.mu.Lock()
	if sc.responses != 0 {
		t.Error("should not track stats for unknown request")
	}
	sc.mu.Unlock()
}

// --- ProxyController ---

func TestWorker_ProxyController_Transparent(t *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1", origin: "origin1"}
	worker := NewWorker(wsConn, wm, sc)

	loginReq := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
	}

	ctrl := &mockProxyController{
		mockController: mockController{id: "ctrl1"},
		readers: []ws.Reader{
			&mockWSReader2{data: []byte("hello")},
		},
		readErrAfter: errors.New("done"),
	}

	worker.ProxyController(context.Background(), ctrl, true, loginReq)

	// Verify mode was set
	info := worker.GetModeInfo()
	if info.DisableStats != true {
		t.Errorf("DisableStats = %v, want true", info.DisableStats)
	}
	if info.Controller != ctrl {
		t.Error("Controller should be set")
	}

	// Verify stats
	sc.mu.Lock()
	if sc.workersInUse["origin1"] != 0 { // IncrWorkersInUse then DecrWorkersInUse = 0
		t.Errorf("workersInUse = %d, want 0 (incr then decr)", sc.workersInUse["origin1"])
	}
	sc.mu.Unlock()
}

func TestWorker_ProxyController_Inspect(_ *testing.T) {
	wsConn := &mockWorkerWSConn{}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1", origin: "origin1"}
	worker := NewWorker(wsConn, wm, sc)

	loginReq := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
	}

	// Create a second protobuf MitmRequest for the controller to send in the proxy loop
	req2 := &protos.MitmRequest{
		Id:     2,
		Method: protos.MitmRequest_RPC_REQUEST,
	}
	req2Bytes, _ := proto.Marshal(req2)

	ctrl := &mockProxyController{
		mockController: mockController{id: "ctrl1"},
		readers: []ws.Reader{
			&mockWSReader2{data: req2Bytes},
		},
		readErrAfter: errors.New("done"),
	}

	worker.ProxyController(context.Background(), ctrl, false, loginReq)

	// With stats enabled (DisableStats=false), both the login request and loop requests should have been stored.
	// (deleteAllRequests is called in Run's defer, not ProxyController)
}

func TestWorker_ProxyController_WriteError(_ *testing.T) {
	wsConn := &mockWorkerWSConn{writeErr: errors.New("write failed")}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1", origin: "origin1"}
	worker := NewWorker(wsConn, wm, sc)

	loginReq := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
	}

	ctrl := &mockProxyController{
		mockController: mockController{id: "ctrl1"},
		readers: []ws.Reader{
			&mockWSReader2{data: []byte("hello")},
		},
		readErrAfter: errors.New("done"),
	}

	// writeErr causes the initial login request write to fail, so ProxyController returns early
	worker.ProxyController(context.Background(), ctrl, true, loginReq)
}

// --- Worker.Run ---

func TestWorker_Run_ForwardsToController(t *testing.T) {
	readerChan := make(chan ws.Reader, 5)
	wsConn := &mockWorkerWSConn{readerChan: readerChan}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1", origin: "origin1"}
	worker := NewWorker(wsConn, wm, sc)

	ctrl := &mockProxyController{mockController: mockController{id: "ctrl1"}}
	worker.modeInfo.Store(&WorkerModeInfo{
		DisableStats: true,
		Controller:   ctrl,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Send a message then cancel
	readerChan <- &mockWSReader2{data: []byte("response data")}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	worker.Run(ctx)

	ctrl.mu.Lock()
	if ctrl.writeCalls != 1 {
		t.Errorf("writeCalls = %d, want 1", ctrl.writeCalls)
	}
	ctrl.mu.Unlock()
}

func TestWorker_Run_NoModeInfo(_ *testing.T) {
	readerChan := make(chan ws.Reader, 1)
	wsConn := &mockWorkerWSConn{readerChan: readerChan}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	// Don't set modeInfo - should exit with error after first message
	readerChan <- &mockWSReader2{data: []byte("data")}

	worker.Run(context.Background())
	// Should exit without panic
}

func TestWorker_Run_ReadError(_ *testing.T) {
	wsConn := &mockWorkerWSConn{readerErr: errors.New("read failed")}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	worker.Run(context.Background())
	// Should exit without panic
}

func TestWorker_Run_ContextCanceled(_ *testing.T) {
	wsConn := &mockWorkerWSConn{readerErr: nil}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker.Run(ctx)
	// Should exit immediately
}

func TestWorker_Run_InspectMode(t *testing.T) {
	readerChan := make(chan ws.Reader, 5)
	wsConn := &mockWorkerWSConn{readerChan: readerChan}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1", origin: "origin1"}
	worker := NewWorker(wsConn, wm, sc)

	ctrl := &mockProxyController{mockController: mockController{id: "ctrl1"}}
	worker.modeInfo.Store(&WorkerModeInfo{
		Controller: ctrl,
	})

	// Store a request so processResponseAndUpdateStats has something to find
	worker.requestTracker = tracking.NewRequestTracker[uint32, struct{}]()
	worker.requestTracker.Add(1, tracking.Request[struct{}]{StartTime: time.Now(), MethodName: "GET_MAP_OBJECTS"})

	// Create a valid MitmResponse
	resp := &protos.MitmResponse{Id: 1, Status: protos.MitmResponse_SUCCESS}
	respBytes, _ := proto.Marshal(resp)

	ctx, cancel := context.WithCancel(context.Background())

	readerChan <- &mockWSReader2{data: respBytes}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	worker.Run(ctx)

	sc.mu.Lock()
	if sc.responses != 1 {
		t.Errorf("responses = %d, want 1", sc.responses)
	}
	sc.mu.Unlock()
}

func TestWorker_Run_WriteToControllerError(_ *testing.T) {
	readerChan := make(chan ws.Reader, 1)
	wsConn := &mockWorkerWSConn{readerChan: readerChan}
	sc := newMockWorkerStatsCollector()
	wm := &mockWelcomeMessage{workerID: "w1"}
	worker := NewWorker(wsConn, wm, sc)

	ctrl := &mockProxyController{
		mockController: mockController{id: "ctrl1"},
		writeErr:       errors.New("write failed"),
	}
	worker.modeInfo.Store(&WorkerModeInfo{
		DisableStats: true,
		Controller:   ctrl,
	})

	readerChan <- &mockWSReader2{data: []byte("data")}

	worker.Run(context.Background())
	// Should exit after write error
}

// --- ReadWorkerWelcomeMessage ---

func TestReadWorkerWelcomeMessage_Success(t *testing.T) {
	welcomeMsg := &protos.WelcomeMessage{
		WorkerId:    "w1",
		Origin:      "origin1",
		DeviceId:    "d1",
		VersionName: "v2.0",
		Useragent:   "test-ua",
		VersionCode: 42,
	}
	welcomeBytes, _ := proto.Marshal(welcomeMsg)

	readerChan := make(chan ws.Reader, 1)
	readerChan <- &mockWSReader2{data: welcomeBytes}
	wsConn := &mockWorkerWSConn{readerChan: readerChan}

	got, err := ReadWorkerWelcomeMessage(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("ReadWorkerWelcomeMessage() error = %v", err)
	}
	if got.GetWorkerId() != "w1" {
		t.Errorf("WorkerId = %q, want %q", got.GetWorkerId(), "w1")
	}
	if got.GetOrigin() != "origin1" {
		t.Errorf("Origin = %q, want %q", got.GetOrigin(), "origin1")
	}
	if got.GetVersionCode() != 42 {
		t.Errorf("VersionCode = %d, want 42", got.GetVersionCode())
	}
}

func TestReadWorkerWelcomeMessage_ReadError(t *testing.T) {
	wsConn := &mockWorkerWSConn{readerErr: errors.New("read failed")}

	_, err := ReadWorkerWelcomeMessage(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadWorkerWelcomeMessage_InvalidProto(t *testing.T) {
	readerChan := make(chan ws.Reader, 1)
	readerChan <- &mockWSReader2{data: []byte("not protobuf")}
	wsConn := &mockWorkerWSConn{readerChan: readerChan}

	_, err := ReadWorkerWelcomeMessage(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for invalid protobuf")
	}
}

// --- Mock helpers ---

type mockWSReader2 struct {
	data    []byte
	doneFn  func()
	doneVal bool
}

func (r *mockWSReader2) Read(p []byte) (n int, err error) {
	return copy(p, r.data), io.EOF
}

func (r *mockWSReader2) Bytes() []byte               { return r.data }
func (r *mockWSReader2) MessageType() ws.MessageType { return ws.MessageBinary }
func (r *mockWSReader2) Len() int                    { return len(r.data) }
func (r *mockWSReader2) Done() {
	r.doneVal = true
	if r.doneFn != nil {
		r.doneFn()
	}
}

type mockProxyController struct {
	mockController

	mu           sync.Mutex
	readerChan   chan ws.Reader
	readers      []ws.Reader
	readerIdx    int
	readerErr    error
	readErrAfter error
	writeCalls   int
	writeErr     error
}

func (c *mockProxyController) Reader(ctx context.Context) (ws.Reader, error) {
	c.mu.Lock()
	if c.readerErr != nil {
		c.mu.Unlock()
		return nil, c.readerErr
	}
	c.mu.Unlock()

	if c.readerChan != nil {
		select {
		case r := <-c.readerChan:
			if r == nil {
				return nil, errors.New("nil reader")
			}
			return r, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readerIdx >= len(c.readers) {
		if c.readErrAfter != nil {
			return nil, c.readErrAfter
		}
		return nil, errors.New("no more readers")
	}
	r := c.readers[c.readerIdx]
	c.readerIdx++
	return r, nil
}

func (c *mockProxyController) WriteAsyncFromReader(_ context.Context, _ ws.Reader) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeCalls++
	return c.writeErr
}

func (c *mockProxyController) WebsocketStats() ws.ConnStats { return ws.ConnStats{} }

// ws import needed for the mock.
var _ = bytes.NewReader
