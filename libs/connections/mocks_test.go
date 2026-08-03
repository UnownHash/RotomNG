package connections

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// --- Mock Controller ---

type mockController struct {
	id                 string
	uuid               string
	workerID           string
	weight             int
	userAgent          string
	protoMajor         int
	protoMinor         int
	closeHandler       func()
	closeCalled        bool
	closeCode          ws.StatusCode
	closeText          string
	flushErr           error
	mu                 sync.Mutex
	disableWorkerStats bool
}

func (c *mockController) AccountInfo() protos.AccountInfo {
	return protos.AccountInfo{}
}

func (c *mockController) Close(code ws.StatusCode, text string) error {
	c.mu.Lock()
	c.closeCalled = true
	c.closeCode = code
	c.closeText = text
	closeHandler := c.closeHandler
	c.mu.Unlock()
	if closeHandler != nil {
		closeHandler()
	}
	return nil
}

func (c *mockController) ID() string             { return c.id }
func (c *mockController) IsZero() bool           { return c == nil || c.id == "" }
func (c *mockController) UserAgent() string      { return c.userAgent }
func (c *mockController) UUID() string           { return c.uuid }
func (c *mockController) ProtoMajorVersion() int { return c.protoMajor }
func (c *mockController) ProtoMinorVersion() int { return c.protoMinor }
func (c *mockController) Reader(_ context.Context) (ws.Reader, error) {
	return nil, errors.New("not implemented")
}
func (c *mockController) WebsocketStats() ws.ConnStats { return ws.ConnStats{} }
func (c *mockController) Weight() int                  { return c.weight }
func (c *mockController) WorkerID() string             { return c.workerID }
func (c *mockController) WriteAsyncFromReader(_ context.Context, _ ws.Reader) error {
	return nil
}

func (c *mockController) Flush(_ context.Context) error { return c.flushErr }
func (c *mockController) SetCloseHandler(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeHandler = fn
}
func (c *mockController) SetUUID(uuid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uuid = uuid
}
func (c *mockController) WriteAsync(_ context.Context, _ ws.MessageType, _ []byte) error {
	return nil
}

// --- Mock Worker ---

type mockWorker struct {
	id           string
	deviceID     string
	origin       string
	isZero       bool
	closeHandler func()
	closeCalled  bool
	closeCode    ws.StatusCode
	closeText    string
	modeInfo     mitm.WorkerModeInfo
	wsStats      ws.ConnStats
	prevStats    ws.ConnStats
	mu           sync.Mutex
}

func (w *mockWorker) Close(code ws.StatusCode, text string) error {
	w.mu.Lock()
	w.closeCalled = true
	w.closeCode = code
	w.closeText = text
	closeHandler := w.closeHandler
	w.mu.Unlock()
	if closeHandler != nil {
		closeHandler()
	}
	return nil
}

func (w *mockWorker) ID() string       { return w.id }
func (w *mockWorker) IsZero() bool     { return w == nil || w.isZero }
func (w *mockWorker) DeviceID() string { return w.deviceID }
func (w *mockWorker) Origin() string   { return w.origin }
func (w *mockWorker) GetModeInfo() mitm.WorkerModeInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.modeInfo
}
func (w *mockWorker) WebsocketStats() (session, total ws.ConnStats) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Mirror mitm.Worker: session is the live connection's stats, total is
	// previous sessions plus the live session.
	total = w.prevStats
	total.Add(w.wsStats)
	return w.wsStats, total
}
func (w *mockWorker) SetCloseHandler(fn func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closeHandler = fn
}
func (w *mockWorker) SetPreviousWSConnStats(stats ws.ConnStats) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.prevStats = stats
}
func (w *mockWorker) ProxyController(_ context.Context, _ mitm.Controller, _ bool, _ *protos.MitmRequest) {
}
func (w *mockWorker) WriteAsync(_ context.Context, _ ws.MessageType, _ []byte) error {
	return nil
}

// --- Mock Worker Selector ---

type mockWorkerSelector struct {
	workers            []*mockWorker
	workerIdx          int
	getAvailableErr    error
	availableWorkers   []*mockWorker
	unavailableWorkers []*mockWorker
	enabledDevices     []string
	disabledDevices    []string
	removedDevices     []string
	pruneCalled        bool
	prunePanic         bool
	mu                 sync.Mutex
}

func (s *mockWorkerSelector) GetAvailableWorker(_ int) (*mockWorker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getAvailableErr != nil {
		return nil, s.getAvailableErr
	}
	if s.workerIdx >= len(s.workers) {
		return nil, errors.New("no workers available")
	}
	w := s.workers[s.workerIdx]
	s.workerIdx++
	return w, nil
}

func (s *mockWorkerSelector) SetWorkerAvailable(worker *mockWorker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.availableWorkers = append(s.availableWorkers, worker)
}

func (s *mockWorkerSelector) SetWorkerUnavailable(worker *mockWorker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailableWorkers = append(s.unavailableWorkers, worker)
}

func (s *mockWorkerSelector) EnableDevice(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabledDevices = append(s.enabledDevices, deviceID)
}

func (s *mockWorkerSelector) DisableDevice(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disabledDevices = append(s.disabledDevices, deviceID)
}

func (s *mockWorkerSelector) RemoveDeadDevice(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removedDevices = append(s.removedDevices, deviceID)
}

