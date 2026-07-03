package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/api"
	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/connections"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/settings"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// Controller defines the interface for a controller connection.
type Controller interface {
	api.Controller
	connections.Controller
	Run(ctx context.Context)
}

// ControllerConnectionManager defines the interface for managing controller connections.
type ControllerConnectionManager[C Controller] interface {
	RegisterControllerConnectionV1(ctx context.Context, wsConn connections.ControllerWSConn, weight int, userAgent string) (C, error)
	RegisterControllerConnectionV2(ctx context.Context, wsConn connections.ControllerWSConn, userAgent string) (C, error)
}

// ControllerStatsCollector defines the interface for collecting controller statistics.
type ControllerStatsCollector interface {
	IncrControllerAcceptFails()
	IncrControllerAccepts()
	IncrControllerConnections(userAgent string)
	DecrControllerConnections(userAgent string)
}

// ControllerHandlerSettings holds settings for the controller handler.
type ControllerHandlerSettings struct {
	PingInterval        time.Duration
	PongWait            time.Duration
	RegistrationTimeout time.Duration
	// DataTimeout, when > 0, considers the connection dead if no data message is
	// received within the timeout, independent of ping/pong keep-alive activity.
	// Zero disables it. Only controller connections use a data timeout.
	DataTimeout time.Duration
}

// Validate validates the controller handler settings.
func (s ControllerHandlerSettings) Validate() error {
	if s.PingInterval <= 0 {
		return errors.New("controller ping_interval must be positive")
	}
	if s.PongWait <= 0 {
		return errors.New("controller pong_wait must be positive")
	}
	if s.RegistrationTimeout <= 0 {
		return errors.New("controller registration_timeout must be positive")
	}
	if s.DataTimeout < 0 {
		return errors.New("controller data_timeout must not be negative")
	}
	return nil
}

// pingSettings maps the handler settings to ws.PingSettings. The read timeout is
// PingInterval+PongWait of silence (no pong or data), matching the keep-alive
// behavior: a ping is sent every PingInterval and a healthy peer's pong resets it.
func (s ControllerHandlerSettings) pingSettings() ws.PingSettings {
	return ws.PingSettings{
		Interval: s.PingInterval,
		Timeout:  s.PingInterval + s.PongWait,
	}
}

type controllerHandlerSettingsContainer = settings.Container[ControllerHandlerSettings]

// ControllerHandlerConfig holds configuration for the controller handler.
type ControllerHandlerConfig[C Controller] struct {
	*controllerHandlerSettingsContainer

	Logger            *slog.Logger
	ConnectionManager ControllerConnectionManager[C]
	BufferPool        *bufferpool.BufferPool
	StatsCollector    ControllerStatsCollector
}

// Init initializes the settings container with the given settings.
func (cfg *ControllerHandlerConfig[C]) Init(s ControllerHandlerSettings) (err error) {
	cfg.controllerHandlerSettingsContainer, err = settings.NewContainer(s)
	return
}

// ControllerHandler handles WebSocket connections from controllers.
type ControllerHandler[C Controller] struct {
	BaseHandler

	ctx               context.Context
	logger            *slog.Logger
	connectionManager ControllerConnectionManager[C]
	statsCollector    ControllerStatsCollector
	bufferPool        *bufferpool.BufferPool
	getSettings       func() ControllerHandlerSettings
	notify            func(func(ControllerHandlerSettings)) func()
}

// applyReadTimeouts hands read-timeout enforcement to the Conn's timeout
// goroutine. It registers for settings changes first, then applies the current
// values, so a reload landing between the two cannot be missed. The returned
// function deregisters the settings watcher.
func (handler *ControllerHandler[C]) applyReadTimeouts(wsConn *ws.Conn) (dereg func()) {
	dereg = handler.notify(func(s ControllerHandlerSettings) {
		_ = wsConn.SetPingSettings(s.pingSettings())
		_ = wsConn.SetReadDataTimeout(s.DataTimeout)
	})
	s := handler.getSettings()
	_ = wsConn.SetPingSettings(s.pingSettings())
	_ = wsConn.SetReadDataTimeout(s.DataTimeout)
	return dereg
}

