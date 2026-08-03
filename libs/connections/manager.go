// Package connections manages the relationships between devices, workers, and controllers.
package connections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/errorutil"
	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/settings"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// Log field key constants.
const (
	fieldDeviceID = "device_id"
	fieldOrigin   = "origin"
)

// Controller weight and protocol version constants.
const (
	MinControllerWeight     = 1
	MaxControllerWeight     = 10
	DefaultControllerWeight = 5

	ProtoMajorVersion = 2
	ProtoMinorVersion = 0
)

type mitmDevice = MITMDevice

// Device wraps a MITMDevice with its associated workers.
type Device[W MITMWorker] struct {
	*mitmDevice

	workers []W
}

// Workers returns the list of workers associated with this device.
func (device *Device[W]) Workers() []W {
	return device.workers
}

// ConnectionManagerSettings holds configuration for the connection manager.
type ConnectionManagerSettings struct {
	DisableWorkerStats bool
}

// Validate checks that the settings are valid.
func (s ConnectionManagerSettings) Validate() error {
	return nil
}

type connectionManagerSettingsContainer = settings.Container[ConnectionManagerSettings]

// ConnectionManagerConfig provides the dependencies for creating a ConnectionManager.
type ConnectionManagerConfig[C Controller, W MITMWorkerConstraint] struct {
	*connectionManagerSettingsContainer

	Logger         *slog.Logger
	StatsCollector StatsCollector
	JobsRunner     JobsRunner

	WorkerSelector WorkerSelector[W]
	NewController  NewControllerFunc[C]

	UserAgent string
}

// Init initializes the settings container with the given settings.
func (cfg *ConnectionManagerConfig[C, W]) Init(s ConnectionManagerSettings) (err error) {
	cfg.connectionManagerSettingsContainer, err = settings.NewContainer(s)
	return
}

// ConnectionManager coordinates all device, worker, and controller relationships.
type ConnectionManager[C Controller, W MITMWorkerConstraint] struct {
	mu     sync.Mutex
	logger *slog.Logger
	wg     sync.WaitGroup

	getSettings    func() ConnectionManagerSettings
	workerSelector WorkerSelector[W]
	jobsRunner     JobsRunner
	statsCollector StatsCollector
	newController  NewControllerFunc[C]

	userAgent string

	allDevicesByID    map[string]*MITMDevice
	workersByDeviceID map[string]map[string]W
	controllers       map[string]C
}

func (mgr *ConnectionManager[C, W]) runInBackground(fn func()) {
	mgr.wg.Go(fn)
}

// must be called with mgr.mu locked.
func (mgr *ConnectionManager[C, W]) makeDevice(device *MITMDevice) *Device[W] {
	workers := mgr.workersByDeviceID[device.ID()]
	workerSlice := make([]W, len(workers))
	idx := 0
	for _, w := range workers {
		workerSlice[idx] = w
		idx++
	}
	return &Device[W]{
		mitmDevice: device.Clone(),
		workers:    workerSlice,
	}
}

// must be called with mgr.mu locked.
func (mgr *ConnectionManager[C, W]) getDevices() []*Device[W] {
	devices := make([]*Device[W], 0, len(mgr.allDevicesByID))
	for _, device := range mgr.allDevicesByID {
		devices = append(devices, mgr.makeDevice(device))
	}
	return devices
}

// must be called with mgr.mu locked.
func (mgr *ConnectionManager[C, W]) getControllers() []C {
	controllers := make([]C, len(mgr.controllers))
	idx := 0
	for _, controller := range mgr.controllers {
		controllers[idx] = controller
		idx++
	}
	return controllers
}

// must be called with mgr.mu locked.
func (mgr *ConnectionManager[C, W]) addNewDevice(deviceID string, origin string) *MITMDevice {
	device := mitm.NewDevice(deviceID, origin)
	mgr.statsCollector.IncrDevicesTotal(origin)
	mgr.allDevicesByID[deviceID] = device
	return device
}