func (s *mockWorkerSelector) PruneWorkerIDsSeen(_ time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prunePanic {
		panic("test panic in PruneWorkerIDsSeen")
	}
	s.pruneCalled = true
}

// --- Mock Stats Collector ---

type mockStatsCollector struct {
	NoOpStatsCollector

	mu                  sync.Mutex
	deviceRegFails      int
	deviceRegistrations map[string]int
	devicesConnected    map[string]int
	devicesDisconnected map[string]int
	devicesTotal        map[string]int
	devicesTotalDecr    map[string]int
	workerRegistrations map[string]int
}

func newMockStatsCollector() *mockStatsCollector {
	return &mockStatsCollector{
		NoOpStatsCollector:  NewNoOpStatsCollector(),
		deviceRegistrations: make(map[string]int),
		devicesConnected:    make(map[string]int),
		devicesDisconnected: make(map[string]int),
		devicesTotal:        make(map[string]int),
		devicesTotalDecr:    make(map[string]int),
		workerRegistrations: make(map[string]int),
	}
}

func (s *mockStatsCollector) IncrDeviceRegistrationFails() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceRegFails++
}

func (s *mockStatsCollector) IncrDeviceRegistrations(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceRegistrations[origin]++
}

func (s *mockStatsCollector) IncrDevicesConnected(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devicesConnected[origin]++
}

func (s *mockStatsCollector) DecrDevicesConnected(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devicesDisconnected[origin]++
}

func (s *mockStatsCollector) IncrDevicesTotal(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devicesTotal[origin]++
}

func (s *mockStatsCollector) DecrDevicesTotal(origin string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devicesTotalDecr[origin] += count
}

func (s *mockStatsCollector) IncrWorkerRegistrations(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workerRegistrations[origin]++
}

// --- Mock Jobs Runner ---

type mockJobsRunner struct {
	runJobResult    jobs.JobInstance
	failedJobResult jobs.JobInstance
	addFailedCalled bool
	runJobCalled    bool
	mu              sync.Mutex
}

func (j *mockJobsRunner) AddFailedJobInstance(_ string, _ string, _ string) jobs.JobInstance {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.addFailedCalled = true
	return j.failedJobResult
}

func (j *mockJobsRunner) RunJob(_ context.Context, _ string, _ jobs.DeviceConn, _ time.Duration) jobs.JobInstance {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.runJobCalled = true
	return j.runJobResult
}

// --- Mock Device WebSocket Connection ---

type mockDeviceWSConn struct {
	initMsg     mitm.DeviceControlInitMessage
	readErr     error
	closeCalled bool
	closeCode   ws.StatusCode
	closeText   string
	stats       ws.ConnStats
	mu          sync.Mutex
}

func (c *mockDeviceWSConn) GetStats() ws.ConnStats {
	return c.stats
}

func (c *mockDeviceWSConn) Close(code ws.StatusCode, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeCalled = true
	c.closeCode = code
	c.closeText = text
	return nil
}

func (c *mockDeviceWSConn) Reader(_ context.Context) (ws.Reader, error) {
	return nil, errors.New("not implemented")
}

func (c *mockDeviceWSConn) ReadJSON(_ context.Context, objptr any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return c.readErr
	}
	data, err := json.Marshal(c.initMsg)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, objptr)
}

func (c *mockDeviceWSConn) WriteJSONAsync(_ context.Context, _ any) error {
	return nil
}

// --- Mock Controller WebSocket Connection ---

type mockControllerWSConn struct {
	stats       ws.ConnStats
	closeErr    error
	closeCalled bool
	closeCode   ws.StatusCode
	closeText   string
	flushErr    error
	writeErr    error
	readers     []ws.Reader
	readerIdx   int
	readerErr   error
	mu          sync.Mutex
}

func (c *mockControllerWSConn) GetStats() ws.ConnStats { return c.stats }
func (c *mockControllerWSConn) Close(code ws.StatusCode, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeCalled = true
	c.closeCode = code
	c.closeText = text
	return c.closeErr
}
func (c *mockControllerWSConn) Reader(_ context.Context) (ws.Reader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readerErr != nil {
		return nil, c.readerErr
	}
	if c.readerIdx >= len(c.readers) {
		return nil, errors.New("no more readers")
	}
	r := c.readers[c.readerIdx]
	c.readerIdx++
	return r, nil
}
func (c *mockControllerWSConn) SetReadDeadline(_ time.Time) error { return nil }
func (c *mockControllerWSConn) Flush(_ context.Context) error     { return c.flushErr }
func (c *mockControllerWSConn) WriteAsync(_ context.Context, _ ws.MessageType, _ []byte) error {
	return c.writeErr
}
func (c *mockControllerWSConn) WriteAsyncFromReader(_ context.Context, _ ws.Reader) error {
	return nil
}

// --- Mock Reader ---

type mockReader struct {
	data    []byte
	msgType ws.MessageType
}

func (r *mockReader) Read(p []byte) (n int, err error) { return copy(p, r.data), nil }
func (r *mockReader) Bytes() []byte                    { return r.data }
func (r *mockReader) MessageType() ws.MessageType      { return r.msgType }
func (r *mockReader) Len() int                         { return len(r.data) }
func (r *mockReader) Done()                            {}
