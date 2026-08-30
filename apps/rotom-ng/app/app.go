// Package app implements the RotomNG application server.
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

	"github.com/UnownHash/RotomNG/libs/api"
	"github.com/UnownHash/RotomNG/libs/auth"
	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/connections"
	"github.com/UnownHash/RotomNG/libs/controller"
	"github.com/UnownHash/RotomNG/libs/gitutil"
	"github.com/UnownHash/RotomNG/libs/handlers"
	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/selector"
	"github.com/UnownHash/RotomNG/libs/services"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/factories"
	app_handlers "github.com/UnownHash/RotomNG/apps/rotom-ng/app/handlers"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/httpserver"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/stats"
	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/version"
)

// Controller is a type alias for the controller connection type.
type Controller = controller.Controller

// MITMWorker is a type alias for the MITM worker type.
type MITMWorker = mitm.Worker

var gitSHA = gitutil.GetGitBuildSHA()

const (
	userAgent  = "RotomNG/" + version.AppVersion
	appVersion = version.AppVersion
)

// Helper functions to extract Settings structs from Config

func getSelectorSettings(cfg *config.Config) selector.Settings {
	var selectorSettings selector.Settings
	if cfg.RateLimit.Enable {
		selectorSettings.DeviceRateLimit = selector.SelectionHistoryConfig{
			Enabled:       true,
			MaxSelections: cfg.RateLimit.MaxSelectionsPerDuration,
			Duration:      cfg.RateLimit.Duration,
		}
	}
	return selectorSettings
}

func getConnectionManagerSettings(cfg *config.Config) connections.ConnectionManagerSettings {
	return connections.ConnectionManagerSettings{
		DisableWorkerStats: cfg.Tuning.DisableWorkerStats,
	}
}

func getControllerHandlerSettings(cfg *config.Config) handlers.ControllerHandlerSettings {
	var dataTimeout time.Duration
	if cfg.ControllerListener.DataTimeout != nil {
		dataTimeout = *cfg.ControllerListener.DataTimeout
	}
	return handlers.ControllerHandlerSettings{
		PingInterval:        cfg.ControllerListener.PingInterval,
		PongWait:            cfg.ControllerListener.PongWait,
		RegistrationTimeout: cfg.ControllerListener.RegistrationTimeout,
		DataTimeout:         dataTimeout,
	}
}

func getDeviceHandlerSettings(cfg *config.Config) handlers.DeviceHandlerSettings {
	return handlers.DeviceHandlerSettings{
		PingInterval: cfg.DeviceListener.PingInterval,
		PongWait:     cfg.DeviceListener.PongWait,
	}
}

// getWorkerHandlerSettings sources the MITM worker ping read-timeout settings
// from the device listener, since workers connect on the device listener.
func getWorkerHandlerSettings(cfg *config.Config) app_handlers.WorkerHandlerSettings {
	return app_handlers.WorkerHandlerSettings{
		PingInterval: cfg.DeviceListener.PingInterval,
		PongWait:     cfg.DeviceListener.PongWait,
	}
}

func getJobsManagerSettings(cfg *config.Config) jobs.ManagerSettings {
	return jobs.ManagerSettings{
		JobsPath: cfg.Jobs.Path,
	}
}

func getHTTPAPIHandlerSettings(cfg *config.Config) app_handlers.HTTPAPIHandlerSettings {
	return app_handlers.HTTPAPIHandlerSettings{
		CurrentConfig: *cfg,
	}
}