// must be called with mgr.mu NOT locked.
func (mgr *ConnectionManager[C, W]) getDeviceConn(deviceID string, forCommand bool) (*mitm.DeviceConn, error) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		return nil, errorutil.NewErrDeviceNotKnown()
	}
	deviceConn := device.Conn()
	if deviceConn == nil {
		return nil, errorutil.NewErrDeviceNotConnected()
	}
	if forCommand && !deviceConn.CanRunCommands() {
		return nil, errorutil.NewErrDeviceCommandsUnavailable()
	}
	return deviceConn, nil
}

func (mgr *ConnectionManager[C, W]) getDeviceConnForCommand(deviceID string) (*mitm.DeviceConn, error) {
	return mgr.getDeviceConn(deviceID, true)
}

func (mgr *ConnectionManager[C, W]) writeRegisterControllerResponse(ctx context.Context, wsConn ControllerWSConn, response *protos.RegisterControllerResponse) error {
	data, err := proto.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal RegisterControllerResponse: %w", err)
	}
	if err = wsConn.WriteAsync(ctx, ws.MessageBinary, data); err != nil {
		return fmt.Errorf("failed to send RegisterControllerResponse: %w", err)
	}

	return nil
}

func (mgr *ConnectionManager[C, W]) getAvailableWorker(weight int) (W, error) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return mgr.workerSelector.GetAvailableWorker(weight)
}

func (mgr *ConnectionManager[C, W]) handleControllerRegistrationRequest(ctx context.Context, wsConn ControllerWSConn, registerRequest *protos.RegisterControllerRequest, userAgent string) (C, error) {
	var zeroController C

	logger := logging.LoggerFromContextOrDefault(ctx, mgr.logger)

	if registerRequest.GetProtoMajorVersion() != ProtoMajorVersion {
		logger.LogAttrs(ctx, slog.LevelError, "major version mismatch during registration",
			slog.String("controller_id", registerRequest.GetId()),
			slog.Int("controller_major_version", int(registerRequest.GetProtoMajorVersion())),
			slog.Int("controller_minor_version", int(registerRequest.GetProtoMinorVersion())),
			slog.String("controller_user_agent", userAgent),
			slog.Int("rotom_major_version", ProtoMajorVersion),
			slog.Int("rotom_minor_version", ProtoMinorVersion),
		)
		return zeroController, errors.New("major version mismatch")
	}

	response := &protos.RegisterControllerResponse{
		UserAgent:         mgr.userAgent,
		ProtoMajorVersion: int32(ProtoMajorVersion),
		ProtoMinorVersion: int32(ProtoMinorVersion),
	}

	weight := int(registerRequest.Weight)
	if weight < MinControllerWeight {
		weight = DefaultControllerWeight
	} else if weight > MaxControllerWeight {
		weight = MaxControllerWeight
	}

	worker, err := mgr.getAvailableWorker(weight)
	if err != nil {
		errStr := err.Error()
		response.Status = protos.RegisterControllerResponse_NO_WORKERS_AVAILABLE
		response.StatusReason = errStr

		if writeErr := mgr.writeRegisterControllerResponse(ctx, wsConn, response); writeErr != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed to write register controller response", slog.String("error", writeErr.Error()))
		}

		ctx, cancelFn := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFn()
		_ = wsConn.Flush(ctx)
		_ = wsConn.Close(errorutil.CloseCodeNoMITMWorkersAvailable, errStr)

		return zeroController, fmt.Errorf("failed to register controller: %w", err)
	}

	response.Status = protos.RegisterControllerResponse_SUCCESS
	response.AssignedWorkerId = worker.ID()

	err = mgr.writeRegisterControllerResponse(ctx, wsConn, response)
	if err != nil {
		mgr.workerSelector.SetWorkerAvailable(worker)
		return zeroController, fmt.Errorf("failed to register controller: %w", err)
	}

	mitmLoginRequest, err := mgr.readLoginRequest(ctx, wsConn)
	if err != nil {
		mgr.workerSelector.SetWorkerAvailable(worker)
		return zeroController, fmt.Errorf("failed to register controller: %w", err)
	}

	controller := mgr.newController(
		wsConn,
		registerRequest.Id,
		mitmLoginRequest,
		worker,
		weight,
		userAgent,
		mgr.getSettings().DisableWorkerStats,
		int(registerRequest.ProtoMajorVersion),
		int(registerRequest.ProtoMinorVersion),
	)

	return controller, nil
}

