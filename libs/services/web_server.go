package services

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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
	// APIFallback, when set, handles /api requests that matched no route
	// registered by SetupAPIRoutes, in place of the 404. It runs only after
	// AuthMiddleware has accepted the request, so it is as protected as the
	// registered routes are. The admin service uses it to reverse-proxy every
	// endpoint it does not serve itself, which is why a new endpoint on
	// rotom-ng needs no corresponding change here.
	APIFallback func(ginContext *gin.Context)
	// UIDisabled reports whether the web UI should be withheld, leaving the
	// API as the only thing this listener answers.
	//
	// It is consulted per request rather than at route-registration time so a
	// config reload can turn the UI off without a restart -- gin's routes are
	// fixed once installed, so the switch has to live inside the handlers.
	// Nil means the UI is always served.
	UIDisabled func() bool
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
		// Session endpoints live on their own group with no auth middleware:
		// they are how the UI obtains a credential, so gating them would make
		// logging in impossible. Registering them as a separate group rather
		// than relying on ordering keeps that independent of gin's
		// middleware-capture-at-registration behaviour.
		if sessionMiddleware, ok := s.config.AuthMiddleware.(SessionAuthMiddleware); ok {
			sessionMiddleware.SetupSessionRoutes(r.Group("/api"), s.logger)
		}

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
		// Built with Rewrite alone rather than from NewSingleHostReverseProxy:
		// that constructor sets Director, and ReverseProxy refuses to serve
		// anything when both are set, which turned every page into a 502.
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.Out.Host = target.Host
			},
		}

		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				s.handleAPIMiss(c)
				return
			}
			if s.uiDisabled() {
				s.refuseUI(c)
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
		// Wrapped rather than installed directly so the UI can be switched off
		// at runtime: skipping the middleware leaves the request to NoRoute,
		// which refuses it below.
		r.Use(func(c *gin.Context) {
			if s.uiDisabled() {
				c.Next()
				return
			}
			staticServer(c)
		})

		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				s.handleAPIMiss(c)
				return
			}
			if s.uiDisabled() {
				s.refuseUI(c)
				return
			}
			c.Request.URL.Path = "/"
			c.FileFromFS("/", ginServeFS)
		})
	}
	return nil
}

// uiDisabled reports whether the UI should be withheld from this request.
func (s *WebServer) uiDisabled() bool {
	return s.config.UIDisabled != nil && s.config.UIDisabled()
}

// refuseUI answers a UI request on a listener whose UI is switched off.
//
// It says so rather than returning a bare 404 because the two are worth
// telling apart: a 404 from a mistyped path is a client problem, while this
// one means the server is configured not to serve the UI at all.
func (s *WebServer) refuseUI(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"status":   fieldError,
		fieldError: "the web ui is disabled",
	})
}

// handleAPIMiss serves an /api request that matched no registered route: the
// configured fallback if there is one, a 404 otherwise.
//
// NoRoute runs outside the authenticated /api group, so the credential check
// the group applies has to be repeated here before the fallback sees the
// request.
func (s *WebServer) handleAPIMiss(c *gin.Context) {
	fallback := s.config.APIFallback
	if fallback == nil {
		c.AbortWithStatusJSON(404, gin.H{"status": fieldError, fieldError: "resource does not exist"})
		return
	}
	if authMiddleware := s.config.AuthMiddleware; authMiddleware != nil {
		// Fail closed on a middleware that cannot answer out-of-chain: an
		// unrecognised type must not quietly turn the fallback into the one
		// unauthenticated way into the API.
		authorizer, ok := authMiddleware.(RequestAuthorizer)
		if !ok {
			s.logger.LogAttrs(c.Request.Context(), slog.LevelError,
				"auth middleware cannot authorize the API fallback; refusing the request",
				slog.String("path", c.Request.URL.Path))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if !authorizer.Allow(c) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	fallback(c)
}
