// Package handlers provides WebSocket and HTTP request handlers for RotomNG.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/UnownHash/RotomNG/libs/api"
	"github.com/UnownHash/RotomNG/libs/connections"
	"github.com/UnownHash/RotomNG/libs/errorutil"
	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/settings"
)

// Constants for repeated string literals used in JSON responses and log fields.
const (
	fieldStatus     = "status"
	fieldMessage    = "message"
	fieldError      = "error"
	fieldRemoteAddr = "remote_addr"
	fieldAction     = "action"
	fieldDevice     = "device"

	valStatusOK    = "ok"
	valStatusError = "error"

	msgActionNotAllDevices = "Action cannot be performed on all devices."
	msgJobsNotEnabled      = "jobs are not enabled"
	msgJobInstanceNotFound = "job instance not found"
	msgProfilingDisabled   = "profiling disabled"
)

// MITMWorker defines the interface for a MITM worker.
type MITMWorker interface {
	api.MITMWorker
	connections.MITMWorkerConstraint
}

// APIHandlerConfig holds configuration for the API handler.
type APIHandlerConfig[C Controller, W MITMWorker] struct {
	*apiHandlerSettingsContainer

	Logger            *slog.Logger
	ConnectionManager *connections.ConnectionManager[C, W]
	JobsManager       *jobs.Manager
	APIConverter      *api.Converter[*connections.Device[W], W, C]
}

// Init initializes the settings container with the given settings.
func (cfg *APIHandlerConfig[C, W]) Init(s APIHandlerSettings) (err error) {
	cfg.apiHandlerSettingsContainer, err = settings.NewContainer(s)
	return
}

// APIHandlerSettings holds settings for the API handler.
type APIHandlerSettings struct {
	ProfilingEnabled bool
	JobsEnabled      bool
}

// Validate validates the API handler settings.
func (s APIHandlerSettings) Validate() error {
	return nil
}

type apiHandlerSettingsContainer = settings.Container[APIHandlerSettings]

// APIHandler handles HTTP API requests for devices, controllers, jobs, and status.
type APIHandler[C Controller, W MITMWorker] struct {
	ctx               context.Context
	logger            *slog.Logger
	getSettings       func() APIHandlerSettings
	connectionManager *connections.ConnectionManager[C, W]
	jobsManager       *jobs.Manager
	apiConverter      *api.Converter[*connections.Device[W], W, C]
}

// NewAPIHandler creates a new APIHandler instance.
func NewAPIHandler[C Controller, W MITMWorker](ctx context.Context, cfg APIHandlerConfig[C, W]) *APIHandler[C, W] {
	return &APIHandler[C, W]{
		ctx:               ctx,
		logger:            cfg.Logger,
		getSettings:       cfg.GetSettings,
		connectionManager: cfg.ConnectionManager,
		jobsManager:       cfg.JobsManager,
		apiConverter:      cfg.APIConverter,
	}
}

func (ah *APIHandler[C, W]) jobsAreEnabled() bool {
	return ah.jobsManager != nil && ah.getSettings().JobsEnabled
}

// SetupAPIRoutes registers the standard API routes on the given router group.
func (ah *APIHandler[C, W]) SetupAPIRoutes(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/status", ah.GetStatus)

	apiGroup.GET("/controller", ah.GetControllers)
	controllerGroup := apiGroup.Group("/controller/:uuid")
	controllerGroup.GET("", ah.GetController)
	controllerGroup.PUT("/action/:action", ah.ControllerAction)

	apiGroup.GET("/device", ah.GetDevices)
	deviceGroup := apiGroup.Group("/device/:deviceID")
	deviceGroup.GET("", ah.GetDevice)
	deviceGroup.PUT("/action/:action", ah.DeviceAction)

	apiGroup.GET("/job", ah.GetJobs)
	jobGroup := apiGroup.Group("/job/:jobID")
	jobGroup.GET("", ah.GetJob)
	jobGroup.PUT("/run", ah.RunJob)
	jobGroup.PUT("/reload", ah.ReloadJobs)

	apiGroup.GET("/job-instance", ah.GetJobInstances)
	jobInstanceGroup := apiGroup.Group("/job-instance/:jobInstanceID")
	jobInstanceGroup.GET("", ah.GetJobInstance)
	jobInstanceGroup.PUT("/clear", ah.ClearJobInstance)

	// Debug routes
	debugGroup := apiGroup.Group("/debug")
	pprofGroup := debugGroup.Group("/pprof")
	pprofGroup.GET("/", ah.PprofIndex)
	pprofGroup.GET("/cmdline", ah.PprofCmdline)
	pprofGroup.GET("/profile", ah.PprofProfile)
	pprofGroup.GET("/symbol", ah.PprofSymbol)
	pprofGroup.GET("/trace", ah.PprofTrace)
}

