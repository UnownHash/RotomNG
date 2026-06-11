package services

import (
	"context"
	"log/slog"
	"net"

	"github.com/gin-gonic/gin"
)

// DeviceHandler handles device control WebSocket connections.
type DeviceHandler interface {
	HandleDeviceControl(ginContext *gin.Context)
}

// WorkerHandler handles worker WebSocket connections.
type WorkerHandler interface {
	HandleWorker(ginContext *gin.Context)
}

// DeviceServerConfig holds configuration for the device server.
type DeviceServerConfig struct {
	Address        string
	Listener       net.Listener
	AuthMiddleware AuthMiddleware
	DeviceHandler  DeviceHandler
	WorkerHandler  WorkerHandler
}

// DeviceServer manages the device server and routing.
type DeviceServer struct {
	*HTTPServer

	deviceHandler DeviceHandler
	workerHandler WorkerHandler
}

// NewDeviceServer creates a new DeviceServer instance.
func NewDeviceServer(ctx context.Context, logger *slog.Logger, config DeviceServerConfig) (*DeviceServer, error) {
	server := &DeviceServer{
		deviceHandler: config.DeviceHandler,
		workerHandler: config.WorkerHandler,
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

// SetupRoutes configures all device routes.
func (s *DeviceServer) SetupRoutes(r *gin.Engine) error {
	r.GET("/control", s.deviceHandler.HandleDeviceControl)
	r.GET("/", s.workerHandler.HandleWorker)
	return nil
}
