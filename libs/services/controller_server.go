// Package services provides HTTP and WebSocket server implementations.
package services

import (
	"context"
	"log/slog"
	"net"

	"github.com/gin-gonic/gin"
)

// ControllerServerConfig holds configuration for the controller server.
type ControllerServerConfig struct {
	Address        string
	Listener       net.Listener
	AuthMiddleware AuthMiddleware
}

// ControllerHandler handles controller WebSocket connections.
type ControllerHandler interface {
	HandleControllerV1(c *gin.Context)
	HandleControllerV2(c *gin.Context)
}

// ControllerServer manages the controller server and routing.
type ControllerServer struct {
	*HTTPServer

	config  ControllerServerConfig
	handler ControllerHandler
}

// NewControllerServer creates a new ControllerServer instance.
func NewControllerServer(ctx context.Context, logger *slog.Logger, config ControllerServerConfig, handler ControllerHandler) (*ControllerServer, error) {
	server := &ControllerServer{
		config:  config,
		handler: handler,
	}
	baseConfig := HTTPServerConfig{
		Address:         config.Address,
		Listener:        config.Listener,
		RoutesInstaller: server,
		AuthMiddleware:  config.AuthMiddleware,
	}
	httpServer, err := NewHTTPServer(ctx, logger, baseConfig)
	if err != nil {
		return nil, err
	}
	server.HTTPServer = httpServer
	return server, nil
}

// SetupRoutes configures all controller routes.
func (s *ControllerServer) SetupRoutes(r *gin.Engine) error {
	r.GET("/", s.handler.HandleControllerV1)
	r.GET("/controller", s.handler.HandleControllerV2)
	return nil
}