func (mgr *ConnectionManager[C, W]) registerController(controller C) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// Generate UUID and handle collisions
	for {
		controllerUUID := uuid.New().String()
		if _, exists := mgr.controllers[controllerUUID]; !exists {
			controller.SetUUID(controllerUUID)
			mgr.controllers[controllerUUID] = controller
			break
		}
		// If UUID collision occurs, loop will generate a new one
	}

	controller.SetCloseHandler(func() {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		delete(mgr.controllers, controller.UUID())
	})
}

func (mgr *ConnectionManager[C, W]) pruneSelectorWorkerIDs() {
	// ensure periodic task runner doesn't die
	defer func() {
		r := recover()
		if r != nil {
			mgr.logger.LogAttrs(context.Background(), slog.LevelError, "PruneWorkerIDsSeen panicked", slog.Any("panic", r))
			return
		}
	}()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	mgr.workerSelector.PruneWorkerIDsSeen(10 * time.Minute)
}

func (mgr *ConnectionManager[C, W]) readLoginRequest(ctx context.Context, wsConn ControllerWSConn) (*protos.MitmRequest, error) {
	reader, err := wsConn.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read login request: %w", err)
	}
	defer reader.Done()

	var mitmRequest protos.MitmRequest
	if err := proto.Unmarshal(reader.Bytes(), &mitmRequest); err != nil {
		return nil, fmt.Errorf("failed to decode MitmRequest to get login request: %w", err)
	}
	loginRequest := mitmRequest.GetLoginRequest()
	if loginRequest == nil {
		return nil, fmt.Errorf("expected login request, but got '%s'",
			protos.MITMRequestMethodName(mitmRequest.Method),
		)
	}
	if loginRequest.WorkerId == "" {
		return nil, errors.New("login request requires a worker id")
	}
	return &mitmRequest, nil
}

func (mgr *ConnectionManager[C, W]) readControllerRegistrationRequest(ctx context.Context, wsConn ControllerWSConn) (*protos.RegisterControllerRequest, error) {
	reader, err := wsConn.Reader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read initial controller message: %w", err)
	}
	defer reader.Done()

	var registerRequest protos.RegisterControllerRequest

	if err := proto.Unmarshal(reader.Bytes(), &registerRequest); err != nil {
		return nil, fmt.Errorf("failure decoding controller registration request: %w", err)
	}

	if registerRequest.Id == "" {
		return nil, errors.New("controller registration request requires an id")
	}

	return &registerRequest, nil
}

// RegisterControllerConnectionV1 registers a v1 controller connection.
func (mgr *ConnectionManager[C, W]) RegisterControllerConnectionV1(ctx context.Context, wsConn ControllerWSConn, weight int, userAgent string) (C, error) {
	var zeroController C

	mitmLoginRequest, err := mgr.readLoginRequest(ctx, wsConn)
	if err != nil {
		return zeroController, fmt.Errorf("failed to register controller: %w", err)
	}

	if weight < MinControllerWeight {
		weight = DefaultControllerWeight
	} else if weight > MaxControllerWeight {
		weight = MaxControllerWeight
	}

	worker, err := mgr.getAvailableWorker(weight)
	if err != nil {
		_ = wsConn.Close(errorutil.CloseCodeNoMITMWorkersAvailable, err.Error())
		return zeroController, err
	}

	controller := mgr.newController(
		wsConn,
		mitmLoginRequest.GetLoginRequest().GetWorkerId(),
		mitmLoginRequest,
		worker,
		weight,
		userAgent,
		mgr.getSettings().DisableWorkerStats,
		1, 0, // major, minor controller version
	)

	mgr.registerController(controller)

	return controller, nil
}

