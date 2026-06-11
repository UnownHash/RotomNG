package api

import (
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/stats"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// Compile-time check that mitm.Worker satisfies MITMWorker.
var _ MITMWorker = (*mitm.Worker)(nil)

// Controller defines the interface for controller operations used by the API layer.
type Controller interface {
	mitm.Controller
}

// MITMDevice defines the interface for device operations used by the API layer.
type MITMDevice[W MITMWorker] interface {
	ID() string
	Origin() string
	Version() string
	PublicIP() string
	IsConnected() bool
	IsSelectionEnabled() bool
	WebsocketStats() (session, total ws.ConnStats)
	GetLastMemoryUsage() *mitm.DeviceMemory
	Workers() []W
}

// MITMWorker defines the interface for worker operations used by the API layer.
type MITMWorker interface {
	ID() string
	DeviceID() string
	Origin() string
	VersionCode() int32
	VersionName() string
	UserAgent() string
	Platform() mitm.WorkerPlatform
	GetModeInfo() mitm.WorkerModeInfo
	WebsocketStats() (session, total ws.ConnStats)
	GetRequestStats() stats.CountDurationWindows[uint64]
}
