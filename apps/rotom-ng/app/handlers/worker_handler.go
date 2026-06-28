package handlers

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/handlers"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/settings"
	"github.com/UnownHash/RotomNG/libs/stats"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// WorkerHandlerSettings holds reloadable settings for the worker handler. The
// ping settings enforce a read timeout on MITM worker connections: a ping is
// sent every PingInterval and the read deadline is extended by
// PingInterval+PongWait on each pong, so a worker that stops responding is
// disconnected rather than holding the connection open forever.
type WorkerHandlerSettings struct {
	PingInterval time.Duration
	PongWait     time.Duration
}

// Validate validates the worker handler settings.
func (s WorkerHandlerSettings) Validate() error {
	if s.PingInterval <= 0 {
		return errors.New("worker ping_interval must be positive")
	}
	if s.PongWait <= 0 {
		return errors.New("worker pong_wait must be positive")
	}
	return nil
}

type workerHandlerSettingsContainer = settings.Container[WorkerHandlerSettings]

// WorkerHandlerConfig holds configuration for the worker handler.
type WorkerHandlerConfig struct {
	*workerHandlerSettingsContainer

	Logger                   *slog.Logger
	BufferPool               *bufferpool.BufferPool
	MITMWorkerStatsCollector MITMWorkerStatsCollector
	ConnectionManager        WorkerConnectionManager
	StatsCollector           WorkerStatsCollector
	// GlobalRequestStats is the shared collector that each worker also records
	// its request stats into, so aggregate stats persist across worker
	// disconnects. The same collector is read by the API handler for the status
	// reply.
	GlobalRequestStats *stats.CountDurationCollector[uint64]
}

// Init initializes the settings container with the given settings.
func (cfg *WorkerHandlerConfig) Init(s WorkerHandlerSettings) (err error) {
	cfg.workerHandlerSettingsContainer, err = settings.NewContainer(s)
	return
}

// WorkerHandler handles incoming MITM worker WebSocket connections.
type WorkerHandler struct {
	handlers.BaseHandler

	ctx context.Context

	logger                   *slog.Logger
	bufferPool               *bufferpool.BufferPool
	mitmWorkerStatsCollector MITMWorkerStatsCollector
	connectionManager        WorkerConnectionManager
	statsCollector           WorkerStatsCollector
	globalRequestStats       *stats.CountDurationCollector[uint64]
	getSettings              func() WorkerHandlerSettings
	notify                   func(func(WorkerHandlerSettings)) func()
}

// NewWorkerHandler creates a new WorkerHandler with the given context and configuration.
func NewWorkerHandler(ctx context.Context, cfg WorkerHandlerConfig) *WorkerHandler {
	if cfg.StatsCollector == nil {
		cfg.StatsCollector = NewNoOpStatsCollector()
	}
	return &WorkerHandler{
		ctx:                      ctx,
		logger:                   cfg.Logger,
		bufferPool:               cfg.BufferPool,
		mitmWorkerStatsCollector: cfg.MITMWorkerStatsCollector,
		connectionManager:        cfg.ConnectionManager,
		statsCollector:           cfg.StatsCollector,
		globalRequestStats:       cfg.GlobalRequestStats,
		getSettings:              cfg.GetSettings,
		notify:                   cfg.Notify,
	}
}

// startManagedPingLoop starts a ping loop for wsConn using the current settings,
// and restarts it whenever settings change. Runs until connCtx is done.
func (handler *WorkerHandler) startManagedPingLoop(connCtx context.Context, wsConn *ws.Conn) {
	settingsCh := make(chan WorkerHandlerSettings, 1)
	dereg := handler.notify(func(s WorkerHandlerSettings) {
		select {
		case <-settingsCh:
		default:
		}
		settingsCh <- s
	})
	defer dereg()

	s := handler.getSettings()
	pingCtx, pingCancel := context.WithCancel(connCtx)
	wsConn.StartPingLoop(pingCtx, s.PingInterval, s.PongWait)

	for {
		select {
		case <-connCtx.Done():
			pingCancel()
			return
		case s = <-settingsCh:
			pingCancel()
			//nolint:fatcontext // each pingCtx derives directly from connCtx; chain depth is constant
			pingCtx, pingCancel = context.WithCancel(connCtx)
			wsConn.StartPingLoop(pingCtx, s.PingInterval, s.PongWait)
		}
	}
}

