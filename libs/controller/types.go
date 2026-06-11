package controller

import (
	"context"
	"time"

	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// SessionStatus represents the status of a controller registration response.
type SessionStatus = protos.RegisterControllerResponse_RegisterControllerResponseStatus

// Mode represents the current operating mode of a controller.
type Mode uint8

// Controller operating modes.
const (
	ModeWaiting Mode = iota
	ModeProxy

	maxModeIndex
)

var controllerModeStrings = []string{"waiting", "proxy"}

func (m Mode) String() string {
	if m >= maxModeIndex {
		return "invalid"
	}
	return controllerModeStrings[m]
}

// MITMWorker defines the interface for a MITM worker used by a controller.
type MITMWorker interface {
	Close(code ws.StatusCode, text string) error
	ID() string
	IsZero() bool
	ProxyController(ctx context.Context, controller mitm.Controller, disableStats bool, initialRequest *protos.MitmRequest)
}

// WSConn defines the WebSocket connection interface for controllers.
type WSConn interface {
	GetStats() ws.ConnStats
	Close(code ws.StatusCode, text string) error
	Reader(ctx context.Context) (ws.Reader, error)
	SetReadDeadline(t time.Time) error
	Flush(ctx context.Context) error
	WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error
	WriteAsyncFromReader(ctx context.Context, reader ws.Reader) error
}