// Device action methods

func (ah *APIHandler[C, W]) deleteDevice(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		devicesRemoved := ah.connectionManager.DeleteUnconnectedDevices()
		c.JSON(http.StatusOK, gin.H{
			fieldStatus:     "ok",
			fieldMessage:    "Removed dead connections",
			"devices_count": devicesRemoved,
		})
		return
	}
	err := ah.connectionManager.DeleteUnconnectedDeviceID(deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Device removed."})
}

func (ah *APIHandler[C, W]) restartDevice(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: msgActionNotAllDevices})
		return
	}

	err := ah.connectionManager.RestartDeviceApp(c.Request.Context(), deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Device app restarted."})
}

func (ah *APIHandler[C, W]) rebootDevice(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: msgActionNotAllDevices})
		return
	}

	err := ah.connectionManager.RebootDevice(c.Request.Context(), deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Device rebooted."})
}

func (ah *APIHandler[C, W]) getLogcat(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: msgActionNotAllDevices})
		return
	}

	zipData, err := ah.connectionManager.GetDeviceLogcat(c.Request.Context(), deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}

	filename := fmt.Sprintf("logcat-%s-%d.zip", deviceID, time.Now().Unix())
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Length", strconv.Itoa(len(zipData)))

	c.Data(http.StatusOK, "application/zip", zipData)
}

func (ah *APIHandler[C, W]) handleDeviceError(c *gin.Context, action string, deviceID string, err error) bool {
	if err == nil {
		return false
	}

	ah.logger.LogAttrs(c.Request.Context(), slog.LevelError, "device operation failed",
		slog.String(fieldRemoteAddr, c.ClientIP()),
		slog.String("device_id", deviceID),
		slog.String(fieldAction, action),
		slog.String(fieldError, err.Error()),
	)

	if errorutil.IsErrDeviceNotKnown(err) {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "Device not found"})
		return true
	}
	if errorutil.IsErrDeviceNotConnected(err) || errorutil.IsErrDeviceIsConnected(err) || errorutil.IsErrDeviceHasWorkersConnected(err) {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: err.Error()})
		return true
	}
	c.JSON(http.StatusInternalServerError, gin.H{fieldStatus: valStatusError, fieldError: err.Error()})
	return true
}

func (ah *APIHandler[C, W]) enableDevice(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: msgActionNotAllDevices})
		return
	}

	device, err := ah.connectionManager.EnableDevice(deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		fieldStatus:  "ok",
		fieldMessage: "Device enabled.",
		fieldDevice:  ah.apiConverter.NewDeviceFromDevice(device, true, false),
	})
}

func (ah *APIHandler[C, W]) disableDevice(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: msgActionNotAllDevices})
		return
	}

	device, err := ah.connectionManager.DisableDevice(deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		fieldStatus:  "ok",
		fieldMessage: "Device disabled.",
		fieldDevice:  ah.apiConverter.NewDeviceFromDevice(device, true, false),
	})
}

