// Package app implements the RotomNG admin UI server: one web UI fronting
// several rotom-ng instances, which it proxies to.
package app

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/auth"
	"github.com/UnownHash/RotomNG/libs/gitutil"
	"github.com/UnownHash/RotomNG/libs/logging"

	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/config"
	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/handlers"
	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/httpserver"
	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/instances"
	"github.com/UnownHash/RotomNG/apps/rotom-ng-ui-server/app/proxy"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/version"
)

var gitSHA = gitutil.GetGitBuildSHA()

const (
	// The admin UI ships the same version as rotom-ng: it is built from the
	// same UI sources and tracks the same API.
	appVersion = version.AppVersion
	userAgent  = "RotomNG-UI/" + version.AppVersion
)

func getInstanceSettings(cfg *config.Config) instances.Settings {
	instanceConfigs := make([]instances.InstanceConfig, len(cfg.Instances))
	for idx, instance := range cfg.Instances {
		instanceConfigs[idx] = instances.InstanceConfig{
			BaseURL:   instance.BaseURL,
			APISecret: instance.APISecret,
		}
	}
	return instances.Settings{
		Instances: instanceConfigs,
		Interval:  cfg.InstanceMonitor.Interval,
		Timeout:   cfg.InstanceMonitor.Timeout,
	}
}

func getHTTPAPIHandlerSettings(cfg *config.Config) handlers.HTTPAPIHandlerSettings {
	return handlers.HTTPAPIHandlerSettings{
		CurrentConfig: *cfg,
	}
}

// FlagConfig holds command-line flag configuration for the application.
type FlagConfig struct {
	DebugMode    bool
	UIPath       string
	UIDev        bool
	UIFS         *embed.FS
	ReloadConfig func() (*config.Config, error)
}

// App is the main admin application, managing the web server and the instance
// monitor.
type App struct {
	cfg      *config.Config
	flagCfg  FlagConfig
	logger   *slog.Logger
	levelVar *slog.LevelVar
	closer   io.Closer

	shutdownTimeout atomic.Int64 // nanoseconds

	ctx    context.Context
	cancel context.CancelFunc

	instanceManager      *instances.Manager
	httpAPIHandlerConfig handlers.HTTPAPIHandlerConfig
	httpAuthMiddleware   *auth.Middleware
	httpServer           *httpserver.HTTPServer
}

// NewApp creates a new App instance with the given configuration.
func NewApp(cfg *config.Config, flagCfg FlagConfig) (*App, error) {
	logger, levelVar, closer, err := cfg.GetLogger()
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:      cfg,
		flagCfg:  flagCfg,
		logger:   logger,
		levelVar: levelVar,
		closer:   closer,
	}, nil
}

// Logger returns the application's logger instance.
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// Cancel cancels the application context, triggering a graceful shutdown.
// This is intended for use by test utilities that need to stop the app
// from outside the app package.
func (a *App) Cancel() {
	if a.cancel != nil {
		a.cancel()
	}
}

