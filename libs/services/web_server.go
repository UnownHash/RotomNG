package services

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// Log field key constants.
const fieldError = "error"

// WebServerConfig holds configuration for the web server.
type WebServerConfig struct {
	Address        string
	Listener       net.Listener
	UIPath         string
	UIFS           *embed.FS
	DevMode        bool
	AuthMiddleware AuthMiddleware
	SetupAPIRoutes func(apiGroup *gin.RouterGroup)
	StatsRegistrar StatsRegistrar
}

// WebServer manages the HTTP server with API routing and UI serving.
type WebServer struct {
	*HTTPServer

	config WebServerConfig
	logger *slog.Logger
}

// NewWebServer creates a new WebServer instance.
func NewWebServer(ctx context.Context, logger *slog.Logger, config WebServerConfig) (*WebServer, error) {
	server := &WebServer{
		config: config,
		logger: logger,
	}
	baseConfig := HTTPServerConfig{
		Address:         config.Address,
		Listener:        config.Listener,
		RoutesInstaller: server,
		StatsRegistrar:  config.StatsRegistrar,
		// AuthMiddleware handled by us for /api only.
	}
	httpServer, err := NewHTTPServer(ctx, logger, baseConfig)
	if err != nil {
		return nil, err
	}
	server.HTTPServer = httpServer
	return server, nil
}

// SetupRoutes configures all HTTP routes.
func (s *WebServer) SetupRoutes(r *gin.Engine) error {
	// API routes
	{
		api := r.Group("/api")

		if authMiddleware := s.config.AuthMiddleware; authMiddleware != nil {
			api.Use(authMiddleware.Handler)
		}

		s.config.SetupAPIRoutes(api)
	}

	// Check if we're in development mode
	if s.config.DevMode { //nolint:nestif // refactoring would be too invasive
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, "setting up HTTP server routes to proxy to UI dev server")

		target, _ := url.Parse("http://localhost:4199")
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Rewrite = func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
		}

		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.AbortWithStatusJSON(404, gin.H{"status": fieldError, fieldError: "resource does not exist"})
				return
			}
			proxy.ServeHTTP(c.Writer, c.Request)
		})
	} else {
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, "setting up HTTP server routes to serve static files")

		var ginServeFS static.ServeFileSystem

		if s.config.UIPath == "" {
			if s.config.UIFS == nil {
				return errors.New("no embedded UI and no -ui-path")
			}
			var err error
			ginServeFS, err = static.EmbedFolder(*s.config.UIFS, "static")
			if err != nil {
				return fmt.Errorf("failed to embed folder for UI: %w", err)
			}
		} else {
			ginServeFS = static.LocalFile(s.config.UIPath, false)
		}

		staticServer := static.Serve("/", ginServeFS)
		r.Use(staticServer)

		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.AbortWithStatusJSON(404, gin.H{"status": fieldError, fieldError: "resource does not exist"})
				return
			}
			c.Request.URL.Path = "/"
			c.FileFromFS("/", ginServeFS)
		})
	}
	return nil
}
