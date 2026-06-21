package mitm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/errorutil"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/stats"
	"github.com/UnownHash/RotomNG/libs/tracking"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// WorkerModeInfo holds the stats mode and controller assignment for a worker.
type WorkerModeInfo struct {
	DisableStats         bool
	Controller           Controller
	ControllerAssignedAt time.Time
}

// WorkerPlatform is a type alias for the platform reported in a worker's welcome message.
type WorkerPlatform = protos.WelcomeMessage_Platform

// RequestData holds the per-request data carried alongside a tracked MITM request.
type RequestData struct {
	// Methods holds the RPC method IDs for an RPC request. It is nil for a login request.
	Methods []int32
}

type requestTracker = tracking.RequestTracker[uint32, RequestData]

// Worker represents a connected MITM worker that proxies between controllers and devices.
type Worker struct {
	id          string
	origin      string
	deviceID    string
	versionName string
	userAgent   string
	versionCode int32
	platform    WorkerPlatform

	requestTracker *requestTracker

	wsConn              WorkerWSConn
	statsCollector      WorkerStatsCollector
	previousWSConnStats ws.ConnStats

	reconnectSession atomic.Bool

	modeInfo atomic.Pointer[WorkerModeInfo]

	onCloseOnce sync.Once
	onClose     func()

	// TimeWindowedStats: count+duration ring buffer, one lock for both metrics.
	requestStats *stats.CountDurationCollector[uint64]
}

// NewWorker creates a new Worker from a WebSocket connection and welcome message.
func NewWorker(wsConn WorkerWSConn, welcomeMsg WorkerWelcomeMessage, statsCollector WorkerStatsCollector) *Worker {
	return &Worker{
		id:             welcomeMsg.GetWorkerId(),
		origin:         welcomeMsg.GetOrigin(),
		deviceID:       welcomeMsg.GetDeviceId(),
		versionName:    welcomeMsg.GetVersionName(),
		userAgent:      welcomeMsg.GetUseragent(),
		versionCode:    welcomeMsg.GetVersionCode(),
		platform:       welcomeMsg.GetPlatform(),
		wsConn:         wsConn,
		statsCollector: statsCollector,

		requestStats: stats.NewCountDurationCollector[uint64](15*time.Minute, 2*time.Second),
	}
}

// IsZero reports whether the worker is nil.
func (worker *Worker) IsZero() bool {
	return worker == nil
}

// ID returns the worker's unique identifier.
func (worker *Worker) ID() string {
	if worker == nil {
		return ""
	}
	return worker.id
}

// DeviceID returns the ID of the device this worker is associated with.
func (worker *Worker) DeviceID() string {
	return worker.deviceID
}

// Origin returns the worker's origin identifier.
func (worker *Worker) Origin() string {
	return worker.origin
}

// VersionCode returns the worker's numeric version code.
func (worker *Worker) VersionCode() int32 {
	return worker.versionCode
}

// VersionName returns the worker's version string.
func (worker *Worker) VersionName() string {
	return worker.versionName
}

// UserAgent returns the worker's user agent string.
func (worker *Worker) UserAgent() string {
	return worker.userAgent
}

// Platform returns the worker's platform type.
func (worker *Worker) Platform() WorkerPlatform {
	return worker.platform
}

// WriteAsync writes a message asynchronously to the worker's WebSocket connection.
func (worker *Worker) WriteAsync(ctx context.Context, msgType ws.MessageType, payload []byte) error {
	return worker.wsConn.WriteAsync(ctx, msgType, payload)
}

// Reader returns the next message reader from the worker's WebSocket connection.
func (worker *Worker) Reader(ctx context.Context) (ws.Reader, error) {
	return worker.wsConn.Reader(ctx)
}

// WebsocketStats returns the current session and cumulative WebSocket statistics.
func (worker *Worker) WebsocketStats() (session, total ws.ConnStats) {
	session = worker.wsConn.GetStats()
	total = worker.previousWSConnStats
	total.Add(session)
	return
}

// GetModeInfo returns the worker's current mode information including proxy mode and controller.
func (worker *Worker) GetModeInfo() (info WorkerModeInfo) {
	if infoPtr := worker.modeInfo.Load(); infoPtr != nil {
		info = *infoPtr
	}
	return
}

// StatsWindows defines the standard time windows used for worker statistics.
var StatsWindows = []time.Duration{
	30 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
}