// RegisterControllerConnectionV2 registers a v2 controller connection.
func (mgr *ConnectionManager[C, W]) RegisterControllerConnectionV2(ctx context.Context, wsConn ControllerWSConn, userAgent string) (C, error) {
	var zeroController C

	registerRequest, err := mgr.readControllerRegistrationRequest(ctx, wsConn)
	if err != nil {
		return zeroController, fmt.Errorf("failed to register controller: %w", err)
	}

	controller, err := mgr.handleControllerRegistrationRequest(ctx, wsConn, registerRequest, userAgent)
	if err != nil {
		return zeroController, err
	}

	mgr.registerController(controller)

	return controller, nil
}

// RegisterDeviceConnection registers a new device control connection.
func (mgr *ConnectionManager[C, W]) RegisterDeviceConnection(ctx context.Context, wsConn mitm.DeviceWSConn) (*mitm.DeviceConn, error) {
	statsCollector := mgr.statsCollector

	initMsg, err := mitm.ReadDeviceControlInitMessage(
		ctx,
		wsConn,
	)
	if err != nil {
		statsCollector.IncrDeviceRegistrationFails()
		_ = wsConn.Close(errorutil.CloseCodeProtocolError, "registration failed")
		return nil, fmt.Errorf("failed to read device control init message: %w", err)
	}

	deviceID := initMsg.DeviceID
	origin := initMsg.Origin

	if deviceID == "" {
		statsCollector.IncrDeviceRegistrationFails()
		_ = wsConn.Close(errorutil.CloseCodeProtocolError, "registration failed")
		return nil, errors.New("no device id in device control init message")
	}
	if origin == "" {
		statsCollector.IncrDeviceRegistrationFails()
		_ = wsConn.Close(errorutil.CloseCodeProtocolError, "registration failed")
		return nil, errors.New("no device origin in device control init message")
	}

	logger := logging.LoggerFromContextOrDefault(ctx, mgr.logger).With(slog.String(fieldDeviceID, deviceID))

	statsCollector.IncrDeviceRegistrations(origin)
	statsCollector.IncrDevicesConnected(origin)

	logger.LogAttrs(ctx, slog.LevelInfo, "registered mitm device control",
		slog.String(fieldDeviceID, deviceID),
		slog.String(fieldOrigin, origin),
	)

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		device = mgr.addNewDevice(deviceID, initMsg.Origin)
	} else {
		device.AccumulateStats(device.Conn())
	}

	deviceVersion := string(initMsg.Version)
	device.SetOrigin(origin)
	device.SetVersion(deviceVersion)
	device.SetPublicIP(initMsg.PublicIP)

	deviceConn := mitm.NewDeviceConn(
		deviceID,
		origin,
		deviceVersion,
		initMsg.PublicIP,
		wsConn,
		statsCollector,
		device.LastMemoryPointer(),
	)

	oldDeviceConn := device.SwapConn(deviceConn)
	if oldDeviceConn == nil {
		// There was no prior connection which means selection was disabled
		// for this device and we must enable it.
		if device.IsSelectionEnabled() {
			mgr.workerSelector.EnableDevice(deviceID)
		}
	} else {
		logger.LogAttrs(ctx, slog.LevelWarn, "closing old connection for device")
		mgr.runInBackground(func() { _ = oldDeviceConn.Close(ws.StatusGoingAway, "A device with same id has connected") })
	}

	deviceConn.SetCloseHandler(func() {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()

		// unset ourselves if we're still set
		if device.CompareAndSwapConn(deviceConn, nil) {
			// No other device connection has replaced us, so we should
			// shut down selection for the workers for this device, unless
			// it was already explicitly requested by user.
			device.AccumulateStats(deviceConn)
			if device.IsSelectionEnabled() {
				mgr.workerSelector.DisableDevice(device.ID())
			}
		}
		statsCollector.DecrDevicesConnected(deviceConn.Origin())
		logger.LogAttrs(ctx, slog.LevelInfo, "deregistered mitm device control")
	})

	return deviceConn, nil
}

