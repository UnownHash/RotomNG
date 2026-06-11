package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/handlers"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// WorkerHandlerConfig holds configuration for the worker handler.
type WorkerHandlerConfig struct {
	Logger                   *slog.Logger
	BufferPool               *bufferpool.BufferPool
	MITMWorkerStatsCollector MITMWorkerStatsCollector
	ConnectionManager        WorkerConnectionManager
	StatsCollector           WorkerStatsCollector
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
	go func(ctx context.Context) {
		<-ctx.Done()
		_ = wsConn.SetReadDeadline(time.Now())
	}(ctx)

	handler.statsCollector.IncrWorkerAccepts()

	welcomeMsg, err := mitm.ReadWorkerWelcomeMessage(ctx, wsConn)
	if err != nil {
		handler.statsCollector.IncrWorkerRegistrationFails()
		_ = wsConn.Close(ws.StatusProtocolError, "registration failed")
		return
	}

	worker := mitm.NewWorker(wsConn, welcomeMsg, handler.mitmWorkerStatsCollector)

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

	logger = logger.With(slog.String("worker_id", worker.ID()))
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm worker registered", slog.String("remote_addr", remoteAddr))

	worker.Run(ctx)
}