// GetRequestStats returns request count and duration (ms) stats for all standard
// windows (30s, 1m, 5m, 15m) in a single lock acquisition with a consistent timestamp.
func (worker *Worker) GetRequestStats() stats.CountDurationWindows[uint64] {
	return worker.requestStats.GetWindows(StatsWindows...)
}

// SetCloseHandler sets the function to be called when the worker is closed.
func (worker *Worker) SetCloseHandler(fn func()) {
	worker.onClose = fn
}

// Close shuts down the worker, invoking the close handler and closing the WebSocket.
func (worker *Worker) Close(code ws.StatusCode, text string) error {
	worker.onCloseOnce.Do(func() {
		if fn := worker.onClose; fn != nil {
			fn()
		}
	})
	if controller := worker.GetModeInfo().Controller; controller != nil && !controller.IsZero() {
		_ = controller.Close(errorutil.CloseCodeMITMWorkerDisconnected, "mitm worker connection being closed")
	}
	return worker.wsConn.Close(code, text)
}

// ProxyController proxies messages from the controller to the worker in the given mode.
// If mitmLoginRequest is non-nil, it is marshaled and written to the worker before
// entering the proxy loop. This ensures modeInfo is set before any data reaches
// the worker, so worker.Run() can forward responses back to the controller.
// In inspect mode, the initial request is also tracked so that when the worker
// processes the response in Run(), it is not treated as a drop.
func (worker *Worker) ProxyController(ctx context.Context, controller Controller, disableStats bool, mitmLoginRequest *protos.MitmRequest) {
	logger := logging.LoggerFromContext(ctx)

	var requestTracker *requestTracker

	if !disableStats {
		requestTracker = tracking.NewRequestTracker[uint32, RequestData]()
		if logger != nil {
			defer requestTracker.Done(func(request tracking.Request[RequestData]) {
				logger.LogAttrs(ctx, slog.LevelWarn, "mitm worker request ended while waiting a response",
					slog.String("worker_id", worker.id),
					slog.String("method_name", request.MethodName),
					slog.Any("methods", request.Data.Methods),
					slog.String("start_time", request.StartTime.Format(time.RFC3339Nano)),
					slog.Duration("age", time.Since(request.StartTime)),
				)
			})
		}
		worker.requestTracker = requestTracker
	}

	worker.modeInfo.Store(&WorkerModeInfo{
		DisableStats:         disableStats,
		Controller:           controller,
		ControllerAssignedAt: time.Now(),
	})

	defer worker.statsCollector.DecrWorkersInUse(worker.origin)
	worker.statsCollector.IncrWorkersInUse(worker.origin)

	payload, err := proto.Marshal(mitmLoginRequest)
	if err != nil {
		_ = controller.Close(errorutil.CloseCodeProtocolError, "failed to marshal initial request")
		if logger != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed to marshal initial request", slog.String("worker_id", worker.id), slog.String("error", err.Error()))
		}
		return
	}

	if requestTracker != nil {
		requestMethodName := worker.getRequestMethodName(mitmLoginRequest)
		requestTracker.Add(mitmLoginRequest.Id, tracking.Request[RequestData]{
			StartTime:  time.Now(),
			MethodName: requestMethodName,
			Data:       RequestData{Methods: getRequestMethods(mitmLoginRequest)},
		})
	}

	if err := worker.wsConn.WriteAsync(ctx, ws.MessageBinary, payload); err != nil {
		if !errors.Is(err, context.Canceled) {
			if logger != nil {
				logger.LogAttrs(ctx, slog.LevelError, "failed writing login request to worker", slog.String("worker_id", worker.id), slog.String("error", err.Error()))
			}
		}
		if ws.GetWebsocketCloseError(err) != nil {
			_ = controller.Close(errorutil.CloseCodeMITMWorkerDisconnected, "mitm worker went away")
		}
		return
	}

	for {
		reader, err := controller.Reader(ctx)
		if err != nil {
			ws.LogWebsocketReadError(ctx, err)
			return
		}

		if requestTracker != nil {
			var mitmRequestProto protos.MitmRequest

			err := proto.Unmarshal(reader.Bytes(), &mitmRequestProto)
			if err == nil {
				requestMethodName := worker.getRequestMethodName(&mitmRequestProto)
				requestTracker.Add(mitmRequestProto.Id, tracking.Request[RequestData]{
					StartTime:  time.Now(),
					MethodName: requestMethodName,
					Data:       RequestData{Methods: getRequestMethods(&mitmRequestProto)},
				})
			}
		}

		err = worker.wsConn.WriteAsyncFromReader(ctx, reader)
		if err != nil {
			reader.Done()
			if !errors.Is(err, context.Canceled) {
				if logger != nil {
					logger.LogAttrs(ctx, slog.LevelError, "error writing to worker", slog.String("worker_id", worker.id), slog.String("error", err.Error()))
				}
			}
			if ws.GetWebsocketCloseError(err) != nil {
				_ = controller.Close(errorutil.CloseCodeMITMWorkerDisconnected, "mitm worker went away")
			}
			return
		}
	}
}

