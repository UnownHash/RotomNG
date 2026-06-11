package services

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/UnownHash/RotomNG/libs/ginutil"
)

// HTTPServerConfig holds configuration for an HTTP server.
type HTTPServerConfig struct {
	Address         string
	Listener        net.Listener
	RoutesInstaller RoutesInstaller
	StatsRegistrar  StatsRegistrar
	AuthMiddleware  AuthMiddleware
}

// HTTPServer manages an HTTP server, a base HTTP service that
// can be emedded or used as a base for any HTTP-based service.
type HTTPServer struct {
	ctx      context.Context
	logger   *slog.Logger
	config   HTTPServerConfig
	server   *http.Server
	listener net.Listener
}

// NewHTTPServer creates a new HTTPServer instance.
func NewHTTPServer(ctx context.Context, logger *slog.Logger, config HTTPServerConfig) (*HTTPServer, error) {
	r := ginutil.NewEngineWithLogger(logger)
	if authMw := config.AuthMiddleware; authMw != nil {
		r.Use(authMw.Handler)
	}
	if registrar := config.StatsRegistrar; registrar != nil {
		registrar.RegisterGinEngine(r)
	}
	s := &HTTPServer{
		ctx:      ctx,
		logger:   logger,
		config:   config,
		server:   &http.Server{Addr: config.Address, Handler: r, ReadHeaderTimeout: 10 * time.Second},
		listener: config.Listener,
	}
	if err := config.RoutesInstaller.SetupRoutes(r); err != nil {
		return nil, err
	}
	return s, nil
}

// Run runs the HTTP server until Shutdown is called.
func (s *HTTPServer) Run() {
	var err error
	if s.listener != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, "HTTP server starting", slog.String("address", s.listener.Addr().String()))
		err = s.server.Serve(s.listener)
	} else {
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, "HTTP server starting", slog.String("address", s.server.Addr))
		err = s.server.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "HTTP server error", slog.String("error", err.Error()))
	}
}

// Shutdown gracefully shuts down the HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) {
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.LogAttrs(ctx, slog.LevelError, "HTTP server shutdown error", slog.String("error", err.Error()))
	} else {
		s.logger.LogAttrs(ctx, slog.LevelInfo, "HTTP server shutdown completed")
	}
}