func (handler *ControllerHandler[C]) handleController(c *gin.Context, isV2 bool) {
	defer handler.PreventShutdown()()
	remoteAddr := c.Request.RemoteAddr
	logger := handler.logger.With(slog.String("component", "controller"))

	userAgent := c.Request.UserAgent()
	if userAgent == "" {
		userAgent = "unknown"
	}
	remoteAddrLogger := logger.With(
		slog.String(fieldRemoteAddr, remoteAddr),
		slog.String("user_agent", userAgent),
	)
	remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelInfo, "received controller connection")
	ctx := logging.ContextWithLogger(handler.ctx, logger)

	defer func() {
		if r := recover(); r != nil {
			logging.LogRecovery(logger, "panic caught in controller handler", r)
		}
		remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelInfo, "controller connection done")
	}()

	wsConn, err := ws.Accept(c.Writer, c.Request, ws.WithAcceptBufferPoolOpt(handler.bufferPool))
	if err != nil {
		handler.statsCollector.IncrControllerAcceptFails()
		remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to upgrade to websocket", slog.String("error", err.Error()))
		return
	}

	// Bound the registration handshake with a manual read deadline. The Conn's
	// timeout goroutine is idle (default ping settings) until we enable the ping
	// and data timeouts after registration succeeds.
	s := handler.getSettings()
	_ = wsConn.SetReadDeadline(time.Now().Add(s.RegistrationTimeout))

	// On app shutdown, expire the read deadline to unblock any reader stuck in
	// Run (Reader does not observe ctx). stop() deregisters on handler return so
	// nothing lingers once the connection ends.
	stop := context.AfterFunc(ctx, func() {
		_ = wsConn.SetReadDeadline(time.Now())
	})
	defer stop()

	closeWsConn := func(code int, reason string) {
		if ctx.Err() != nil {
			_ = wsConn.Close(ws.StatusGoingAway, "rotom is shutting down")
			return
		}
		_ = wsConn.Close(code, reason)
	}

	defer closeWsConn(ws.StatusNormalClosure, "")

	handler.statsCollector.IncrControllerAccepts()

	var controller C

	if isV2 { //nolint:nestif // refactoring would be too invasive
		var err error

		controller, err = handler.connectionManager.RegisterControllerConnectionV2(ctx, wsConn, userAgent)
		if err != nil {
			remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelError, "controller v2 registration failed", slog.String("error", err.Error()))
			closeWsConn(ws.StatusInternalServerError, err.Error())
			return
		}
		defer controller.Close(ws.StatusNormalClosure, "")
		remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelInfo, "v2 controller registered")
	} else {
		weight := 0
		if weightStr := c.Query("weight"); weightStr != "" {
			parsedWeight, err := strconv.Atoi(weightStr)
			if err != nil {
				remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelError, "received invalid weight query parameter", slog.String("weight", weightStr), slog.String("error", err.Error()))
				c.JSON(http.StatusBadRequest, gin.H{fieldError: "weight parameter must be a valid integer"})
				return
			}
			weight = parsedWeight
		}

		var err error
		controller, err = handler.connectionManager.RegisterControllerConnectionV1(ctx, wsConn, weight, userAgent)
		if err != nil {
			remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelError, "controller v1 registration failed", slog.String("error", err.Error()))
			closeWsConn(ws.StatusInternalServerError, err.Error())
			return
		}
		defer controller.Close(ws.StatusNormalClosure, "")
		remoteAddrLogger.LogAttrs(c.Request.Context(), slog.LevelInfo, "v1 controller registered")
	}

	handler.statsCollector.IncrControllerConnections(userAgent)
	defer handler.statsCollector.DecrControllerConnections(userAgent)

	// Registration complete: clear the handshake deadline and hand read-timeout
	// enforcement to the Conn's timeout goroutine (ping + data timeouts), which
	// runs safely alongside the reads performed by controller.Run.
	_ = wsConn.SetReadDeadline(time.Time{})
	defer handler.applyReadTimeouts(wsConn)()

	ctx = logging.ContextWithLogger(ctx, logger)
	controller.Run(ctx)
}

// HandleControllerV1 handles a v1 controller WebSocket connection.
func (handler *ControllerHandler[C]) HandleControllerV1(c *gin.Context) {
	handler.handleController(c, false)
}

// HandleControllerV2 handles a v2 controller WebSocket connection.
func (handler *ControllerHandler[C]) HandleControllerV2(c *gin.Context) {
	handler.handleController(c, true)
}

// NewControllerHandler creates a new ControllerHandler instance.
func NewControllerHandler[C Controller](ctx context.Context, cfg ControllerHandlerConfig[C]) *ControllerHandler[C] {
	if cfg.StatsCollector == nil {
		cfg.StatsCollector = NewNoOpStatsCollector()
	}
	return &ControllerHandler[C]{
		ctx:               ctx,
		logger:            cfg.Logger,
		connectionManager: cfg.ConnectionManager,
		statsCollector:    cfg.StatsCollector,
		bufferPool:        cfg.BufferPool,
		getSettings:       cfg.GetSettings,
		notify:            cfg.Notify,
	}
}
