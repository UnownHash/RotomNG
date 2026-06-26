package handlers

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/settings"
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

// DeviceHandlerSettings holds settings for the device handler.
type DeviceHandlerSettings struct {
	PingInterval time.Duration
	PongWait     time.Duration
}

// Validate validates the device handler settings.
func (s DeviceHandlerSettings) Validate() error {
	if s.PingInterval <= 0 {
		return errors.New("device ping_interval must be positive")
	}
	if s.PongWait <= 0 {
		return errors.New("device pong_wait must be positive")
	}
	return nil
}

type deviceHandlerSettingsContainer = settings.Container[DeviceHandlerSettings]

// DeviceHandlerConfig holds configuration for the device handler.
type DeviceHandlerConfig struct {
	*deviceHandlerSettingsContainer

	Logger              *slog.Logger
	BufferPool          *bufferpool.BufferPool
	ConnectionManager   DeviceConnectionManager
	DeviceMonitorConfig mitm.DeviceMonitorConfig
	StatsCollector      DeviceStatsCollector
}

// Init initializes the settings container with the given settings.
func (cfg *DeviceHandlerConfig) Init(s DeviceHandlerSettings) (err error) {
	cfg.deviceHandlerSettingsContainer, err = settings.NewContainer(s)
	return
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
	getSettings         func() DeviceHandlerSettings
	notify              func(func(DeviceHandlerSettings)) func()
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
		getSettings:         cfg.GetSettings,
		notify:              cfg.Notify,
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

	// Install the read deadline and pong handler synchronously, before the
	// registration handshake reads from the connection. This bounds the
	// handshake and must happen on this (the reading) goroutine so it cannot
	// race with the reads or Close performed during registration. The ping loop
	// itself is only started once registration succeeds (see below).
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

	// Registration is complete; start the managed ping loop now that nothing
	// else mutates the connection's read state. It is safe to run concurrently
	// with the reads performed by deviceConn.Run.
	pingLoopWg.Go(func() {
		handler.startManagedPingLoop(pingLoopCtx, wsConn)
	})

	logger = logger.With(slog.String("device_id", deviceConn.ID()))
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm device control connection registered", slog.String(fieldRemoteAddr, remoteAddr))

	deviceConn.Run(ctx, handler.deviceMonitorConfig)
}

// startManagedPingLoop starts a ping loop for wsConn using the current settings,
// and restarts it whenever settings change. Runs until connCtx is done.
func (handler *DeviceHandler) startManagedPingLoop(connCtx context.Context, wsConn *ws.Conn) {
	settingsCh := make(chan DeviceHandlerSettings, 1)
	dereg := handler.notify(func(s DeviceHandlerSettings) {
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