// RegisterWorker registers a new MITM worker connection.
func (mgr *ConnectionManager[C, W]) RegisterWorker(ctx context.Context, worker W) error {
	statsCollector := mgr.statsCollector

	logger := logging.LoggerFromContextOrDefault(ctx, mgr.logger)

	deviceID := worker.DeviceID()
	workerID := worker.ID()
	workerOrigin := worker.Origin()

	logger = logger.With(slog.String("worker_id", workerID))
	logger.LogAttrs(ctx, slog.LevelInfo, "done reading worker welcome message")

	if workerID == "" {
		return errors.New("worker_id cannot be empty")
	}
	if deviceID == "" {
		return errors.New("device_id cannot be empty")
	}
	if workerOrigin == "" {
		return errors.New("origin cannot be empty")
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		// worker connected before control connection
		device = mgr.addNewDevice(deviceID, workerOrigin)
		// don't allow workers for this device to be used until the device control
		// connection is established.
		mgr.workerSelector.DisableDevice(deviceID)
	}

	if workerOrigin != device.Origin() {
		logger.LogAttrs(ctx, slog.LevelWarn, "worker origin differs from device origin", slog.String("device_origin", device.Origin()))
	}

	deviceWorkers := mgr.workersByDeviceID[deviceID]
	if deviceWorkers == nil {
		deviceWorkers = make(map[string]W)
		mgr.workersByDeviceID[deviceID] = deviceWorkers
	}

	existingWorker := deviceWorkers[workerID]
	deviceWorkers[workerID] = worker

	if !existingWorker.IsZero() {
		// Carry the replaced worker's cumulative total (previous sessions + its
		// current session) forward, not just its current session.
		_, wsStats := existingWorker.WebsocketStats()
		worker.SetPreviousWSConnStats(wsStats)
		if existingWorker.Origin() != workerOrigin {
			logger.LogAttrs(ctx, slog.LevelInfo, "deregistered worker",
				slog.String(fieldDeviceID, deviceID),
				slog.String(fieldOrigin, existingWorker.Origin()),
			)
		} else {
			logger.LogAttrs(ctx, slog.LevelWarn, "new worker connection replacing old one")
		}
		mgr.runInBackground(func() { //nolint:contextcheck // background goroutine, no parent context
			logger.LogAttrs(context.Background(), slog.LevelWarn, "closing old connection for worker")
			_ = existingWorker.Close(ws.StatusGoingAway, "A worker with same id has connected")
		})
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "registered worker",
		slog.String(fieldDeviceID, deviceID),
		slog.String(fieldOrigin, workerOrigin),
	)

	workerSelector := mgr.workerSelector
	workerSelector.SetWorkerAvailable(worker)
	statsCollector.IncrWorkerRegistrations(workerOrigin)

	worker.SetCloseHandler(func() { //nolint:contextcheck // close handler callback, no parent context
		mgr.mu.Lock()
		defer mgr.mu.Unlock()

		workerSelector.SetWorkerUnavailable(worker)
		if deviceWorkers := mgr.workersByDeviceID[deviceID]; deviceWorkers != nil && deviceWorkers[workerID] == worker {
			delete(deviceWorkers, workerID)
			logger.LogAttrs(context.Background(), slog.LevelInfo, "deregistered worker",
				slog.String(fieldDeviceID, deviceID),
				slog.String(fieldOrigin, workerOrigin),
			)
		}
		if controller := worker.GetModeInfo().Controller; controller != nil && !controller.IsZero() {
			mgr.runInBackground(func() {
				logger.LogAttrs(context.Background(), slog.LevelInfo, "closing controller connection when deregistering worker", slog.String("controller_id", controller.ID()))
				_ = controller.Close(errorutil.CloseCodeMITMWorkerDisconnected, "mitm worker went away")
			})
		}
	})

	return nil
}

// GetDevices returns a snapshot of all registered devices with their workers.
func (mgr *ConnectionManager[C, W]) GetDevices() []*Device[W] {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return mgr.getDevices()
}

// GetDeviceByID returns a device by its ID, or nil if not found.
func (mgr *ConnectionManager[C, W]) GetDeviceByID(id string) *Device[W] {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	device, ok := mgr.allDevicesByID[id]
	if !ok {
		return nil
	}
	return mgr.makeDevice(device)
}

