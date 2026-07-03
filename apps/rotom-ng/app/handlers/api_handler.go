// Package handlers provides HTTP and WebSocket request handlers for the RotomNG application.
package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/handlers"
	"github.com/UnownHash/RotomNG/libs/settings"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
)

// HTTPAPIHandlerSettings holds the current configuration for HTTP API handlers.
type HTTPAPIHandlerSettings struct {
	CurrentConfig config.Config
}

// Validate validates the HTTPAPIHandlerSettings. Currently always returns nil.
func (s HTTPAPIHandlerSettings) Validate() error {
	return nil
}

type apiHandlerSettingsContainer = settings.Container[HTTPAPIHandlerSettings]

// HTTPAPIHandlerConfig holds configuration for HTTP api handlers.
type HTTPAPIHandlerConfig struct {
	*apiHandlerSettingsContainer

	AppVersion string
	GitSHA     string
	Logger     *slog.Logger
	APIHandler *handlers.APIHandler[*Controller, *MITMWorker]
	ReloadFn   func() error
}

// Init initializes the settings container with the given settings.
func (cfg *HTTPAPIHandlerConfig) Init(s HTTPAPIHandlerSettings) (err error) {
	cfg.apiHandlerSettingsContainer, err = settings.NewContainer(s)
	return
}

type apiHandler = handlers.APIHandler[*Controller, *MITMWorker]

// HTTPAPIHandler wraps the generic HTTPAPIHandler and adds app-specific endpoints.
type HTTPAPIHandler struct {
	*apiHandler

	logger      *slog.Logger
	getSettings func() HTTPAPIHandlerSettings
	appVersion  string
	gitSHA      string
	reloadFn    func() error
}

// NewHTTPAPIHandler creates a new HTTPAPIHandler instance.
func NewHTTPAPIHandler(cfg HTTPAPIHandlerConfig) *HTTPAPIHandler {
	return &HTTPAPIHandler{
		apiHandler:  cfg.APIHandler,
		logger:      cfg.Logger,
		getSettings: cfg.GetSettings,
		appVersion:  cfg.AppVersion,
		gitSHA:      cfg.GitSHA,
		reloadFn:    cfg.ReloadFn,
	}
}

// GetConfig returns the current application configuration as JSON.
func (ah *HTTPAPIHandler) GetConfig(c *gin.Context) {
	cfg := ah.getSettings().CurrentConfig

	tuning := gin.H{"profiling": cfg.Tuning.Profiling}
	if cfg.Tuning.DisableWorkerStats {
		tuning["disable_worker_stats"] = true
	}
	jsonConfig := gin.H{
		"version": ah.appVersion,
		"sha":     ah.gitSHA,
		"tuning":  tuning,
	}
	if cfg.Instance != "" {
		jsonConfig["instance"] = cfg.Instance
	}
	if cfg.Jobs != nil && cfg.Jobs.Enable {
		jsonConfig["jobs"] = gin.H{
			"enable": true,
			"path":   cfg.Jobs.Path,
		}
	}
	if rateLimit := cfg.RateLimit; rateLimit != nil && rateLimit.Enable {
		jsonConfig["rate_limit"] = gin.H{
			"enable":         cfg.RateLimit.Enable,
			"max_selections": cfg.RateLimit.MaxSelectionsPerDuration,
			"duration":       cfg.RateLimit.Duration,
		}
	}
	if cfg.DeviceListener != nil {
		jsonConfig["device_listener"] = gin.H{
			"ping_interval": cfg.DeviceListener.PingInterval.String(),
			"pong_wait":     cfg.DeviceListener.PongWait.String(),
		}
	}
	if cfg.ControllerListener != nil {
		var dataTimeout time.Duration
		if cfg.ControllerListener.DataTimeout != nil {
			dataTimeout = *cfg.ControllerListener.DataTimeout
		}
		jsonConfig["controller_listener"] = gin.H{
			"ping_interval":        cfg.ControllerListener.PingInterval.String(),
			"pong_wait":            cfg.ControllerListener.PongWait.String(),
			"registration_timeout": cfg.ControllerListener.RegistrationTimeout.String(),
			"data_timeout":         dataTimeout.String(),
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": jsonConfig})
}

// GetPrometheusEnabled returns whether Prometheus metrics are enabled.
func (ah *HTTPAPIHandler) GetPrometheusEnabled() bool {
	return ah.getSettings().CurrentConfig.Prometheus.Enable
}

// ConfigReload handles a request to reload the application configuration.
func (ah *HTTPAPIHandler) ConfigReload(c *gin.Context) {
	logger := ah.logger.With(slog.String("remote_addr", c.Request.RemoteAddr))
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "config reload requested")

	if err := ah.reloadFn(); err != nil {
		ah.logger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to reload config", slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "config reloaded")
	ah.GetConfig(c)
}
