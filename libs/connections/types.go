package connections

import (
	"context"
	"time"

	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// SessionStatus represents the status of a controller registration response.
type SessionStatus = protos.RegisterControllerResponse_RegisterControllerResponseStatus

// Status represents the current status of all connections.
type Status[C Controller, W MITMWorker] struct {
	Devices     []*Device[W]
	Controllers []C
}

// MITMDevice is the device type from the mitm package.
type MITMDevice = mitm.Device

// MITMWorker defines the interface for a MITM worker connection.
type MITMWorker interface {
	Close(code ws.StatusCode, text string) error
	ID() string
	IsZero() bool
	DeviceID() string
	Origin() string
	GetModeInfo() mitm.WorkerModeInfo
	WebsocketStats() (total, session ws.ConnStats)
	SetCloseHandler(fn func())
	SetPreviousWSConnStats(stats ws.ConnStats)
	ProxyController(ctx context.Context, controller mitm.Controller, disableStats bool, initialRequest *protos.MitmRequest)
	WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error
}

// MITMWorkerConstraint is a type constraint requiring comparable and MITMWorker.
type MITMWorkerConstraint interface {
	comparable

	MITMWorker
}

// JobsRunner defines the interface for running jobs on devices.
type JobsRunner interface {
	AddFailedJobInstance(jobID string, deviceID string, result string) jobs.JobInstance
	RunJob(ctx context.Context, jobID string, deviceConn jobs.DeviceConn, timeout time.Duration) jobs.JobInstance
}

// ControllerWSConn is the websocket connection interface used for controller connections.
type ControllerWSConn interface {
	GetStats() ws.ConnStats
	Close(code ws.StatusCode, text string) error
	Reader(ctx context.Context) (ws.Reader, error)
	SetReadDeadline(t time.Time) error
	Flush(ctx context.Context) error
	WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error
	WriteAsyncFromReader(ctx context.Context, reader ws.Reader) error
}

// NewControllerFunc is a factory function for creating Controller instances.
type NewControllerFunc[C Controller] func(
	wsConn ControllerWSConn,
	id string,
	mitmLoginRequest *protos.MitmRequest,
	mitmWorker MITMWorker,
	weight int,
	userAgent string,
	disableWorkerStats bool,
	protoMajorVersion int,
	protoMinorVersion int,
) C

// Controller defines the interface for a controller connection.
type Controller interface {
	comparable

	mitm.Controller

	Flush(ctx context.Context) error
	SetCloseHandler(fn func())
	SetUUID(uuid string)
	WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error
}

// WorkerSelector defines the interface for selecting available workers.
type WorkerSelector[W MITMWorker] interface {
	GetAvailableWorker(weight int) (W, error)
	SetWorkerAvailable(worker W)
	SetWorkerUnavailable(worker W)
	EnableDevice(deviceID string)
	DisableDevice(deviceID string)
	RemoveDeadDevice(deviceID string)
	PruneWorkerIDsSeen(dur time.Duration)
}

// StatsCollector defines the interface for collecting connection statistics.
type StatsCollector interface {
	mitm.DeviceStatsCollector

	IncrDeviceRegistrationFails()
	IncrDeviceRegistrations(origin string)
	IncrDevicesConnected(origin string)
	DecrDevicesConnected(origin string)
	IncrDevicesTotal(origin string)
	DecrDevicesTotal(origin string, count int)

	IncrWorkerRegistrations(origin string)
}
