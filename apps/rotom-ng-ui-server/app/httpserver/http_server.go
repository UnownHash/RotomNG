// Package httpserver provides the HTTP/API server for the RotomNG admin UI.
package httpserver

import (
	"context"
	"embed"
	"log/slog"
	"net"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/services"
)

// HTTPAPIHandler defines the interface for the routes this service serves
// itself.
type HTTPAPIHandler interface {
	SetupAPIRoutes(apiGroup *gin.RouterGroup)
}

// Config holds configuration for the HTTP server.
type Config struct {
	Address        string
	Listener       net.Listener
	UIPath         string
	UIFS           *embed.FS
	DevMode        bool
	AuthMiddleware services.AuthMiddleware
	APIHandler     HTTPAPIHandler
	// ProxyHandler receives every /api request the APIHandler did not claim.
	ProxyHandler func(ginContext *gin.Context)
}

// HTTPServer is a type alias for the generic web server.
type HTTPServer = services.WebServer

// NewHTTPServer creates a new HTTPServer instance with admin-specific route
// setup.
func NewHTTPServer(ctx context.Context, logger *slog.Logger, cfg Config) (*HTTPServer, error) {
	webServerConfig := services.WebServerConfig{
		Address:        cfg.Address,
		Listener:       cfg.Listener,
		UIPath:         cfg.UIPath,
		UIFS:           cfg.UIFS,
		DevMode:        cfg.DevMode,
		AuthMiddleware: cfg.AuthMiddleware,
		SetupAPIRoutes: cfg.APIHandler.SetupAPIRoutes,
		APIFallback:    cfg.ProxyHandler,
	}
	return services.NewWebServer(ctx, logger, webServerConfig)
}