// GetControllers returns a snapshot of all registered controllers.
func (mgr *ConnectionManager[C, W]) GetControllers() []C {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return mgr.getControllers()
}

// GetControllerByUUID returns a controller by its UUID.
func (mgr *ConnectionManager[C, W]) GetControllerByUUID(uuid string) C {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return mgr.controllers[uuid]
}

// GetStatus returns the current status of all registered devices.
func (mgr *ConnectionManager[C, W]) GetStatus() *Status[C, W] {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	return &Status[C, W]{
		Devices:     mgr.getDevices(),
		Controllers: mgr.getControllers(),
	}
}

func (mgr *ConnectionManager[C, W]) hasWorkers(deviceID string) bool {
	return len(mgr.workersByDeviceID[deviceID]) > 0
}

// DeleteUnconnectedDevices removes devices from memory that are not
// currently connected and have no workers connected.
func (mgr *ConnectionManager[C, W]) DeleteUnconnectedDevices() int {
	deviceOriginsToDecr := make(map[string]int)

	statsCollector := mgr.statsCollector
	devicesRemoved := 0

	defer func() {
		for origin, count := range deviceOriginsToDecr {
			statsCollector.DecrDevicesTotal(origin, count)
		}
	}()

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	for _, device := range mgr.allDevicesByID {
		if mgr.hasWorkers(device.ID()) {
			continue
		}
		if device.IsConnected() {
			continue
		}
		mgr.workerSelector.RemoveDeadDevice(device.ID())
		deviceOriginsToDecr[device.Origin()]++
		delete(mgr.allDevicesByID, device.ID())
		delete(mgr.workersByDeviceID, device.ID())
		devicesRemoved++
	}
	return devicesRemoved
}

// DeleteUnconnectedDeviceID removes a specific unconnected device by ID.
func (mgr *ConnectionManager[C, W]) DeleteUnconnectedDeviceID(deviceID string) (err error) {
	var origin string

	defer func() {
		if err == nil {
			mgr.statsCollector.DecrDevicesTotal(origin, 1)
		}
	}()

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		err = errorutil.NewErrDeviceNotKnown()
		return
	}
	if mgr.hasWorkers(deviceID) {
		err = errorutil.NewErrDeviceHasWorkersConnected()
		return
	}
	if device.IsConnected() {
		err = errorutil.NewErrDeviceIsConnected()
		return
	}
	mgr.workerSelector.RemoveDeadDevice(device.ID())
	origin = device.Origin()
	delete(mgr.allDevicesByID, device.ID())
	delete(mgr.workersByDeviceID, deviceID)
	return nil
}

// RestartDeviceApp restarts the application on the device.
func (mgr *ConnectionManager[C, W]) RestartDeviceApp(ctx context.Context, deviceID string) error {
	deviceConn, err := mgr.getDeviceConnForCommand(deviceID)
	if err != nil {
		return err
	}
	return deviceConn.RestartApp(ctx)
}

// RebootDevice reboots the device.
func (mgr *ConnectionManager[C, W]) RebootDevice(ctx context.Context, deviceID string) error {
	deviceConn, err := mgr.getDeviceConnForCommand(deviceID)
	if err != nil {
		return err
	}
	return deviceConn.Reboot(ctx)
}

// GetDeviceLogcat retrieves device logcat data as a zip file.
func (mgr *ConnectionManager[C, W]) GetDeviceLogcat(ctx context.Context, deviceID string) ([]byte, error) {
	deviceConn, err := mgr.getDeviceConnForCommand(deviceID)
	if err != nil {
		return nil, err
	}
	return deviceConn.GetLogcat(ctx)
}

// RunJob executes a job command on the device.
func (mgr *ConnectionManager[C, W]) RunJob(ctx context.Context, jobID string, deviceID string, timeout time.Duration) jobs.JobInstance {
	if mgr.jobsRunner == nil {
		now := time.Now()
		return jobs.JobInstance{
			JobID:      jobID,
			StartedAt:  now,
			FinishedAt: now,
			DeviceID:   deviceID,
			Result:     "jobs are not available",
			Status:     jobs.JobInstanceStatusFailed,
		}
	}
	deviceConn, err := mgr.getDeviceConnForCommand(deviceID)
	if err != nil {
		return mgr.jobsRunner.AddFailedJobInstance(
			jobID,
			deviceID,
			err.Error(),
		)
	}
	return mgr.jobsRunner.RunJob(ctx, jobID, deviceConn, timeout)
}

