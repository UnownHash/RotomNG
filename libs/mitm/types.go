package mitm

import (
	"context"
	"time"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// Controller defines the interface for a controller connection that communicates with workers.
type Controller interface {
	AccountInfo() protos.AccountInfo
	Close(code ws.StatusCode, text string) error
	ID() string
	IsZero() bool
	UserAgent() string
	UUID() string
	ProtoMajorVersion() int
	ProtoMinorVersion() int
	Reader(ctx context.Context) (ws.Reader, error)
	WebsocketStats() ws.ConnStats
	Weight() int
	WorkerID() string
	WriteAsyncFromReader(ctx context.Context, reader ws.Reader) error
}

// WorkerWelcomeMessage defines the interface for a worker's initial welcome message.
type WorkerWelcomeMessage interface {
	GetWorkerId() string
	GetOrigin() string
	GetDeviceId() string
	GetVersionName() string
	GetUseragent() string
	GetVersionCode() int32
	GetPlatform() protos.WelcomeMessage_Platform
}

// WorkerStatsCollector defines the interface for collecting worker-level metrics.
type WorkerStatsCollector interface {
	IncrWorkerRequests(method string)
	IncrWorkerDroppedResponses()
	IncrWorkerResponses(duration time.Duration, method string, status string, errStr string)
	IncrRPCRequests(duration time.Duration)
	IncrWorkersInUse(origin string)
	DecrWorkersInUse(origin string)
}

// WorkerWSConn defines the WebSocket connection interface for worker communication.
type WorkerWSConn interface {
	GetStats() ws.ConnStats
	Close(code ws.StatusCode, text string) error
	Reader(ctx context.Context) (ws.Reader, error)
	WriteAsyncFromReader(ctx context.Context, reader ws.Reader) error
	WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error
}

// DeviceStatsCollector defines the interface for collecting device-level metrics.
type DeviceStatsCollector interface {
	// Device memory metrics
	SetDeviceMemoryFree(origin string, value float64)
	SetDeviceMemoryMITM(origin string, value float64)
	SetDeviceMemoryStart(origin string, value float64)

	// Device command metrics
	IncrDeviceCommandExecuted(origin string, command string)
	IncrDeviceCommandSuccess(origin string, command string)
	IncrDeviceCommandError(origin string, command string)
}

// DeviceWSConn defines the WebSocket connection interface for device communication.
type DeviceWSConn interface {
	GetStats() ws.ConnStats
	Close(code ws.StatusCode, text string) error
	Reader(ctx context.Context) (ws.Reader, error)
	ReadJSON(ctx context.Context, objptr any) error
	WriteJSONAsync(ctx context.Context, obj any) error
}