func (ah *APIHandler[C, W]) disconnectDevice(c *gin.Context, action, deviceID string) {
	if deviceID == "_" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: msgActionNotAllDevices})
		return
	}

	err := ah.connectionManager.DisconnectDevice(deviceID)
	if ah.handleDeviceError(c, action, deviceID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Device disconnected."})
}

// Controller action methods

func (ah *APIHandler[C, W]) disconnectController(c *gin.Context, action, controllerUUID string) {
	err := ah.connectionManager.DisconnectController(controllerUUID)
	if ah.handleControllerError(c, action, controllerUUID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Controller disconnected."})
}

func (ah *APIHandler[C, W]) reconnectController(c *gin.Context, action, controllerUUID string) {
	err := ah.connectionManager.ReconnectController(controllerUUID)
	if ah.handleControllerError(c, action, controllerUUID, err) {
		return
	}

	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Controller set to be told to reconnect/restart session."})
}

func (ah *APIHandler[C, W]) handleControllerError(c *gin.Context, action string, controllerUUID string, err error) bool {
	if err == nil {
		return false
	}

	ah.logger.LogAttrs(c.Request.Context(), slog.LevelError, "controller operation failed",
		slog.String(fieldRemoteAddr, c.ClientIP()),
		slog.String("controller_uuid", controllerUUID),
		slog.String(fieldAction, action),
		slog.String(fieldError, err.Error()),
	)

	if errorutil.IsErrControllerNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "Controller not found"})
		return true
	}
	c.JSON(http.StatusInternalServerError, gin.H{fieldStatus: valStatusError, fieldError: err.Error()})
	return true
}

// ControllerAction handles PUT /api/controller/:uuid/action/:action.
func (ah *APIHandler[C, W]) ControllerAction(c *gin.Context) {
	controllerUUID := c.Param("uuid")
	action := c.Param("action")

	ah.logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "controller action requested",
		slog.String(fieldRemoteAddr, c.ClientIP()),
		slog.String("controller_uuid", controllerUUID),
		slog.String(fieldAction, action),
	)

	switch action {
	case "disconnect":
		ah.disconnectController(c, action, controllerUUID)
	case "reconnect":
		ah.reconnectController(c, action, controllerUUID)
	default:
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "Action not found"})
	}
}

// Job handlers

// GetJobs handles GET /api/job.
func (ah *APIHandler[C, W]) GetJobs(c *gin.Context) {
	c.HandlerName()
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}
	jobs := ah.jobsManager.GetJobs()
	apiJobs := ah.apiConverter.ConnectionJobsToAPIJobs(jobs)
	c.JSON(http.StatusOK, gin.H{"jobs": apiJobs})
}

// GetJob handles GET /api/job/:jobId.
func (ah *APIHandler[C, W]) GetJob(c *gin.Context) {
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}
	jobID := c.Param("jobID")
	job, ok := ah.jobsManager.GetJobByID(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "job not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": ah.apiConverter.ConnectionJobToAPIJob(job)})
}

// ReloadJobs handles PUT /api/job/:jobId/reload.
func (ah *APIHandler[C, W]) ReloadJobs(c *gin.Context) {
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}

	jobID := c.Param("jobID")
	if jobID != "-" {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: "reloading a single job is not implemented"})
		return
	}

	logger := ah.logger.With(slog.String("remote_addr", c.Request.RemoteAddr))
	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "jobs reload requested")

	if err := ah.jobsManager.Reload(); err != nil {
		ah.logger.LogAttrs(c.Request.Context(), slog.LevelError, "failed to reload jobs", slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{fieldStatus: valStatusError, fieldError: err.Error()})
		return
	}

	logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "jobs reloaded")
	c.JSON(http.StatusOK, gin.H{fieldStatus: valStatusOK, fieldMessage: "Jobs reloaded successfully"})
}