// Wait waits for any spawned goroutines to exit.
func (mgr *ConnectionManager[C, W]) Wait() {
	mgr.wg.Wait()
}

// EnableDevice enables device selection for the specified device.
func (mgr *ConnectionManager[C, W]) EnableDevice(deviceID string) (*Device[W], error) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		return nil, errorutil.NewErrDeviceNotKnown()
	}

	if device.SetSelectionEnabled(true) {
		mgr.workerSelector.EnableDevice(deviceID)
	}

	return mgr.makeDevice(device), nil
}

// DisableDevice disables device selection for the specified device.
func (mgr *ConnectionManager[C, W]) DisableDevice(deviceID string) (*Device[W], error) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		return nil, errorutil.NewErrDeviceNotKnown()
	}

	if device.SetSelectionEnabled(false) {
		mgr.workerSelector.DisableDevice(deviceID)
	}

	return mgr.makeDevice(device), nil
}

// ReconnectController tells a controller to reconnect/restart-session.
func (mgr *ConnectionManager[C, W]) ReconnectController(uuid string) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	controller, ok := mgr.controllers[uuid]
	if !ok {
		return errorutil.NewErrControllerNotFound()
	}

	mgr.runInBackground(func() {
		_ = controller.Close(errorutil.CloseCodeRestartSession, "api reconnect (restart session) request")
	})
	return nil
}

// DisconnectController disconnects a controller by UUID.
func (mgr *ConnectionManager[C, W]) DisconnectController(uuid string) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	controller, ok := mgr.controllers[uuid]
	if !ok {
		return errorutil.NewErrControllerNotFound()
	}

	mgr.runInBackground(func() {
		_ = controller.Close(errorutil.CloseCodeKillSession, "api disconnect (kill session) request")
	})
	return nil
}

// DisconnectDevice disconnects a device by ID (both control connection and the workers).
func (mgr *ConnectionManager[C, W]) DisconnectDevice(deviceID string) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	device, ok := mgr.allDevicesByID[deviceID]
	if !ok {
		return errorutil.NewErrDeviceNotKnown()
	}

	deviceConn := device.Conn()
	if deviceConn == nil {
		return errorutil.NewErrDeviceNotConnected()
	}

	workerMap := mgr.workersByDeviceID[device.ID()]
	workers := make([]W, 0, len(workerMap))
	for _, worker := range workerMap {
		workers = append(workers, worker)
	}
	mgr.runInBackground(func() {
		for _, worker := range workers {
			_ = worker.Close(ws.StatusGoingAway, "api disconnect request")
		}
		_ = deviceConn.Close(ws.StatusGoingAway, "api disconnect request")
	})
	return nil
}

// RunPeriodicTasks runs background maintenance tasks until the context is cancelled.
func (mgr *ConnectionManager[C, W]) RunPeriodicTasks(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mgr.pruneSelectorWorkerIDs()
		}
	}
}

// NewConnectionManager creates a new ConnectionManager instance.
func NewConnectionManager[C Controller, W MITMWorkerConstraint](cfg ConnectionManagerConfig[C, W]) *ConnectionManager[C, W] {
	if cfg.StatsCollector == nil {
		cfg.StatsCollector = NewNoOpStatsCollector()
	}
	return &ConnectionManager[C, W]{
		logger:            cfg.Logger,
		getSettings:       cfg.GetSettings,
		workerSelector:    cfg.WorkerSelector,
		jobsRunner:        cfg.JobsRunner,
		statsCollector:    cfg.StatsCollector,
		newController:     cfg.NewController,
		userAgent:         cfg.UserAgent,
		allDevicesByID:    make(map[string]*MITMDevice),
		workersByDeviceID: make(map[string]map[string]W),
		controllers:       make(map[string]C),
	}
}
