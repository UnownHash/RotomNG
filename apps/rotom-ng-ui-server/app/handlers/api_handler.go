// Package handlers provides the HTTP API handlers this service answers
// itself, as opposed to the ones it proxies to an instance.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/settings"

	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/config"
	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/instances"
)

// Response field keys.
const (
	fieldStatus = "status"
	fieldError  = "error"
	statusOK    = "ok"
	statusError = "error"
)

// InstanceLister supplies the current state of every configured instance.
type InstanceLister interface {
	Snapshot() []instances.State
}

// HTTPAPIHandlerSettings holds the configuration the handlers read, so a
// config reload can swap it without restarting the server.
type HTTPAPIHandlerSettings struct {
	CurrentConfig config.Config
}

// Validate validates the HTTPAPIHandlerSettings. Currently always returns nil.
func (s HTTPAPIHandlerSettings) Validate() error {
	return nil
}

type apiHandlerSettingsContainer = settings.Container[HTTPAPIHandlerSettings]

// HTTPAPIHandlerConfig holds configuration for the HTTP API handlers.
type HTTPAPIHandlerConfig struct {
	*apiHandlerSettingsContainer

	AppVersion string
	GitSHA     string
	Logger     *slog.Logger
	Instances  InstanceLister
	ReloadFn   func() error
}

// Init initializes the settings container with the given settings.
func (cfg *HTTPAPIHandlerConfig) Init(s HTTPAPIHandlerSettings) (err error) {
	cfg.apiHandlerSettingsContainer, err = settings.NewContainer(s)
	return
}

// HTTPAPIHandler serves this service's own API endpoints.
type HTTPAPIHandler struct {
	logger      *slog.Logger
	getSettings func() HTTPAPIHandlerSettings
	instances   InstanceLister
	appVersion  string
	gitSHA      string
	reloadFn    func() error
}

// NewHTTPAPIHandler creates a new HTTPAPIHandler instance.
func NewHTTPAPIHandler(cfg HTTPAPIHandlerConfig) *HTTPAPIHandler {
	return &HTTPAPIHandler{
		logger:      cfg.Logger,
		getSettings: cfg.GetSettings,
		instances:   cfg.Instances,
		appVersion:  cfg.AppVersion,
		gitSHA:      cfg.GitSHA,
		reloadFn:    cfg.ReloadFn,
	}
}

// SetupAPIRoutes registers the endpoints this service answers itself. Every
// other /api path is proxied, via the web server's API fallback.
func (ah *HTTPAPIHandler) SetupAPIRoutes(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/config", ah.GetConfig)
	apiGroup.PUT("/config/reload", ah.ConfigReload)
}

// GetConfig returns this service's configuration as JSON, on the same path
// rotom-ng serves its own.
//
// The reply carries the fields of rotom-ng's config that make sense for a
// service that holds no devices of its own, plus "instances". That key is what
// tells the UI it is talking to this service rather than to a rotom-ng: it is
// always present here -- an empty list when nothing is configured -- and never
// present in a rotom-ng reply. Per-instance settings the UI gates features on
// live in each entry's own "config", so the UI follows the instance the
// operator has selected.
func (ah *HTTPAPIHandler) GetConfig(c *gin.Context) {
	cfg := ah.getSettings().CurrentConfig

	jsonConfig := gin.H{
		"version":   ah.appVersion,
		"sha":       ah.gitSHA,
		"instances": ah.instances.Snapshot(),
	}
	if cfg.Instance != "" {
		jsonConfig["instance"] = cfg.Instance
	}

	c.JSON(http.StatusOK, gin.H{fieldStatus: statusOK, "config": jsonConfig})
}

// ConfigReload handles a request to reload the application configuration.
func (ah *HTTPAPIHandler) ConfigReload(c *gin.Context) {
	logger := ah.logger.With(slog.String("remote_addr", c.Request.RemoteAddr))
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "config reload requested")

	if err := ah.reloadFn(); err != nil {
		ah.logger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to reload config", slog.String(fieldError, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{fieldStatus: statusError, fieldError: err.Error()})
		return
	}
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "config reloaded")
	ah.GetConfig(c)
}