// ClearJobInstance handles PUT /api/job-instance/:jobInstanceId/clear.
func (ah *APIHandler[C, W]) ClearJobInstance(c *gin.Context) {
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}
	jobInstanceIDStr := c.Param("jobInstanceID")
	if jobInstanceIDStr == "-" {
		ah.jobsManager.ClearJobInstances()
		c.JSON(http.StatusOK, gin.H{fieldStatus: "success"})
		return
	}

	jobInstanceID, err := strconv.ParseUint(jobInstanceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobInstanceNotFound})
		return
	}

	if ah.jobsManager.ClearJobInstance(jobInstanceID) {
		c.JSON(http.StatusOK, gin.H{fieldStatus: "success"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobInstanceNotFound})
	}
}

// GetJobInstances handles GET /api/job-instance.
func (ah *APIHandler[C, W]) GetJobInstances(c *gin.Context) {
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}
	jobInstances := ah.jobsManager.GetJobInstances()
	apiJobInstances := ah.apiConverter.NewJobInstancesFromJobInstances(jobInstances)
	c.JSON(http.StatusOK, gin.H{"instances": apiJobInstances})
}

// GetJobInstance handles GET /api/job-instance/:jobInstanceId.
func (ah *APIHandler[C, W]) GetJobInstance(c *gin.Context) {
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}

	jobInstanceIDStr := c.Param("jobInstanceID")
	jobInstanceID, err := strconv.ParseUint(jobInstanceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobInstanceNotFound})
		return
	}
	jobInstance, ok := ah.jobsManager.GetJobInstanceByID(jobInstanceID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobInstanceNotFound})
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": ah.apiConverter.NewJobInstanceFromJobInstance(jobInstance)})
}

// RunJob handles PUT /api/job/:jobId/run.
func (ah *APIHandler[C, W]) RunJob(c *gin.Context) {
	if !ah.jobsAreEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: msgJobsNotEnabled})
		return
	}

	jobID := c.Param("jobID")
	if jobID == "" {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "no job id"})
		return
	}

	_, ok := ah.jobsManager.GetJobByID(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "job not found"})
		return
	}

	jobRequest := struct {
		DeviceIDs []string `json:"device_ids"`
	}{}

	if err := c.ShouldBindJSON(&jobRequest); err != nil {
		c.JSON(
			http.StatusBadRequest,
			gin.H{fieldStatus: valStatusError, fieldError: fmt.Sprintf("failed to decode request: %v", err.Error())},
		)
		return
	}

	if len(jobRequest.DeviceIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{fieldStatus: valStatusError, fieldError: "no 'device_ids' in request"})
		return
	}

	jobInstances := make([]api.JobInstance, len(jobRequest.DeviceIDs))
	for idx, deviceID := range jobRequest.DeviceIDs {
		// jobs run async, so we need to use a Background context, not http request context.
		jobInstances[idx] = ah.apiConverter.NewJobInstanceFromJobInstance(
			ah.connectionManager.RunJob(context.Background(), jobID, deviceID, time.Minute),
		)
	}

	c.JSON(http.StatusOK, gin.H{"instances": jobInstances})
}

// GetStatus handles GET /api/status.
func (ah *APIHandler[C, W]) GetStatus(c *gin.Context) {
	status := ah.connectionManager.GetStatus()
	response := ah.apiConverter.StatusResponseFromDevicesAndControllers(status.Devices, status.Controllers)
	c.JSON(http.StatusOK, response)
}

// GetControllers handles GET /api/controller.
func (ah *APIHandler[C, W]) GetControllers(c *gin.Context) {
	mitmControllers := ah.connectionManager.GetControllers()
	controllers := make([]api.ControllerResponse, len(mitmControllers))
	for idx, mitmController := range mitmControllers {
		controllers[idx] = ah.apiConverter.NewControllerResponseFromController(mitmController)
	}
	c.JSON(http.StatusOK, gin.H{"controllers": controllers})
}

// GetController handles GET /api/controller/:uuid.
func (ah *APIHandler[C, W]) GetController(c *gin.Context) {
	uuid := c.Param("uuid")

	mitmController := ah.connectionManager.GetControllerByUUID(uuid)
	if mitmController.IsZero() {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "Controller not found"})
		return
	}

	controller := ah.apiConverter.NewControllerResponseFromController(mitmController)
	c.JSON(http.StatusOK, gin.H{"controller": controller})
}