// HandleWorker upgrades the HTTP connection to a WebSocket and manages the worker lifecycle.
func (handler *WorkerHandler) HandleWorker(c *gin.Context) {
	defer handler.PreventShutdown()()
	remoteAddr := c.Request.RemoteAddr
	logger := logging.LoggerFromContext(c.Request.Context())
	if logger == nil {
		logger = handler.logger.With(slog.String("component", "mitm_worker"))
	}

	ctx := logging.ContextWithLogger(handler.ctx, logger)
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "received mitm worker connection", slog.String("remote_addr", remoteAddr))
	defer func() {
		if r := recover(); r != nil {
			logging.LogRecovery(
				logger.With(slog.String("remote_addr", remoteAddr)),
				"panic caught in mitm worker handler",
				r,
			)
		}
		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm worker connection done", slog.String("remote_addr", remoteAddr))
	}()

	wsConn, err := ws.Accept(c.Writer, c.Request, ws.WithAcceptBufferPoolOpt(handler.bufferPool))
	if err != nil {
		handler.statsCollector.IncrWorkerAcceptFails()
		logger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to upgrade mitm worker connection to websocket", slog.String("remote_addr", remoteAddr), slog.String("error", err.Error()))
		return
	}
	// this Close only happens if we didn't reach the other Close below, indicating
	// we must have panicked during Register.
	defer wsConn.Close(ws.StatusInternalServerError, "unexpected error handling worker connection")

	// Install the read deadline and pong handler synchronously, before the
	// welcome handshake reads from the connection. This bounds the handshake and
	// must happen on this (the reading) goroutine so it cannot race with the
	// reads performed during registration. The ping loop itself is only started
	// once registration succeeds (see below).
	s := handler.getSettings()
	wsConn.EnableReadTimeout(s.PingInterval, s.PongWait)

	// pingLoopCtx is cancelled before wsConn.Close() (LIFO defer order) so that
	// startManagedPingLoop cannot call wg.Go after wg.Wait has returned.
	pingLoopCtx, pingLoopCancel := context.WithCancel(ctx)
	var pingLoopWg sync.WaitGroup
	defer func() {
		pingLoopCancel()
		pingLoopWg.Wait()
	}()
	go func() {
		<-pingLoopCtx.Done()
		_ = wsConn.SetReadDeadline(time.Now())
	}()

	handler.statsCollector.IncrWorkerAccepts()

	welcomeMsg, err := mitm.ReadWorkerWelcomeMessage(ctx, wsConn)
	if err != nil {
		handler.statsCollector.IncrWorkerRegistrationFails()
		_ = wsConn.Close(ws.StatusProtocolError, "registration failed")
		return
	}

	worker := mitm.NewWorker(wsConn, welcomeMsg, handler.mitmWorkerStatsCollector, handler.globalRequestStats)

	handler.statsCollector.IncrWorkersConnected(worker.Origin())
	defer handler.statsCollector.DecrWorkersConnected(worker.Origin())

	err = handler.connectionManager.RegisterWorker(
		ctx,
		worker,
	)
	if err != nil {
		logger.LogAttrs(c.Request.Context(), slog.LevelError, "mitm worker registration failed", slog.String("remote_addr", remoteAddr), slog.String("error", err.Error()))
		handler.statsCollector.IncrWorkerRegistrationFails()
		_ = worker.Close(ws.StatusProtocolError, "registration failed: "+err.Error())
		return
	}
	defer worker.Close(ws.StatusNormalClosure, "")

	// Registration is complete; start the managed ping loop now that nothing
	// else mutates the connection's read state. It is safe to run concurrently
	// with the reads performed by worker.Run.
	pingLoopWg.Go(func() {
		handler.startManagedPingLoop(pingLoopCtx, wsConn)
	})

	logger = logger.With(slog.String("worker_id", worker.ID()))
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm worker registered", slog.String("remote_addr", remoteAddr))

	worker.Run(ctx)
}
