// Package controller implements the controller connection lifecycle and proxying.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

const (
	// ControllerReadTimeout is the read deadline for controller connections.
	// Controllers are responsible for keeping the connection alive by sending
	// ServerTime requests, etc.
	ControllerReadTimeout = 5 * time.Minute
)

// Controller represents a connected controller session managing a MITM worker.
type Controller struct {
	wsConn WSConn

	uuid              string
	id                string
	userAgent         string
	weight            int
	protoMajorVersion int
	protoMinorVersion int

	mitmWorker       MITMWorker
	mitmLoginRequest *protos.MitmRequest
	accountInfo      protos.AccountInfo

	disableWorkerStats bool

	onCloseOnce sync.Once
	onClose     func()
}

// NewController creates a new Controller with the given connection and configuration.
func NewController(
	wsConn WSConn,
	id string,
	mitmLoginRequest *protos.MitmRequest,
	mitmWorker MITMWorker,
	weight int,
	userAgent string,
	disableWorkerStats bool,
	protoMajorVersion int,
	protoMinorVersion int,
) *Controller {
	return &Controller{
		wsConn:           wsConn,
		id:               id,
		mitmLoginRequest: mitmLoginRequest,
		accountInfo: protos.GetAccountInfoFromLoginRequest(
			mitmLoginRequest.GetLoginRequest(),
		),
		mitmWorker:         mitmWorker,
		weight:             weight,
		userAgent:          userAgent,
		disableWorkerStats: disableWorkerStats,
		protoMajorVersion:  protoMajorVersion,
		protoMinorVersion:  protoMinorVersion,
	}
}

// SetUUID assigns a unique identifier to this controller.
func (controller *Controller) SetUUID(uuid string) {
	controller.uuid = uuid
}

// WriteAsync writes a message asynchronously to the controller's WebSocket connection.
func (controller *Controller) WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error {
	return controller.wsConn.WriteAsync(ctx, msgType, payload)
}

// IsZero reports whether the controller is nil.
func (controller *Controller) IsZero() bool {
	return controller == nil
}

// UUID returns the controller's unique identifier.
func (controller *Controller) UUID() string {
	return controller.uuid
}

// ID returns the controller's registration identifier.
func (controller *Controller) ID() string {
	return controller.id
}

// AccountInfo returns the account information for the controller's session.
func (controller *Controller) AccountInfo() protos.AccountInfo {
	return controller.accountInfo
}

// WorkerID returns the ID of the MITM worker assigned to this controller.
func (controller *Controller) WorkerID() string {
	if mitmWorker := controller.mitmWorker; !mitmWorker.IsZero() {
		return mitmWorker.ID()
	}
	return ""
}

// UserAgent returns the controller's user agent string.
func (controller *Controller) UserAgent() string {
	return controller.userAgent
}

// Weight returns the controller's weight for worker selection.
func (controller *Controller) Weight() int {
	return controller.weight
}

// ProtoMajorVersion returns the major protocol version supported by the controller.
func (controller *Controller) ProtoMajorVersion() int {
	return controller.protoMajorVersion
}

// ProtoMinorVersion returns the minor protocol version supported by the controller.
func (controller *Controller) ProtoMinorVersion() int {
	return controller.protoMinorVersion
}

// WebsocketStats returns the WebSocket connection statistics.
func (controller *Controller) WebsocketStats() ws.ConnStats {
	return controller.wsConn.GetStats()
}

// SetCloseHandler registers a function to be called when the controller closes.
func (controller *Controller) SetCloseHandler(fn func()) {
	controller.onClose = fn
}

// Flush flushes any buffered writes on the controller's WebSocket connection.
func (controller *Controller) Flush(ctx context.Context) error {
	return controller.wsConn.Flush(ctx)
}

// Close closes the controller connection with the given status code and reason.
func (controller *Controller) Close(code ws.StatusCode, text string) error {
	controller.onCloseOnce.Do(func() {
		if fn := controller.onClose; fn != nil {
			fn()
		}
	})
	return controller.wsConn.Close(code, text)
}

// Reader returns a reader for the next message from the controller. If no
// message is read within ControllerReadTimeout, the read fails so the caller
// can disconnect the controller. The deadline is applied via the context so
// that ws.Conn.Reader honors it (it resets the underlying read deadline from
// the context on every call).
func (controller *Controller) Reader(ctx context.Context) (ws.Reader, error) {
	readCtx, cancel := context.WithTimeout(ctx, ControllerReadTimeout)
	defer cancel()

	reader, err := controller.wsConn.Reader(readCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, fmt.Errorf("read timeout after %s: %w", ControllerReadTimeout, err)
		}
		return nil, err
	}
	return reader, nil
}

// WriteAsyncFromReader writes the contents of a WSReader asynchronously to the controller.
func (controller *Controller) WriteAsyncFromReader(ctx context.Context, reader ws.Reader) error {
	err := controller.wsConn.WriteAsyncFromReader(ctx, reader)
	if err != nil {
		return err
	}
	return nil
}

// Run starts the controller's main loop, proxying between the controller and its worker.
func (controller *Controller) Run(ctx context.Context) {
	worker := controller.mitmWorker
	defer worker.Close(ws.StatusNormalClosure, "controller done")

	logger := logging.LoggerFromContext(ctx).With(
		slog.String("controller_id", controller.id),
		slog.String("session", controller.accountInfo.String()),
		slog.String("worker_id", worker.ID()),
	)
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.LogAttrs(ctx, slog.LevelInfo, "controller running")

	// connection manager will have already read the mitm request
	mitmLoginRequest := controller.mitmLoginRequest
	controller.mitmLoginRequest = nil // don't need this anymore

	worker.ProxyController(ctx, controller, controller.disableWorkerStats, mitmLoginRequest)
}