func getBaseAPIHandlerSettings(cfg *config.Config) handlers.APIHandlerSettings {
	return handlers.APIHandlerSettings{
		ProfilingEnabled: cfg.Tuning.Profiling,
		JobsEnabled:      cfg.Jobs.Enable,
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

// App is the main RotomNG application, managing all servers and connections.
type App struct {
	cfg      *config.Config
	flagCfg  FlagConfig
	logger   *slog.Logger
	levelVar *slog.LevelVar
	closer   io.Closer

	shutdownTimeout atomic.Int64 // nanoseconds

	// dependencies initialized in Init()
	ctx    context.Context
	cancel context.CancelFunc

	bufferPool     *bufferpool.BufferPool
	statsCollector *stats.PromStatsCollector

	selectorConfig          selector.Config
	connectionManagerConfig connections.ConnectionManagerConfig[*Controller, *MITMWorker]
	deviceHandlerConfig     handlers.DeviceHandlerConfig
	workerHandlerConfig     app_handlers.WorkerHandlerConfig
	controllerHandlerConfig handlers.ControllerHandlerConfig[*Controller]
	jobsManagerConfig       jobs.ManagerConfig
	httpAPIHandlerConfig    app_handlers.HTTPAPIHandlerConfig
	apiHandlerConfig        handlers.APIHandlerConfig[*Controller, *MITMWorker]

	connectionManager *connections.ConnectionManager[*Controller, *MITMWorker]
	jobsManager       *jobs.Manager

	deviceAuthMiddleware     *auth.Middleware
	controllerAuthMiddleware *auth.Middleware
	httpAuthMiddleware       *auth.Middleware

	deviceHandler     *handlers.DeviceHandler
	workerHandler     *app_handlers.WorkerHandler
	controllerHandler *handlers.ControllerHandler[*Controller]

	deviceServer     *services.DeviceServer
	controllerServer *services.ControllerServer
	httpServer       *httpserver.HTTPServer
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

// Run starts the application servers and blocks until shutdown.
func (a *App) Run() {
	a.statsCollector.IncrAppStartups(appVersion)
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

			a.statsCollector.IncrConfigReloads(appVersion)
			a.logger.LogAttrs(context.Background(), slog.LevelInfo, "config reloaded")
		}
	})

	for _, server := range []interface{ Run() }{a.deviceServer, a.controllerServer, a.httpServer} {
		wg.Go(func() {
			defer a.cancel()
			server.Run()
		})
	}

	wg.Go(func() {
		a.connectionManager.RunPeriodicTasks(a.ctx)
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

// Cancel cancels the application context, triggering a graceful shutdown.
// This is intended for use by test utilities that need to stop the app
// from outside the app package.
func (a *App) Cancel() {
	if a.cancel != nil {
		a.cancel()
	}
}

// Logger returns the application's logger instance.
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// Init initializes all application dependencies and servers.
func (a *App) Init() error {
	if a.flagCfg.DebugMode {
		a.levelVar.Set(slog.LevelDebug)
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	if err := a.checkUIAssets(); err != nil {
		return err
	}

	a.logger.LogAttrs(context.Background(), slog.LevelInfo, "starting RotomNG", slog.String("version", appVersion), slog.String("git_sha", gitSHA))

	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.setShutdownTimeout(a.cfg.ShutdownTimeout)

	a.bufferPool = bufferpool.New(8 * 1024)
	a.statsCollector = stats.NewPromStatsCollector(a.cfg.Prometheus.Namespace)

	// Shared aggregate of request stats across all workers. Workers (via the
	// worker handler) record into it; the API handler reads from it for the
	// status reply, so the totals stay accurate even as workers disconnect.
	globalRequestStats := mitm.NewRequestStatsCollector()

	selectorSettings := getSelectorSettings(a.cfg)
	a.selectorConfig = selector.Config{}
	if err := a.selectorConfig.Init(selectorSettings); err != nil {
		return fmt.Errorf("invalid selector config: %w", err)
	}

	workerSelector := selector.NewBalancedSelector[*MITMWorker](a.selectorConfig)

	a.jobsManagerConfig = jobs.ManagerConfig{
		Logger: a.logger.With(slog.String("component", "jobs_manager")),
	}
	if err := a.jobsManagerConfig.Init(getJobsManagerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid jobs manager config: %w", err)
	}

	a.jobsManager = jobs.NewManager(a.jobsManagerConfig)
	if err := a.jobsManager.Reload(); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelWarn, "failed to load jobs (continuing with no jobs)", slog.String("error", err.Error()))
	}

	connectionManagerSettings := getConnectionManagerSettings(a.cfg)
	a.connectionManagerConfig = connections.ConnectionManagerConfig[*Controller, *MITMWorker]{
		Logger:         a.logger,
		WorkerSelector: workerSelector,
		JobsRunner:     a.jobsManager,
		StatsCollector: a.statsCollector,
		NewController:  factories.NewControllerFactory(),
		UserAgent:      userAgent,
	}
	if err := a.connectionManagerConfig.Init(connectionManagerSettings); err != nil {
		return fmt.Errorf("invalid connection manager config: %w", err)
	}

	a.connectionManager = connections.NewConnectionManager(a.connectionManagerConfig)

	deviceMonitorConfig := mitm.DeviceMonitorConfig{}
	if err := deviceMonitorConfig.Init(mitm.GetDeviceMonitorDefaultSettings()); err != nil {
		return fmt.Errorf("invalid device monitor config: %w", err)
	}

	a.deviceHandlerConfig = handlers.DeviceHandlerConfig{
		Logger:              a.logger,
		BufferPool:          a.bufferPool,
		ConnectionManager:   a.connectionManager,
		StatsCollector:      a.statsCollector,
		DeviceMonitorConfig: deviceMonitorConfig,
	}
	if err := a.deviceHandlerConfig.Init(getDeviceHandlerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid device handler config: %w", err)
	}
	a.deviceHandler = handlers.NewDeviceHandler(a.ctx, a.deviceHandlerConfig)

	a.workerHandlerConfig = app_handlers.WorkerHandlerConfig{
		Logger:                   a.logger,
		BufferPool:               a.bufferPool,
		MITMWorkerStatsCollector: a.statsCollector,
		ConnectionManager:        a.connectionManager,
		StatsCollector:           a.statsCollector,
		GlobalRequestStats:       globalRequestStats,
	}
	if err := a.workerHandlerConfig.Init(getWorkerHandlerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid worker handler config: %w", err)
	}
	a.workerHandler = app_handlers.NewWorkerHandler(a.ctx, a.workerHandlerConfig)

	a.deviceAuthMiddleware = auth.NewMiddleware(a.cfg.DeviceListener.Secret)
	deviceServerConfig := services.DeviceServerConfig{
		Address:        a.cfg.DeviceListener.Address,
		Listener:       a.cfg.DeviceListener.Listener,
		AuthMiddleware: a.deviceAuthMiddleware,
		DeviceHandler:  a.deviceHandler,
		WorkerHandler:  a.workerHandler,
	}
	var err error
	a.deviceServer, err = services.NewDeviceServer(
		a.ctx,
		a.logger.With(slog.String("component", "device_server")),
		deviceServerConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to create device server: %w", err)
	}

	a.controllerHandlerConfig = handlers.ControllerHandlerConfig[*Controller]{
		Logger:            a.logger,
		ConnectionManager: a.connectionManager,
		BufferPool:        a.bufferPool,
		StatsCollector:    a.statsCollector,
	}
	if err := a.controllerHandlerConfig.Init(getControllerHandlerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid controller handler config: %w", err)
	}
	a.controllerHandler = handlers.NewControllerHandler(a.ctx, a.controllerHandlerConfig)

	a.controllerAuthMiddleware = auth.NewMiddleware(a.cfg.ControllerListener.Secret)
	controllerServerConfig := services.ControllerServerConfig{
		Address:        a.cfg.ControllerListener.Address,
		Listener:       a.cfg.ControllerListener.Listener,
		AuthMiddleware: a.controllerAuthMiddleware,
	}
	a.controllerServer, err = services.NewControllerServer(
		a.ctx,
		a.logger.With(slog.String("component", "controller_server")),
		controllerServerConfig,
		a.controllerHandler,
	)
	if err != nil {
		return fmt.Errorf("failed to create controller server: %w", err)
	}

	a.httpAuthMiddleware = auth.NewMiddleware(a.cfg.HTTPListener.Secret)
	a.httpAuthMiddleware.SetSessionTTL(a.cfg.HTTPListener.UISessionTTL)
	a.apiHandlerConfig = handlers.APIHandlerConfig[*Controller, *MITMWorker]{
		Logger:             a.logger.With(slog.String("component", "api")),
		ConnectionManager:  a.connectionManager,
		JobsManager:        a.jobsManager,
		APIConverter:       api.NewConverter[*connections.Device[*MITMWorker], *MITMWorker, *Controller](),
		GlobalRequestStats: globalRequestStats,
	}
	if err := a.apiHandlerConfig.Init(getBaseAPIHandlerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid api handler config: %w", err)
	}

	a.httpAPIHandlerConfig = app_handlers.HTTPAPIHandlerConfig{
		Logger:     a.logger.With(slog.String("component", "api")),
		APIHandler: handlers.NewAPIHandler(a.ctx, a.apiHandlerConfig),
		AppVersion: appVersion,
		GitSHA:     gitSHA,
		ReloadFn:   a.reload,
	}
	if err := a.httpAPIHandlerConfig.Init(getHTTPAPIHandlerSettings(a.cfg)); err != nil {
		return fmt.Errorf("invalid http api handler config: %w", err)
	}

	httpServerConfig := httpserver.Config{
		Address:        a.cfg.HTTPListener.Address,
		Listener:       a.cfg.HTTPListener.Listener,
		UIPath:         a.flagCfg.UIPath,
		UIFS:           a.flagCfg.UIFS,
		DevMode:        a.flagCfg.UIDev,
		MetricsHandler: a.statsCollector,
		AuthMiddleware: a.httpAuthMiddleware,
		APIHandler:     app_handlers.NewHTTPAPIHandler(a.httpAPIHandlerConfig),
		StatsRegistrar: a.statsCollector,
	}

	a.httpServer, err = httpserver.NewHTTPServer(
		a.ctx,
		a.logger.With(slog.String("component", "http_server")),
		httpServerConfig,
	)
	if err != nil {
		return fmt.Errorf("failed to setup http server: %w", err)
	}

	return nil
}

// checkUIAssets fails startup when the UI is going to be served but its bundle
// is not there, which is otherwise a confusing 404 at first page load.
//
// The assets only have to be present if the UI will actually be served.
// Starting with http_listener.disable_ui set is a supported way to run an
// API-only listener, so a missing bundle is not an error then. Switching the UI
// back on at runtime does not re-check: the static handler simply 404s until
// the assets are in place.
func (a *App) checkUIAssets() error {
	if a.cfg.HTTPListener.DisableUI || a.flagCfg.UIDev {
		return nil
	}
	// An embedded bundle is always present, unless overridden by -ui-path.
	if a.flagCfg.UIFS != nil && a.flagCfg.UIPath == "" {
		return nil
	}

	indexPath := a.flagCfg.UIPath + "/index.html"
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("UI index.html file does not exist at path '%s' (ensure you built the UI or use -ui-path)", indexPath)
		}
		return fmt.Errorf("UI index.html file is not readable at path '%s' (ensure you built the UI or use -ui-path): %w", indexPath, err)
	}
	return nil
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

	connManagerSettings := getConnectionManagerSettings(cfg)
	if err := connManagerSettings.Validate(); err != nil {
		return err
	}

	selectorSettings := getSelectorSettings(cfg)
	if err := selectorSettings.Validate(); err != nil {
		return err
	}

	jobManagerSettings := getJobsManagerSettings(cfg)
	if err := jobManagerSettings.Validate(); err != nil {
		return err
	}

	deviceHandlerSettings := getDeviceHandlerSettings(cfg)
	if err := deviceHandlerSettings.Validate(); err != nil {
		return err
	}

	workerHandlerSettings := getWorkerHandlerSettings(cfg)
	if err := workerHandlerSettings.Validate(); err != nil {
		return err
	}

	controllerHandlerSettings := getControllerHandlerSettings(cfg)
	if err := controllerHandlerSettings.Validate(); err != nil {
		return err
	}

	apiHandlerSettings := getBaseAPIHandlerSettings(cfg)
	if err := apiHandlerSettings.Validate(); err != nil {
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
	if err := a.apiHandlerConfig.PutSettings(apiHandlerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "api_handler"), slog.String("error", err.Error()))
	}
	if err := a.connectionManagerConfig.PutSettings(connManagerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "connection_manager"), slog.String("error", err.Error()))
	}
	if err := a.selectorConfig.PutSettings(selectorSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "selector"), slog.String("error", err.Error()))
	}
	if err := a.jobsManagerConfig.PutSettings(jobManagerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "jobs_manager"), slog.String("error", err.Error()))
	}
	if err := a.deviceHandlerConfig.PutSettings(deviceHandlerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "device_handler"), slog.String("error", err.Error()))
	}
	if err := a.workerHandlerConfig.PutSettings(workerHandlerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "worker_handler"), slog.String("error", err.Error()))
	}
	if err := a.controllerHandlerConfig.PutSettings(controllerHandlerSettings); err != nil {
		a.logger.LogAttrs(context.Background(), slog.LevelError, "failed to apply settings", slog.String("component", "controller_handler"), slog.String("error", err.Error()))
	}
	a.controllerAuthMiddleware.SetSecret(cfg.ControllerListener.Secret)
	a.deviceAuthMiddleware.SetSecret(cfg.DeviceListener.Secret)
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

		var shutdownWg sync.WaitGroup
		for _, server := range []interface {
			Shutdown(ctx context.Context)
		}{a.deviceServer, a.controllerServer, a.httpServer} {
			shutdownWg.Go(func() {
				server.Shutdown(timeoutCtx)
			})
		}
		shutdownWg.Wait()

		a.deviceHandler.Wait()
		a.workerHandler.Wait()
		a.controllerHandler.Wait()

		wg.Wait()
		a.connectionManager.Wait()
		a.jobsManager.Wait()
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
