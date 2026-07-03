package handlers

import (
	"context"
	"errors"
	"log/slog"
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

// pingSettings maps the handler settings to ws.PingSettings. The read timeout is
// PingInterval+PongWait of silence (no pong or data), matching the keep-alive
// behavior: a ping is sent every PingInterval and a healthy peer's pong resets it.
func (s DeviceHandlerSettings) pingSettings() ws.PingSettings {
	return ws.PingSettings{
		Interval: s.PingInterval,
		Timeout:  s.PingInterval + s.PongWait,
	}
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

	// Bound the registration handshake with a manual read deadline. The Conn's
	// timeout goroutine is idle (default ping settings) until we enable pinging
	// after registration succeeds.
	s := handler.getSettings()
	_ = wsConn.SetReadDeadline(time.Now().Add(s.PingInterval + s.PongWait))

	// On app shutdown, expire the read deadline to unblock any reader stuck in
	// Run (Reader does not observe ctx). stop() deregisters on handler return so
	// nothing lingers once the connection ends.
	stop := context.AfterFunc(ctx, func() {
		_ = wsConn.SetReadDeadline(time.Now())
	})
	defer stop()

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

	// Registration complete: clear the handshake deadline and hand read-timeout
	// enforcement to the Conn's timeout goroutine, which runs safely alongside
	// the reads performed by deviceConn.Run.
	_ = wsConn.SetReadDeadline(time.Time{})
	defer handler.applyReadTimeouts(wsConn)()

	logger = logger.With(slog.String("device_id", deviceConn.ID()))
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "mitm device control connection registered", slog.String(fieldRemoteAddr, remoteAddr))

	deviceConn.Run(ctx, handler.deviceMonitorConfig)
}

// applyReadTimeouts hands read-timeout enforcement to the Conn's timeout
// goroutine. It registers for settings changes first, then applies the current
// values, so a reload landing between the two cannot be missed. The returned
// function deregisters the settings watcher.
func (handler *DeviceHandler) applyReadTimeouts(wsConn *ws.Conn) (dereg func()) {
	dereg = handler.notify(func(s DeviceHandlerSettings) {
		_ = wsConn.SetPingSettings(s.pingSettings())
	})
	_ = wsConn.SetPingSettings(handler.getSettings().pingSettings())
	return dereg
}