// Init initializes all application dependencies and servers.
func (a *App) Init() error {
	if a.flagCfg.DebugMode {
		a.levelVar.Set(slog.LevelDebug)
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	hasEmbeddedUI := a.flagCfg.UIFS != nil
	if !a.flagCfg.UIDev && (!hasEmbeddedUI || a.flagCfg.UIPath != "") {
		indexPath := a.flagCfg.UIPath + "/index.html"
		if _, err := os.Stat(indexPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("UI index.html file does not exist at path '%s' (ensure you built the UI or use -ui-path)", indexPath)
			}
			return fmt.Errorf("UI index.html file is not readable at path '%s' (ensure you built the UI or use -ui-path): %w", indexPath, err)
		}
	}

	a.logger.LogAttrs(context.Background(), slog.LevelInfo, "starting RotomNG UI",
		slog.String("version", appVersion),
		slog.String("git_sha", gitSHA),
		slog.Int("instances", len(a.cfg.Instances)),
	)

	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.setShutdownTimeout(a.cfg.ShutdownTimeout)

	var err error
	a.instanceManager, err = instances.NewManager(instances.ManagerConfig{
		Logger:    a.logger.With(slog.String("component", "instances")),
		UserAgent: userAgent,
	}, getInstanceSettings(a.cfg))
	if err != nil {
		return fmt.Errorf("invalid instance settings: %w", err)
	}

	a.httpAPIHandlerConfig = handlers.HTTPAPIHandlerConfig{
		Logger:     a.logger.With(slog.String("component", "api")),
		AppVersion: appVersion,
		GitSHA:     gitSHA,
		Instances:  a.instanceManager,
		ReloadFn:   a.reload,
	}
	if err := a.httpAPIHandlerConfig.Init(getHTTPAPIHandlerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid http api handler config: %w", err)
	}

	apiProxy := proxy.New(proxy.Config{
		Logger:    a.logger.With(slog.String("component", "proxy")),
		Resolver:  a.instanceManager,
		UserAgent: userAgent,
	})

	a.httpAuthMiddleware = auth.NewMiddleware(a.cfg.HTTPListener.Secret)
	a.httpAuthMiddleware.SetSessionTTL(a.cfg.HTTPListener.UISessionTTL)

	a.httpServer, err = httpserver.NewHTTPServer(
		a.ctx,
		a.logger.With(slog.String("component", "http_server")),
		httpserver.Config{
			Address:        a.cfg.HTTPListener.Address,
			Listener:       a.cfg.HTTPListener.Listener,
			UIPath:         a.flagCfg.UIPath,
			UIFS:           a.flagCfg.UIFS,
			DevMode:        a.flagCfg.UIDev,
			AuthMiddleware: a.httpAuthMiddleware,
			APIHandler:     handlers.NewHTTPAPIHandler(a.httpAPIHandlerConfig),
			ProxyHandler:   apiProxy.Handler,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to setup http server: %w", err)
	}

	return nil
}

// Run starts the application servers and blocks until shutdown.
func (a *App) Run() {
	a.logger.LogAttrs(context.Background(), slog.LevelInfo, "Application startup complete")

	var wg sync.WaitGroup

	// reload handling
	wg.Go(func() {
		reloadChan := make(chan os.Signal, 1)
		signal.Notify(reloadChan, syscall.SIGHUP)
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-reloadChan:
			}

			a.logger.LogAttrs(context.Background(), slog.LevelInfo, "config reload requested")

			if err := a.reload(); err != nil {
				a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to reload config", slog.String("error", err.Error()))
				continue
			}

			a.logger.LogAttrs(context.Background(), slog.LevelInfo, "config reloaded")
		}
	})

	wg.Go(func() {
		defer a.cancel()
		a.httpServer.Run()
	})

	wg.Go(func() {
		a.instanceManager.Run(a.ctx)
	})

	// Wait for interrupt
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case <-c:
		a.cancel()
	case <-a.ctx.Done():
	}

	a.shutdown(&wg)
}

func (a *App) setShutdownTimeout(d time.Duration) {
	a.shutdownTimeout.Store(int64(d))
}

func (a *App) getShutdownTimeout() time.Duration {
	return time.Duration(a.shutdownTimeout.Load())
}

func (a *App) reload() error {
	cfg, err := a.flagCfg.ReloadConfig()
	if err != nil {
		return err
	}

	instanceSettings := getInstanceSettings(cfg)
	if err := instanceSettings.Validate(); err != nil {
		return err
	}

	httpAPIHandlerSettings := getHTTPAPIHandlerSettings(cfg)
	if err := httpAPIHandlerSettings.Validate(); err != nil {
		return err
	}

	// now apply the settings.
	if err := a.httpAPIHandlerConfig.PutSettings(httpAPIHandlerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "http_api_handler"), slog.String("error", err.Error()))
	}
	a.instanceManager.SetSettings(instanceSettings)
	a.httpAuthMiddleware.SetSecret(cfg.HTTPListener.Secret)
	a.httpAuthMiddleware.SetSessionTTL(cfg.HTTPListener.UISessionTTL)

	a.setShutdownTimeout(cfg.ShutdownTimeout)

	newLevel, err := logging.ParseSlogLevel(cfg.Logging.Level)
	if err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to set log level", slog.String("error", err.Error()))
	} else {
		a.levelVar.Set(newLevel)
	}

	return nil
}

func (a *App) shutdown(wg *sync.WaitGroup) {
	a.logger.LogAttrs(context.Background(), slog.LevelInfo, "shutting down")

	shutdownTimeout := a.getShutdownTimeout()

	shutdownCh := make(chan struct{})
	go func() {
		defer close(shutdownCh)
		timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer timeoutCancel()

		a.httpServer.Shutdown(timeoutCtx)
		wg.Wait()
	}()

	select {
	case <-shutdownCh:
		a.logger.LogAttrs(context.Background(), slog.LevelInfo, "shutdown complete")
	case <-time.After(shutdownTimeout + (100 * time.Millisecond)):
		a.logger.LogAttrs(context.Background(), slog.LevelInfo, "shutdown timed out")
	}

	if a.closer != nil {
		_ = a.closer.Close()
	}
}
