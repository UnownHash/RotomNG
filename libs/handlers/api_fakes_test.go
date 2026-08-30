package handlers

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/stats"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// The APIHandler is generic over its controller and worker types, so the tests
// instantiate it with fakes rather than the real websocket-backed types. That
// keeps the HTTP surface -- which is what these tests are about -- reachable
// without standing up connections, while still running against a real
// ConnectionManager, selector, and jobs manager.

// fakeWorker satisfies connections.MITMWorker, api.MITMWorker, and the
// selector's worker constraint.
type fakeWorker struct {
	id       string
	deviceID string
	origin   string

	closed       atomic.Bool
	closeHandler atomic.Pointer[func()]
}

func newFakeWorker(id, deviceID, origin string) *fakeWorker {
	return &fakeWorker{id: id, deviceID: deviceID, origin: origin}
}

func (w *fakeWorker) ID() string       { return w.id }
func (w *fakeWorker) DeviceID() string { return w.deviceID }
func (w *fakeWorker) Origin() string   { return w.origin }
func (w *fakeWorker) IsZero() bool     { return w == nil }

func (w *fakeWorker) Close(ws.StatusCode, string) error {
	w.closed.Store(true)
	if handler := w.closeHandler.Load(); handler != nil {
		(*handler)()
	}
	return nil
}

func (w *fakeWorker) SetCloseHandler(fn func())           { w.closeHandler.Store(&fn) }
func (w *fakeWorker) SetPreviousWSConnStats(ws.ConnStats) {}
func (w *fakeWorker) GetModeInfo() mitm.WorkerModeInfo    { return mitm.WorkerModeInfo{} }
func (w *fakeWorker) VersionCode() int32                  { return 1 }
func (w *fakeWorker) VersionName() string                 { return "fake" }
func (w *fakeWorker) UserAgent() string                   { return "fake-worker" }
func (w *fakeWorker) Platform() mitm.WorkerPlatform       { return mitm.WorkerPlatform(0) }
func (w *fakeWorker) WriteAsync(context.Context, ws.MessageType, []byte) error {
	return nil
}

func (w *fakeWorker) WebsocketStats() (session, total ws.ConnStats) {
	return ws.ConnStats{}, ws.ConnStats{}
}

func (w *fakeWorker) GetRequestStats() stats.CountDurationWindows[uint64] {
	return stats.CountDurationWindows[uint64]{}
}

func (w *fakeWorker) ProxyController(context.Context, mitm.Controller, bool, *protos.MitmRequest) {
}

// fakeController satisfies handlers.Controller: api.Controller,
// connections.Controller, and Run.
type fakeController struct {
	id       string
	workerID string
	weight   int

	uuid         atomic.Pointer[string]
	closed       atomic.Bool
	closeCode    atomic.Int64
	closeHandler atomic.Pointer[func()]
}

func newFakeController(id, workerID string, weight int) *fakeController {
	return &fakeController{id: id, workerID: workerID, weight: weight}
}

func (c *fakeController) ID() string       { return c.id }
func (c *fakeController) WorkerID() string { return c.workerID }
func (c *fakeController) Weight() int      { return c.weight }
func (c *fakeController) IsZero() bool     { return c == nil }
func (c *fakeController) UserAgent() string {
	return "fake-controller"
}
func (c *fakeController) ProtoMajorVersion() int { return 1 }
func (c *fakeController) ProtoMinorVersion() int { return 0 }

func (c *fakeController) UUID() string {
	if uuid := c.uuid.Load(); uuid != nil {
		return *uuid
	}
	return ""
}

func (c *fakeController) SetUUID(uuid string) { c.uuid.Store(&uuid) }

func (c *fakeController) AccountInfo() protos.AccountInfo { return protos.AccountInfo{} }

func (c *fakeController) Close(code ws.StatusCode, _ string) error {
	c.closed.Store(true)
	c.closeCode.Store(int64(code))
	if handler := c.closeHandler.Load(); handler != nil {
		(*handler)()
	}
	return nil
}

func (c *fakeController) SetCloseHandler(fn func())    { c.closeHandler.Store(&fn) }
func (c *fakeController) WebsocketStats() ws.ConnStats { return ws.ConnStats{} }
func (c *fakeController) Flush(context.Context) error  { return nil }
func (c *fakeController) Run(context.Context)          {}
func (c *fakeController) WriteAsync(context.Context, ws.MessageType, []byte) error {
	return nil
}
func (c *fakeController) WriteAsyncFromReader(context.Context, ws.Reader) error { return nil }

func (c *fakeController) Reader(ctx context.Context) (ws.Reader, error) {
	// Nothing ever reads from a controller in these tests; blocking until the
	// context is done is the honest answer for a connection with no traffic.
	<-ctx.Done()
	return nil, ctx.Err()
}

// fakeWSReader is a ws.Reader over a fixed payload.
type fakeWSReader struct {
	*bytes.Reader

	payload []byte
}

func newFakeWSReader(payload []byte) *fakeWSReader {
	return &fakeWSReader{Reader: bytes.NewReader(payload), payload: payload}
}

func (r *fakeWSReader) Bytes() []byte               { return r.payload }
func (r *fakeWSReader) MessageType() ws.MessageType { return ws.MessageBinary }
func (r *fakeWSReader) Len() int                    { return len(r.payload) }
func (r *fakeWSReader) Done()                       {}

// fakeControllerWSConn satisfies connections.ControllerWSConn, serving one
// canned message: the registration request the manager reads on connect.
type fakeControllerWSConn struct {
	firstMessage []byte
	read         atomic.Bool
}

func (c *fakeControllerWSConn) GetStats() ws.ConnStats            { return ws.ConnStats{} }
func (c *fakeControllerWSConn) Close(ws.StatusCode, string) error { return nil }
func (c *fakeControllerWSConn) SetReadDeadline(time.Time) error   { return nil }
func (c *fakeControllerWSConn) Flush(context.Context) error       { return nil }

func (c *fakeControllerWSConn) WriteAsync(context.Context, ws.MessageType, []byte) error {
	return nil
}
func (c *fakeControllerWSConn) WriteAsyncFromReader(context.Context, ws.Reader) error { return nil }

func (c *fakeControllerWSConn) Reader(ctx context.Context) (ws.Reader, error) {
	if c.read.Swap(true) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return newFakeWSReader(c.firstMessage), nil
}

// noopConnStats satisfies connections.StatsCollector.
type noopConnStats struct{}

func (noopConnStats) SetDeviceMemoryFree(string, float64)      {}
func (noopConnStats) SetDeviceMemoryMITM(string, float64)      {}
func (noopConnStats) SetDeviceMemoryStart(string, float64)     {}
func (noopConnStats) IncrDeviceCommandExecuted(string, string) {}
func (noopConnStats) IncrDeviceCommandSuccess(string, string)  {}
func (noopConnStats) IncrDeviceCommandError(string, string)    {}
func (noopConnStats) IncrDeviceRegistrationFails()             {}
func (noopConnStats) IncrDeviceRegistrations(string)           {}
func (noopConnStats) IncrDevicesConnected(string)              {}
func (noopConnStats) DecrDevicesConnected(string)              {}
func (noopConnStats) IncrDevicesTotal(string)                  {}
func (noopConnStats) DecrDevicesTotal(string, int)             {}
func (noopConnStats) IncrWorkerRegistrations(string)           {}
