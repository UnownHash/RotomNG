// Package httpserver provides the HTTP/API server for RotomNG.
package httpserver

import (
	"context"
	"embed"
	"log/slog"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/services"
)

// HTTPAPIHandler defines the interface for API route handlers.
type HTTPAPIHandler interface {
	SetupAPIRoutes(apiGroup *gin.RouterGroup)
	GetPrometheusEnabled() bool
	GetConfig(ginContext *gin.Context)
	ConfigReload(ginContext *gin.Context)
}

// MetricsHandler defines the interface for providing a metrics HTTP handler.
type MetricsHandler interface {
	GetMetricsHandler() http.Handler
}

// Config holds configuration for the HTTP server.
type Config struct {
	Address        string
	Listener       net.Listener
	UIPath         string
	UIFS           *embed.FS
	DevMode        bool
	MetricsHandler MetricsHandler
	AuthMiddleware services.AuthMiddleware
	APIHandler     HTTPAPIHandler
	StatsRegistrar services.StatsRegistrar
}

// HTTPServer is a type alias for the generic web server.
type HTTPServer = services.WebServer

// NewHTTPServer creates a new HTTPServer instance with app-specific route setup.
func NewHTTPServer(ctx context.Context, logger *slog.Logger, cfg Config) (*HTTPServer, error) {
	metricsHandler := cfg.MetricsHandler.GetMetricsHandler()
	apiHandler := cfg.APIHandler

	webServerConfig := services.WebServerConfig{
		Address:        cfg.Address,
		Listener:       cfg.Listener,
		UIPath:         cfg.UIPath,
		UIFS:           cfg.UIFS,
		DevMode:        cfg.DevMode,
		AuthMiddleware: cfg.AuthMiddleware,
		StatsRegistrar: cfg.StatsRegistrar,
		SetupAPIRoutes: func(apiGroup *gin.RouterGroup) {
			apiGroup.GET("/config", apiHandler.GetConfig)
			apiGroup.PUT("/config/reload", apiHandler.ConfigReload)

			apiHandler.SetupAPIRoutes(apiGroup)

			apiGroup.GET("/metrics", func(c *gin.Context) {
				if metricsHandler == nil || !apiHandler.GetPrometheusEnabled() {
					c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "metrics are not enabled"})
					return
				}
				metricsHandler.ServeHTTP(c.Writer, c.Request)
			})
		},
	}
	return services.NewWebServer(ctx, logger, webServerConfig)
}