// Run reads responses from the worker and forwards them to the assigned controller.
func (worker *Worker) Run(ctx context.Context) {
	var errRet error

	logger := logging.LoggerFromContext(ctx)

	var modeInfo *WorkerModeInfo

	defer func() {
		if logger != nil {
			if errRet != nil {
				logger.LogAttrs(ctx, slog.LevelInfo, "mitm worker Run() done", slog.String("error", errRet.Error()))
			} else {
				logger.LogAttrs(ctx, slog.LevelInfo, "mitm worker Run() done")
			}
		}
	}()

	for {
		if errRet = ctx.Err(); errRet != nil {
			return
		}

		reader, err := worker.wsConn.Reader(ctx)
		if err != nil {
			errRet = fmt.Errorf("failed to read mitm message: %w", err)
			return
		}

		if modeInfo == nil {
			modeInfo = worker.modeInfo.Load()
			if modeInfo == nil {
				errRet = errors.New("received worker message without request")
				return
			}
		}

		if worker.requestTracker != nil {
			worker.processResponseAndUpdateStats(reader.Bytes())
		}
		err = modeInfo.Controller.WriteAsyncFromReader(ctx, reader)
		if err != nil {
			reader.Done()
			if logger != nil && !errors.Is(err, context.Canceled) {
				logger.LogAttrs(ctx, slog.LevelError, "error writing to controller", slog.String("controller_id", modeInfo.Controller.ID()), slog.String("error", err.Error()))
			}
			errRet = fmt.Errorf("failed to write to controller: %w", err)
			return
		}
	}
}

// SetPreviousWSConnStats sets the accumulated WebSocket stats from previous sessions.
func (worker *Worker) SetPreviousWSConnStats(stats ws.ConnStats) {
	worker.previousWSConnStats = stats
}

func (worker *Worker) getRequestMethodName(mitmRequestProto *protos.MitmRequest) string {
	return protos.MITMRequestMethodName(mitmRequestProto.Method)
}

// getRequestMethods returns the RPC method IDs for an RPC request. It returns nil
// for a login request (or any request without an RPC payload).
func getRequestMethods(mitmRequestProto *protos.MitmRequest) []int32 {
	rpcRequest := mitmRequestProto.GetRpcRequest()
	if rpcRequest == nil {
		return nil
	}
	methods := make([]int32, 0, len(rpcRequest.Request))
	for idx, singleRequest := range rpcRequest.Request {
		methods[idx] = singleRequest.Method
	}
	return methods
}

func (worker *Worker) processResponseAndUpdateStats(payload []byte) {
	var mitmResponse protos.MitmResponse

	if err := proto.Unmarshal(payload, &mitmResponse); err != nil {
		return
	}

	request, ok := worker.requestTracker.Get(mitmResponse.Id)
	if !ok {
		return
	}

	status := mitmResponse.GetStatus()
	duration := time.Since(request.StartTime)
	worker.statsCollector.IncrWorkerResponses(
		duration,
		request.MethodName,
		protos.MITMResponseStatusName(status),
		mitmResponse.GetMitmError(),
	)
	if request.MethodName == "RPC_REQUEST" {
		worker.statsCollector.IncrRPCRequests(duration)
	}

	// Record stats in TimeWindowedStats fields
	durationMs := uint64(max(duration.Milliseconds(), 0))
	worker.requestStats.Add(1, durationMs)
}

// ReadWorkerWelcomeMessage reads and decodes the initial welcome message from a worker connection.
func ReadWorkerWelcomeMessage(ctx context.Context, wsConn WorkerWSConn) (*protos.WelcomeMessage, error) {
	reader, err := wsConn.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not read mitm worker welcome message: %w", err)
	}
	defer reader.Done()

	var welcomeMsg protos.WelcomeMessage
	err = proto.Unmarshal(reader.Bytes(), &welcomeMsg)
	if err != nil {
		return nil, fmt.Errorf("could not decode mitm worker welcome message: %w", err)
	}

	return &welcomeMsg, nil
}
