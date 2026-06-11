package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// MITMDeviceConn is an alias for mitm.DeviceConn.
type MITMDeviceConn = mitm.DeviceConn

// DeviceConnectionManager defines the interface for managing device connections.
type DeviceConnectionManager interface {
	RegisterDeviceConnection(ctx context.Context, wsConn mitm.DeviceWSConn) (*MITMDeviceConn, error)
}

// DeviceStatsCollector defines the interface for collecting device statistics.
type DeviceStatsCollector interface {
	IncrDeviceControlAccepts()
	IncrDeviceControlAcceptFails()
}

// DeviceHandlerConfig holds configuration for the device handler.
type DeviceHandlerConfig struct {
	Logger              *slog.Logger
	BufferPool          *bufferpool.BufferPool
	ConnectionManager   DeviceConnectionManager
	DeviceMonitorConfig mitm.DeviceMonitorConfig
	StatsCollector      DeviceStatsCollector
}

// DeviceHandler handles WebSocket connections from MITM devices.
type DeviceHandler struct {
	BaseHandler

	ctx context.Context

	logger              *slog.Logger
	bufferPool          *bufferpool.BufferPool
	connectionManager   DeviceConnectionManager
	deviceMonitorConfig mitm.DeviceMonitorConfig
	statsCollector      DeviceStatsCollector
}

// NewDeviceHandler creates a new DeviceHandler instance.
func NewDeviceHandler(ctx context.Context, cfg DeviceHandlerConfig) *DeviceHandler {
	if cfg.StatsCollector == nil {
		cfg.StatsCollector = NewNoOpStatsCollector()
	}
	return &DeviceHandler{
		ctx:                 ctx,
		logger:              cfg.Logger,
		bufferPool:          cfg.BufferPool,
		connectionManager:   cfg.ConnectionManager,
		deviceMonitorConfig: cfg.DeviceMonitorConfig,
		statsCollector:      cfg.StatsCollector,
	}
}

// HandleDeviceControl handles a device control WebSocket connection.
func (handler *DeviceHandler) HandleDeviceControl(c *gin.Context) {
	defer handler.PreventShutdown()()
	remoteAddr := c.Request.RemoteAddr
	logger := handler.logger.With(slog.String("component", "mitm_device"))
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "received mitm device control connection", slog.String(fieldRemoteAddr, remoteAddr))
	ctx := logging.ContextWithLogger(handler.ctx, logger)

	defer func() {
		if r := recover(); r != nil {
			logging.LogRecovery(
				logger.With(slog.String(fieldRemoteAddr, remoteAddr)),
				"panic caught in mitm device control handler",
				r,
			)
		}
		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm device control connection done", slog.String(fieldRemoteAddr, remoteAddr))
	}()

	wsConn, err := ws.Accept(c.Writer, c.Request, ws.WithAcceptBufferPoolOpt(handler.bufferPool))
	if err != nil {
		handler.statsCollector.IncrDeviceControlAcceptFails()
		logger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to upgrade to websocket", slog.String(fieldRemoteAddr, remoteAddr), slog.String("error", err.Error()))
		return
	}
	// this Close only happens if we didn't reach the other Close below, indicating
	// we must have panicked during Register.
	defer wsConn.Close(ws.StatusInternalServerError, "unexpected error handling device connection")
	go func(ctx context.Context) {
		<-ctx.Done()
		_ = wsConn.SetReadDeadline(time.Now())
	}(ctx)

	handler.statsCollector.IncrDeviceControlAccepts()

	deviceConn, err := handler.connectionManager.RegisterDeviceConnection(
		ctx,
		wsConn,
	)
	if err != nil {
		logger.LogAttrs(c.Request.Context(), slog.LevelError, "device registration failed", slog.String(fieldRemoteAddr, remoteAddr), slog.String("error", err.Error()))
		return
	}
	defer deviceConn.Close(ws.StatusNormalClosure, "")

	logger = logger.With(slog.String("device_id", deviceConn.ID()))
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm device control connection registered", slog.String(fieldRemoteAddr, remoteAddr))

	deviceConn.Run(ctx, handler.deviceMonitorConfig)
}