// GetDevices handles GET /api/device.
func (ah *APIHandler[C, W]) GetDevices(c *gin.Context) {
	includeWorkers := c.Query("include_workers") == "true"

	connDevices := ah.connectionManager.GetDevices()
	devices := make([]api.Device, len(connDevices))
	for idx, connDevice := range connDevices {
		devices[idx] = ah.apiConverter.NewDeviceFromDevice(connDevice, true, includeWorkers)
	}
	c.JSON(http.StatusOK, gin.H{"devices": devices})
}

// GetDevice handles GET /api/device/:deviceID.
func (ah *APIHandler[C, W]) GetDevice(c *gin.Context) {
	deviceID := c.Param("deviceID")
	includeWorkers := c.Query("include_workers") == "true"

	connDevice := ah.connectionManager.GetDeviceByID(deviceID)
	if connDevice == nil {
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "Device not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{fieldDevice: ah.apiConverter.NewDeviceFromDevice(connDevice, true, includeWorkers)})
}

// DeviceAction handles PUT /api/device/:deviceID/action/:action.
func (ah *APIHandler[C, W]) DeviceAction(c *gin.Context) {
	deviceID := c.Param("deviceID")
	action := c.Param("action")

	ah.logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "device action requested",
		slog.String(fieldRemoteAddr, c.ClientIP()),
		slog.String("device_id", deviceID),
		slog.String(fieldAction, action),
	)

	switch action {
	case "delete":
		ah.deleteDevice(c, action, deviceID)
	case "restart":
		ah.restartDevice(c, action, deviceID)
	case "reboot":
		ah.rebootDevice(c, action, deviceID)
	case "logcat":
		ah.getLogcat(c, action, deviceID)
	case "disable":
		ah.disableDevice(c, action, deviceID)
	case "enable":
		ah.enableDevice(c, action, deviceID)
	case "disconnect":
		ah.disconnectDevice(c, action, deviceID)
	default:
		c.JSON(http.StatusNotFound, gin.H{fieldStatus: valStatusError, fieldError: "Action not found"})
	}
}

// Pprof handlers

func (ah *APIHandler[C, W]) isProfilingEnabled() bool {
	settings := ah.getSettings()
	return settings.ProfilingEnabled
}

// PprofIndex handles GET /api/debug/pprof/.
func (ah *APIHandler[C, W]) PprofIndex(c *gin.Context) {
	if !ah.isProfilingEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldError: msgProfilingDisabled})
		return
	}
	pprof.Index(c.Writer, c.Request)
}

// PprofCmdline handles GET /api/debug/pprof/cmdline.
func (ah *APIHandler[C, W]) PprofCmdline(c *gin.Context) {
	if !ah.isProfilingEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldError: msgProfilingDisabled})
		return
	}
	pprof.Cmdline(c.Writer, c.Request)
}

// PprofProfile handles GET /api/debug/pprof/profile.
func (ah *APIHandler[C, W]) PprofProfile(c *gin.Context) {
	if !ah.isProfilingEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldError: msgProfilingDisabled})
		return
	}
	pprof.Profile(c.Writer, c.Request)
}

// PprofSymbol handles GET /api/debug/pprof/symbol.
func (ah *APIHandler[C, W]) PprofSymbol(c *gin.Context) {
	if !ah.isProfilingEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldError: msgProfilingDisabled})
		return
	}
	pprof.Symbol(c.Writer, c.Request)
}

// PprofTrace handles GET /api/debug/pprof/trace.
func (ah *APIHandler[C, W]) PprofTrace(c *gin.Context) {
	if !ah.isProfilingEnabled() {
		c.JSON(http.StatusNotFound, gin.H{fieldError: msgProfilingDisabled})
		return
	}
	pprof.Trace(c.Writer, c.Request)
}
