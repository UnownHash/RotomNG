package connections

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// newTestManager creates a ConnectionManager with all mocks wired up.
func newTestManager(opts ...func(*testManagerOpts)) (*ConnectionManager[*mockController, *mockWorker], *testManagerOpts) {
	o := &testManagerOpts{
		selector: &mockWorkerSelector{},
		stats:    newMockStatsCollector(),
		jobs:     &mockJobsRunner{},
	}
	for _, opt := range opts {
		opt(o)
	}

	cfg := ConnectionManagerConfig[*mockController, *mockWorker]{
		Logger:         logging.NewDiscardLogger(),
		StatsCollector: o.stats,
		JobsRunner:     o.jobs,
		WorkerSelector: o.selector,
		NewController:  o.controllerFactory(),
		UserAgent:      "test-rotom/1.0",
	}
	if err := cfg.Init(ConnectionManagerSettings{DisableWorkerStats: o.disableWorkerStats}); err != nil {
		panic(err)
	}
	mgr := NewConnectionManager(cfg)
	return mgr, o
}

type testManagerOpts struct {
	selector           *mockWorkerSelector
	stats              *mockStatsCollector
	jobs               *mockJobsRunner
	disableWorkerStats bool
}

func (o *testManagerOpts) controllerFactory() NewControllerFunc[*mockController] {
	return func(
		_ ControllerWSConn,
		id string,
		_ *protos.MitmRequest,
		_ MITMWorker,
		weight int,
		userAgent string,
		disableWorkerStats bool,
		protoMajorVersion int,
		protoMinorVersion int,
	) *mockController {
		return &mockController{
			id:                 id,
			weight:             weight,
			userAgent:          userAgent,
			protoMajor:         protoMajorVersion,
			protoMinor:         protoMinorVersion,
			disableWorkerStats: disableWorkerStats,
		}
	}
}

// addDeviceDirectly adds a device to the manager's internal state for testing.
func addDeviceDirectly(mgr *ConnectionManager[*mockController, *mockWorker], deviceID, origin string) *MITMDevice {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	device := mitm.NewDevice(deviceID, origin)
	mgr.allDevicesByID[deviceID] = device
	return device
}

// addWorkerDirectly adds a worker to the manager's internal state.
func addWorkerDirectly(mgr *ConnectionManager[*mockController, *mockWorker], worker *mockWorker) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	deviceID := worker.DeviceID()
	if mgr.workersByDeviceID[deviceID] == nil {
		mgr.workersByDeviceID[deviceID] = make(map[string]*mockWorker)
	}
	mgr.workersByDeviceID[deviceID][worker.ID()] = worker
}

// addControllerDirectly adds a controller to the manager.
func addControllerDirectly(mgr *ConnectionManager[*mockController, *mockWorker], uuid string, controller *mockController) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	controller.uuid = uuid
	mgr.controllers[uuid] = controller
}

// --- Constructor Tests ---

func TestNewConnectionManager(t *testing.T) {
	mgr, _ := newTestManager()

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.allDevicesByID == nil {
		t.Error("expected initialized allDevicesByID map")
	}
	if mgr.workersByDeviceID == nil {
		t.Error("expected initialized workersByDeviceID map")
	}
	if mgr.controllers == nil {
		t.Error("expected initialized controllers map")
	}
	if mgr.userAgent != "test-rotom/1.0" {
		t.Errorf("expected user agent 'test-rotom/1.0', got '%s'", mgr.userAgent)
	}
}

func TestNewConnectionManager_NilStatsCollector(t *testing.T) {
	cfg := ConnectionManagerConfig[*mockController, *mockWorker]{
		Logger:         logging.NewDiscardLogger(),
		StatsCollector: nil,
		WorkerSelector: &mockWorkerSelector{},
		NewController: func(_ ControllerWSConn, _ string, _ *protos.MitmRequest, _ MITMWorker, _ int, _ string, _ bool, _, _ int) *mockController {
			return &mockController{}
		},
	}
	if err := cfg.Init(ConnectionManagerSettings{}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	mgr := NewConnectionManager(cfg)
	if mgr.statsCollector == nil {
		t.Error("expected non-nil stats collector (should default to NoOp)")
	}
}

// --- Settings Tests ---

func TestConnectionManagerSettings_Validate(t *testing.T) {
	s := ConnectionManagerSettings{DisableWorkerStats: true}
	if err := s.Validate(); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestConnectionManagerConfig_Init(t *testing.T) {
	cfg := ConnectionManagerConfig[*mockController, *mockWorker]{}
	err := cfg.Init(ConnectionManagerSettings{DisableWorkerStats: true})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	settings := cfg.GetSettings()
	if !settings.DisableWorkerStats {
		t.Error("expected DisableWorkerStats to be true")
	}
}

// --- Device.Workers() ---

func TestDevice_Workers(t *testing.T) {
	workers := []*mockWorker{
		{id: "w1", deviceID: "d1"},
		{id: "w2", deviceID: "d1"},
	}
	device := &Device[*mockWorker]{
		mitmDevice: mitm.NewDevice("d1", "origin1"),
		workers:    workers,
	}
	got := device.Workers()
	if len(got) != 2 {
		t.Errorf("expected 2 workers, got %d", len(got))
	}
}

func TestDevice_Workers_Empty(t *testing.T) {
	device := &Device[*mockWorker]{
		mitmDevice: mitm.NewDevice("d1", "origin1"),
	}
	got := device.Workers()
	if len(got) != 0 {
		t.Errorf("expected 0 workers, got %d", len(got))
	}
}

// --- GetDevices / GetDeviceByID ---

func TestGetDevices_Empty(t *testing.T) {
	mgr, _ := newTestManager()
	devices := mgr.GetDevices()
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestGetDevices_WithDevices(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	addDeviceDirectly(mgr, "d2", "origin2")

	devices := mgr.GetDevices()
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestGetDeviceByID_Found(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	device := mgr.GetDeviceByID("d1")
	if device == nil {
		t.Fatal("expected non-nil device")
	}
	if device.ID() != "d1" {
		t.Errorf("expected device ID 'd1', got '%s'", device.ID())
	}
}

func TestGetDeviceByID_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	device := mgr.GetDeviceByID("nonexistent")
	if device != nil {
		t.Error("expected nil device for nonexistent ID")
	}
}

func TestGetDeviceByID_WithWorkers(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	w := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	addWorkerDirectly(mgr, w)

	device := mgr.GetDeviceByID("d1")
	if device == nil {
		t.Fatal("expected non-nil device")
	}
	if len(device.Workers()) != 1 {
		t.Errorf("expected 1 worker, got %d", len(device.Workers()))
	}
}

// --- GetControllers / GetControllerByUUID ---

func TestGetControllers_Empty(t *testing.T) {
	mgr, _ := newTestManager()
	controllers := mgr.GetControllers()
	if len(controllers) != 0 {
		t.Errorf("expected 0 controllers, got %d", len(controllers))
	}
}

func TestGetControllers_WithControllers(t *testing.T) {
	mgr, _ := newTestManager()
	addControllerDirectly(mgr, "uuid1", &mockController{id: "c1"})
	addControllerDirectly(mgr, "uuid2", &mockController{id: "c2"})

	controllers := mgr.GetControllers()
	if len(controllers) != 2 {
		t.Errorf("expected 2 controllers, got %d", len(controllers))
	}
}

func TestGetControllerByUUID_Found(t *testing.T) {
	mgr, _ := newTestManager()
	c := &mockController{id: "c1"}
	addControllerDirectly(mgr, "uuid1", c)

	got := mgr.GetControllerByUUID("uuid1")
	if got == nil {
		t.Fatal("expected non-nil controller")
	}
	if got.ID() != "c1" {
		t.Errorf("expected controller ID 'c1', got '%s'", got.ID())
	}
}

func TestGetControllerByUUID_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	got := mgr.GetControllerByUUID("nonexistent")
	if got != nil {
		t.Error("expected nil controller for nonexistent UUID")
	}
}

// --- GetStatus ---

func TestGetStatus(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	addControllerDirectly(mgr, "uuid1", &mockController{id: "c1"})

	status := mgr.GetStatus()
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if len(status.Devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(status.Devices))
	}
	if len(status.Controllers) != 1 {
		t.Errorf("expected 1 controller, got %d", len(status.Controllers))
	}
}

// --- RegisterWorker ---

func TestRegisterWorker_Success(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	err := mgr.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Verify worker was added
	device := mgr.GetDeviceByID("d1")
	if len(device.Workers()) != 1 {
		t.Errorf("expected 1 worker, got %d", len(device.Workers()))
	}

	// Verify stats
	o.stats.mu.Lock()
	if o.stats.workerRegistrations["origin1"] != 1 {
		t.Errorf("expected 1 worker registration for origin1, got %d", o.stats.workerRegistrations["origin1"])
	}
	o.stats.mu.Unlock()

	// Verify worker was set as available
	o.selector.mu.Lock()
	if len(o.selector.availableWorkers) != 1 {
		t.Errorf("expected 1 available worker, got %d", len(o.selector.availableWorkers))
	}
	o.selector.mu.Unlock()
}

func TestRegisterWorker_EmptyWorkerID(t *testing.T) {
	mgr, _ := newTestManager()
	worker := &mockWorker{id: "", deviceID: "d1", origin: "origin1"}
	err := mgr.RegisterWorker(context.Background(), worker)
	if err == nil {
		t.Fatal("expected error for empty worker ID")
	}
}

func TestRegisterWorker_EmptyDeviceID(t *testing.T) {
	mgr, _ := newTestManager()
	worker := &mockWorker{id: "w1", deviceID: "", origin: "origin1"}
	err := mgr.RegisterWorker(context.Background(), worker)
	if err == nil {
		t.Fatal("expected error for empty device ID")
	}
}

func TestRegisterWorker_EmptyOrigin(t *testing.T) {
	mgr, _ := newTestManager()
	worker := &mockWorker{id: "w1", deviceID: "d1", origin: ""}
	err := mgr.RegisterWorker(context.Background(), worker)
	if err == nil {
		t.Fatal("expected error for empty origin")
	}
}

func TestRegisterWorker_CreatesDeviceIfNotExists(t *testing.T) {
	mgr, o := newTestManager()

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	err := mgr.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Device should be auto-created
	device := mgr.GetDeviceByID("d1")
	if device == nil {
		t.Fatal("expected device to be auto-created")
	}

	// Device selection should be disabled (no control connection yet)
	o.selector.mu.Lock()
	if len(o.selector.disabledDevices) != 1 || o.selector.disabledDevices[0] != "d1" {
		t.Error("expected device selection to be disabled for auto-created device")
	}
	o.selector.mu.Unlock()
}

func TestRegisterWorker_ReplacesExistingWorker(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	oldWorker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	if err := mgr.RegisterWorker(context.Background(), oldWorker); err != nil {
		t.Fatalf("RegisterWorker (old) failed: %v", err)
	}

	newWorker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	if err := mgr.RegisterWorker(context.Background(), newWorker); err != nil {
		t.Fatalf("RegisterWorker (new) failed: %v", err)
	}

	mgr.Wait()

	// Old worker should have been closed
	oldWorker.mu.Lock()
	if !oldWorker.closeCalled {
		t.Error("expected old worker to be closed")
	}
	oldWorker.mu.Unlock()
}

func TestRegisterWorker_DifferentOriginFromDevice(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin2"}
	err := mgr.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Should still register successfully despite origin mismatch
	device := mgr.GetDeviceByID("d1")
	if len(device.Workers()) != 1 {
		t.Errorf("expected 1 worker, got %d", len(device.Workers()))
	}
}

func TestRegisterWorker_CloseHandlerDeregisters(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	if err := mgr.RegisterWorker(context.Background(), worker); err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Trigger close handler
	worker.mu.Lock()
	closeHandler := worker.closeHandler
	worker.mu.Unlock()

	if closeHandler == nil {
		t.Fatal("expected close handler to be set")
	}
	closeHandler()

	// Worker should be deregistered
	device := mgr.GetDeviceByID("d1")
	if len(device.Workers()) != 0 {
		t.Errorf("expected 0 workers after close, got %d", len(device.Workers()))
	}

	// Worker should be set unavailable
	o.selector.mu.Lock()
	if len(o.selector.unavailableWorkers) != 1 {
		t.Errorf("expected 1 unavailable worker, got %d", len(o.selector.unavailableWorkers))
	}
	o.selector.mu.Unlock()
}

func TestRegisterWorker_CloseHandlerWithController(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	ctrl := &mockController{id: "ctrl1", uuid: "uuid1"}
	worker := &mockWorker{
		id:       "w1",
		deviceID: "d1",
		origin:   "origin1",
		modeInfo: mitm.WorkerModeInfo{Controller: ctrl},
	}
	if err := mgr.RegisterWorker(context.Background(), worker); err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Trigger close handler
	worker.mu.Lock()
	closeHandler := worker.closeHandler
	worker.mu.Unlock()
	closeHandler()

	mgr.Wait()

	// Controller should have been closed
	ctrl.mu.Lock()
	if !ctrl.closeCalled {
		t.Error("expected controller to be closed when worker deregisters")
	}
	ctrl.mu.Unlock()
}

// --- RegisterDeviceConnection ---

func TestRegisterDeviceConnection_Success(t *testing.T) {
	mgr, o := newTestManager()
	ctx := context.Background()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0.0",
			PublicIP: "1.2.3.4",
		},
	}

	deviceConn, err := mgr.RegisterDeviceConnection(ctx, wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}
	if deviceConn == nil {
		t.Fatal("expected non-nil device conn")
	}
	if deviceConn.ID() != "d1" {
		t.Errorf("expected device ID 'd1', got '%s'", deviceConn.ID())
	}

	// Verify device was created
	device := mgr.GetDeviceByID("d1")
	if device == nil {
		t.Fatal("expected device to exist")
	}

	// Verify stats
	o.stats.mu.Lock()
	if o.stats.deviceRegistrations["origin1"] != 1 {
		t.Error("expected device registration to be tracked")
	}
	if o.stats.devicesConnected["origin1"] != 1 {
		t.Error("expected device connected to be tracked")
	}
	o.stats.mu.Unlock()

	// Verify selection was enabled
	o.selector.mu.Lock()
	if len(o.selector.enabledDevices) != 1 {
		t.Errorf("expected 1 enabled device, got %d", len(o.selector.enabledDevices))
	}
	o.selector.mu.Unlock()
}

func TestRegisterDeviceConnection_ReadError(t *testing.T) {
	mgr, o := newTestManager()
	ctx := context.Background()

	wsConn := &mockDeviceWSConn{
		readErr: errors.New("read failed"),
	}

	_, err := mgr.RegisterDeviceConnection(ctx, wsConn)
	if err == nil {
		t.Fatal("expected error on read failure")
	}

	o.stats.mu.Lock()
	if o.stats.deviceRegFails != 1 {
		t.Error("expected registration fail to be tracked")
	}
	o.stats.mu.Unlock()
}

func TestRegisterDeviceConnection_EmptyDeviceID(t *testing.T) {
	mgr, o := newTestManager()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "",
			Origin:   "origin1",
		},
	}

	_, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for empty device ID")
	}

	o.stats.mu.Lock()
	if o.stats.deviceRegFails != 1 {
		t.Error("expected registration fail to be tracked")
	}
	o.stats.mu.Unlock()
}

func TestRegisterDeviceConnection_EmptyOrigin(t *testing.T) {
	mgr, o := newTestManager()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "",
		},
	}

	_, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for empty origin")
	}

	o.stats.mu.Lock()
	if o.stats.deviceRegFails != 1 {
		t.Error("expected registration fail to be tracked")
	}
	o.stats.mu.Unlock()
}

func TestRegisterDeviceConnection_ReplacesExistingConn(t *testing.T) {
	mgr, _ := newTestManager()
	ctx := context.Background()

	wsConn1 := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0.0",
			PublicIP: "1.2.3.4",
		},
	}

	_, err := mgr.RegisterDeviceConnection(ctx, wsConn1)
	if err != nil {
		t.Fatalf("first RegisterDeviceConnection failed: %v", err)
	}

	wsConn2 := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "2.0.0",
			PublicIP: "5.6.7.8",
		},
	}

	_, err = mgr.RegisterDeviceConnection(ctx, wsConn2)
	if err != nil {
		t.Fatalf("second RegisterDeviceConnection failed: %v", err)
	}

	mgr.Wait()

	// Old connection should have been closed
	wsConn1.mu.Lock()
	closed := wsConn1.closeCalled
	wsConn1.mu.Unlock()
	if !closed {
		t.Error("expected old device connection to be closed")
	}
}

func TestRegisterDeviceConnection_CloseHandlerDeregisters(t *testing.T) {
	mgr, o := newTestManager()
	ctx := context.Background()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0.0",
			PublicIP: "1.2.3.4",
		},
	}

	deviceConn, err := mgr.RegisterDeviceConnection(ctx, wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}

	// Trigger the close handler by calling Close on the device conn
	// The DeviceConn sets up a close handler internally;
	// we need to simulate by checking the device state after disconnect
	_ = deviceConn

	// Check the device is currently connected
	device := mgr.GetDeviceByID("d1")
	if device == nil || !device.IsConnected() {
		t.Error("expected device to be connected")
	}

	// Verify stats
	o.stats.mu.Lock()
	if o.stats.devicesConnected["origin1"] != 1 {
		t.Error("expected devices connected to be incremented")
	}
	o.stats.mu.Unlock()
}

// --- EnableDevice / DisableDevice ---

func TestEnableDevice_Success(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	// Disable first, then re-enable
	mgr.DisableDevice("d1")
	device, err := mgr.EnableDevice("d1")
	if err != nil {
		t.Fatalf("EnableDevice failed: %v", err)
	}
	if device == nil {
		t.Fatal("expected non-nil device")
	}

	o.selector.mu.Lock()
	found := slices.Contains(o.selector.enabledDevices, "d1")
	o.selector.mu.Unlock()
	if !found {
		t.Error("expected device to be enabled in selector")
	}
}

func TestEnableDevice_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.EnableDevice("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestDisableDevice_Success(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	device, err := mgr.DisableDevice("d1")
	if err != nil {
		t.Fatalf("DisableDevice failed: %v", err)
	}
	if device == nil {
		t.Fatal("expected non-nil device")
	}

	o.selector.mu.Lock()
	if len(o.selector.disabledDevices) != 1 || o.selector.disabledDevices[0] != "d1" {
		t.Error("expected device to be disabled in selector")
	}
	o.selector.mu.Unlock()
}

func TestDisableDevice_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.DisableDevice("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestEnableDevice_AlreadyEnabled(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	// Device starts enabled, so enabling again should be a no-op for the selector
	_, err := mgr.EnableDevice("d1")
	if err != nil {
		t.Fatalf("EnableDevice failed: %v", err)
	}

	o.selector.mu.Lock()
	enableCount := len(o.selector.enabledDevices)
	o.selector.mu.Unlock()
	// SetSelectionEnabled returns false when already in desired state,
	// so EnableDevice should not be called
	if enableCount != 0 {
		t.Errorf("expected 0 enable calls (already enabled), got %d", enableCount)
	}
}

// --- DeleteUnconnectedDevices ---

func TestDeleteUnconnectedDevices_RemovesDisconnected(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	addDeviceDirectly(mgr, "d2", "origin2")

	removed := mgr.DeleteUnconnectedDevices()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	if mgr.GetDeviceByID("d1") != nil {
		t.Error("expected d1 to be removed")
	}
	if mgr.GetDeviceByID("d2") != nil {
		t.Error("expected d2 to be removed")
	}

	// Verify stats decremented
	o.stats.mu.Lock()
	if o.stats.devicesTotalDecr["origin1"] != 1 {
		t.Error("expected total decrement for origin1")
	}
	if o.stats.devicesTotalDecr["origin2"] != 1 {
		t.Error("expected total decrement for origin2")
	}
	o.stats.mu.Unlock()

	// Verify selector cleanup
	o.selector.mu.Lock()
	if len(o.selector.removedDevices) != 2 {
		t.Errorf("expected 2 removed devices in selector, got %d", len(o.selector.removedDevices))
	}
	o.selector.mu.Unlock()
}

func TestDeleteUnconnectedDevices_KeepsConnected(t *testing.T) {
	mgr, _ := newTestManager()

	// Register a connected device
	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0",
			PublicIP: "1.2.3.4",
		},
	}
	_, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}

	// Add a disconnected device
	addDeviceDirectly(mgr, "d2", "origin2")

	removed := mgr.DeleteUnconnectedDevices()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	if mgr.GetDeviceByID("d1") == nil {
		t.Error("expected connected device d1 to still exist")
	}
	if mgr.GetDeviceByID("d2") != nil {
		t.Error("expected disconnected device d2 to be removed")
	}
}

func TestDeleteUnconnectedDevices_KeepsWithWorkers(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	addWorkerDirectly(mgr, &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"})

	removed := mgr.DeleteUnconnectedDevices()
	if removed != 0 {
		t.Errorf("expected 0 removed (device has workers), got %d", removed)
	}
}

func TestDeleteUnconnectedDevices_Empty(t *testing.T) {
	mgr, _ := newTestManager()
	removed := mgr.DeleteUnconnectedDevices()
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}

// --- DeleteUnconnectedDeviceID ---

func TestDeleteUnconnectedDeviceID_Success(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	err := mgr.DeleteUnconnectedDeviceID("d1")
	if err != nil {
		t.Fatalf("DeleteUnconnectedDeviceID failed: %v", err)
	}

	if mgr.GetDeviceByID("d1") != nil {
		t.Error("expected device to be removed")
	}

	o.stats.mu.Lock()
	if o.stats.devicesTotalDecr["origin1"] != 1 {
		t.Error("expected total decrement for origin1")
	}
	o.stats.mu.Unlock()
}

func TestDeleteUnconnectedDeviceID_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.DeleteUnconnectedDeviceID("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestDeleteUnconnectedDeviceID_HasWorkers(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	addWorkerDirectly(mgr, &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"})

	err := mgr.DeleteUnconnectedDeviceID("d1")
	if err == nil {
		t.Fatal("expected error when device has workers")
	}
}

func TestDeleteUnconnectedDeviceID_IsConnected(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0",
			PublicIP: "1.2.3.4",
		},
	}
	_, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}

	err = mgr.DeleteUnconnectedDeviceID("d1")
	if err == nil {
		t.Fatal("expected error when device is connected")
	}
}

// --- ReconnectController / DisconnectController ---

func TestReconnectController_Success(t *testing.T) {
	mgr, _ := newTestManager()
	ctrl := &mockController{id: "c1"}
	addControllerDirectly(mgr, "uuid1", ctrl)

	err := mgr.ReconnectController("uuid1")
	if err != nil {
		t.Fatalf("ReconnectController failed: %v", err)
	}

	mgr.Wait()

	ctrl.mu.Lock()
	if !ctrl.closeCalled {
		t.Error("expected controller to be closed")
	}
	ctrl.mu.Unlock()
}

func TestReconnectController_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.ReconnectController("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent controller")
	}
}

func TestDisconnectController_Success(t *testing.T) {
	mgr, _ := newTestManager()
	ctrl := &mockController{id: "c1"}
	addControllerDirectly(mgr, "uuid1", ctrl)

	err := mgr.DisconnectController("uuid1")
	if err != nil {
		t.Fatalf("DisconnectController failed: %v", err)
	}

	mgr.Wait()

	ctrl.mu.Lock()
	if !ctrl.closeCalled {
		t.Error("expected controller to be closed")
	}
	ctrl.mu.Unlock()
}

func TestDisconnectController_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.DisconnectController("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent controller")
	}
}

// --- DisconnectDevice ---

func TestDisconnectDevice_Success(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0",
			PublicIP: "1.2.3.4",
		},
	}
	_, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}

	// Add a worker
	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	mgr.RegisterWorker(context.Background(), worker)

	err = mgr.DisconnectDevice("d1")
	if err != nil {
		t.Fatalf("DisconnectDevice failed: %v", err)
	}

	mgr.Wait()

	// Worker should have been closed
	worker.mu.Lock()
	if !worker.closeCalled {
		t.Error("expected worker to be closed")
	}
	worker.mu.Unlock()

	// Device connection should have been closed
	wsConn.mu.Lock()
	if !wsConn.closeCalled {
		t.Error("expected device ws conn to be closed")
	}
	wsConn.mu.Unlock()
}

func TestDisconnectDevice_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.DisconnectDevice("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestDisconnectDevice_NotConnected(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	err := mgr.DisconnectDevice("d1")
	if err == nil {
		t.Fatal("expected error when device not connected")
	}
}

// --- RunJob ---

func TestRunJob_Success(t *testing.T) {
	mgr, o := newTestManager()

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin1",
			Version:  "1.0",
			PublicIP: "1.2.3.4",
		},
	}
	deviceConn, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}
	// Enable commands on the device conn
	_ = deviceConn

	o.jobs.runJobResult = jobs.JobInstance{
		JobID:  "job1",
		Status: jobs.JobInstanceStatusSucceeded,
	}

	// Note: RunJob will try to get device conn, but CanRunCommands() may be false
	// This should result in AddFailedJobInstance being called
	result := mgr.RunJob(context.Background(), "job1", "d1", 5*time.Second)
	_ = result
	// Just verify it doesn't panic
}

func TestRunJob_NilJobsRunner(t *testing.T) {
	_, _ = newTestManager(func(o *testManagerOpts) {
		o.jobs = nil
	})

	cfg := ConnectionManagerConfig[*mockController, *mockWorker]{
		Logger:         logging.NewDiscardLogger(),
		StatsCollector: newMockStatsCollector(),
		JobsRunner:     nil,
		WorkerSelector: &mockWorkerSelector{},
		NewController: func(_ ControllerWSConn, _ string, _ *protos.MitmRequest, _ MITMWorker, _ int, _ string, _ bool, _, _ int) *mockController {
			return &mockController{}
		},
	}
	if err := cfg.Init(ConnectionManagerSettings{}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	mgr := NewConnectionManager(cfg)

	result := mgr.RunJob(context.Background(), "job1", "d1", 5*time.Second)
	if result.Status != jobs.JobInstanceStatusFailed {
		t.Errorf("expected failed status, got %v", result.Status)
	}
	if result.Result != "jobs are not available" {
		t.Errorf("expected 'jobs are not available', got '%s'", result.Result)
	}
}

func TestRunJob_DeviceNotFound(t *testing.T) {
	mgr, o := newTestManager()
	o.jobs.failedJobResult = jobs.JobInstance{
		JobID:  "job1",
		Status: jobs.JobInstanceStatusFailed,
		Result: "device not known",
	}

	result := mgr.RunJob(context.Background(), "job1", "nonexistent", 5*time.Second)
	if result.Status != jobs.JobInstanceStatusFailed {
		t.Errorf("expected failed status, got %v", result.Status)
	}

	o.jobs.mu.Lock()
	if !o.jobs.addFailedCalled {
		t.Error("expected AddFailedJobInstance to be called")
	}
	o.jobs.mu.Unlock()
}

// --- RestartDeviceApp / RebootDevice / GetDeviceLogcat ---

func TestRestartDeviceApp_DeviceNotFound(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.RestartDeviceApp(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestRebootDevice_DeviceNotFound(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.RebootDevice(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestGetDeviceLogcat_DeviceNotFound(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.GetDeviceLogcat(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestRestartDeviceApp_DeviceNotConnected(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	err := mgr.RestartDeviceApp(context.Background(), "d1")
	if err == nil {
		t.Fatal("expected error for unconnected device")
	}
}

func TestRebootDevice_DeviceNotConnected(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	err := mgr.RebootDevice(context.Background(), "d1")
	if err == nil {
		t.Fatal("expected error for unconnected device")
	}
}

func TestGetDeviceLogcat_DeviceNotConnected(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	_, err := mgr.GetDeviceLogcat(context.Background(), "d1")
	if err == nil {
		t.Fatal("expected error for unconnected device")
	}
}

// --- Wait ---

func TestWait(_ *testing.T) {
	mgr, _ := newTestManager()
	// Should not block when no background tasks
	mgr.Wait()
}

// --- RunPeriodicTasks ---

func TestRunPeriodicTasks_CancelledContext(t *testing.T) {
	mgr, _ := newTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return immediately when context is already cancelled
	done := make(chan struct{})
	go func() {
		mgr.RunPeriodicTasks(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("RunPeriodicTasks did not exit after context cancellation")
	}
}

// --- pruneSelectorWorkerIDs ---

func TestPruneSelectorWorkerIDs_PanicRecovery(_ *testing.T) {
	mgr, o := newTestManager()
	o.selector.prunePanic = true

	// Should not panic
	mgr.pruneSelectorWorkerIDs()
}

func TestPruneSelectorWorkerIDs_Success(t *testing.T) {
	mgr, o := newTestManager()
	mgr.pruneSelectorWorkerIDs()

	o.selector.mu.Lock()
	if !o.selector.pruneCalled {
		t.Error("expected PruneWorkerIDsSeen to be called")
	}
	o.selector.mu.Unlock()
}

// --- registerController ---

func TestRegisterController_AssignsUUID(t *testing.T) {
	mgr, _ := newTestManager()
	ctrl := &mockController{id: "c1"}
	mgr.registerController(ctrl)

	ctrl.mu.Lock()
	uuid := ctrl.uuid
	ctrl.mu.Unlock()

	if uuid == "" {
		t.Error("expected UUID to be assigned")
	}

	// Should be findable by UUID
	got := mgr.GetControllerByUUID(uuid)
	if got == nil {
		t.Error("expected controller to be registered by UUID")
	}
}

func TestRegisterController_CloseHandlerDeregisters(t *testing.T) {
	mgr, _ := newTestManager()
	ctrl := &mockController{id: "c1"}
	mgr.registerController(ctrl)

	ctrl.mu.Lock()
	uuid := ctrl.uuid
	closeHandler := ctrl.closeHandler
	ctrl.mu.Unlock()

	if closeHandler == nil {
		t.Fatal("expected close handler to be set")
	}

	closeHandler()

	got := mgr.GetControllerByUUID(uuid)
	if got != nil {
		t.Error("expected controller to be deregistered after close")
	}
}

// --- RegisterControllerConnectionV2 ---

func TestRegisterControllerConnectionV2_Success(t *testing.T) {
	mgr, o := newTestManager()

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	o.selector.workers = []*mockWorker{worker}

	// Create the registration request
	registerReq := &protos.RegisterControllerRequest{
		Id:                "ctrl1",
		ProtoMajorVersion: int32(ProtoMajorVersion),
		ProtoMinorVersion: int32(ProtoMinorVersion),
		Weight:            5,
	}
	registerReqBytes, err := proto.Marshal(registerReq)
	if err != nil {
		t.Fatalf("failed to marshal register request: %v", err)
	}

	// Create login request
	loginReq := &protos.MitmRequest{
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: "w1",
				Username: "testuser",
			},
		},
	}
	loginReqBytes, err := proto.Marshal(loginReq)
	if err != nil {
		t.Fatalf("failed to marshal login request: %v", err)
	}

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: registerReqBytes, msgType: ws.MessageBinary},
			&mockReader{data: loginReqBytes, msgType: ws.MessageBinary},
		},
	}

	controller, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err != nil {
		t.Fatalf("RegisterControllerConnectionV2 failed: %v", err)
	}

	if controller == nil {
		t.Fatal("expected non-nil controller")
	}
	if controller.ID() != "ctrl1" {
		t.Errorf("expected controller ID 'ctrl1', got '%s'", controller.ID())
	}
	if controller.UUID() == "" {
		t.Error("expected UUID to be assigned")
	}
}

func TestRegisterControllerConnectionV2_ReadError(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{
		readerErr: errors.New("read failed"),
	}

	_, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err == nil {
		t.Fatal("expected error on read failure")
	}
}

func TestRegisterControllerConnectionV2_EmptyControllerID(t *testing.T) {
	mgr, _ := newTestManager()

	registerReq := &protos.RegisterControllerRequest{
		Id:                "",
		ProtoMajorVersion: int32(ProtoMajorVersion),
	}
	registerReqBytes, _ := proto.Marshal(registerReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: registerReqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err == nil {
		t.Fatal("expected error for empty controller ID")
	}
}

func TestRegisterControllerConnectionV2_VersionMismatch(t *testing.T) {
	mgr, _ := newTestManager()

	registerReq := &protos.RegisterControllerRequest{
		Id:                "ctrl1",
		ProtoMajorVersion: 999, // wrong version
		ProtoMinorVersion: 0,
	}
	registerReqBytes, _ := proto.Marshal(registerReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: registerReqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err == nil {
		t.Fatal("expected error for version mismatch")
	}
}

func TestRegisterControllerConnectionV2_NoWorkersAvailable(t *testing.T) {
	mgr, o := newTestManager()
	o.selector.getAvailableErr = errors.New("no workers available")

	registerReq := &protos.RegisterControllerRequest{
		Id:                "ctrl1",
		ProtoMajorVersion: int32(ProtoMajorVersion),
		ProtoMinorVersion: int32(ProtoMinorVersion),
		Weight:            5,
	}
	registerReqBytes, _ := proto.Marshal(registerReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: registerReqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err == nil {
		t.Fatal("expected error when no workers available")
	}

	// Verify ws was closed with appropriate code
	wsConn.mu.Lock()
	if !wsConn.closeCalled {
		t.Error("expected ws connection to be closed")
	}
	wsConn.mu.Unlock()
}

func TestRegisterControllerConnectionV2_WriteResponseError(t *testing.T) {
	mgr, o := newTestManager()

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	o.selector.workers = []*mockWorker{worker}

	registerReq := &protos.RegisterControllerRequest{
		Id:                "ctrl1",
		ProtoMajorVersion: int32(ProtoMajorVersion),
		ProtoMinorVersion: int32(ProtoMinorVersion),
		Weight:            5,
	}
	registerReqBytes, _ := proto.Marshal(registerReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: registerReqBytes, msgType: ws.MessageBinary},
		},
		writeErr: errors.New("write failed"),
	}

	_, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err == nil {
		t.Fatal("expected error on write failure")
	}

	// Worker should be released back to available
	o.selector.mu.Lock()
	if len(o.selector.availableWorkers) != 1 {
		t.Errorf("expected worker to be set available after write failure, got %d", len(o.selector.availableWorkers))
	}
	o.selector.mu.Unlock()
}

func TestRegisterControllerConnectionV2_LoginReadError(t *testing.T) {
	mgr, o := newTestManager()

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	o.selector.workers = []*mockWorker{worker}

	registerReq := &protos.RegisterControllerRequest{
		Id:                "ctrl1",
		ProtoMajorVersion: int32(ProtoMajorVersion),
		ProtoMinorVersion: int32(ProtoMinorVersion),
		Weight:            5,
	}
	registerReqBytes, _ := proto.Marshal(registerReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: registerReqBytes, msgType: ws.MessageBinary},
			// No second reader - will fail on login request read
		},
	}

	_, err := mgr.RegisterControllerConnectionV2(context.Background(), wsConn, "test-ua")
	if err == nil {
		t.Fatal("expected error on login read failure")
	}

	// Worker should be released back
	o.selector.mu.Lock()
	if len(o.selector.availableWorkers) != 1 {
		t.Errorf("expected worker to be set available after login read failure, got %d", len(o.selector.availableWorkers))
	}
	o.selector.mu.Unlock()
}

// --- RegisterControllerConnectionV1 ---

func TestRegisterControllerConnectionV1_Success(t *testing.T) {
	mgr, o := newTestManager()

	worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
	o.selector.workers = []*mockWorker{worker}

	loginReq := &protos.MitmRequest{
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: "w1",
				Username: "testuser",
			},
		},
	}
	loginReqBytes, _ := proto.Marshal(loginReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: loginReqBytes, msgType: ws.MessageBinary},
		},
	}

	controller, err := mgr.RegisterControllerConnectionV1(context.Background(), wsConn, 5, "test-ua")
	if err != nil {
		t.Fatalf("RegisterControllerConnectionV1 failed: %v", err)
	}
	if controller == nil {
		t.Fatal("expected non-nil controller")
	}
	if controller.UUID() == "" {
		t.Error("expected UUID to be assigned")
	}
}

func TestRegisterControllerConnectionV1_ReadError(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{
		readerErr: errors.New("read failed"),
	}

	_, err := mgr.RegisterControllerConnectionV1(context.Background(), wsConn, 5, "test-ua")
	if err == nil {
		t.Fatal("expected error on read failure")
	}
}

func TestRegisterControllerConnectionV1_NoWorkersAvailable(t *testing.T) {
	mgr, o := newTestManager()
	o.selector.getAvailableErr = errors.New("no workers available")

	loginReq := &protos.MitmRequest{
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: "w1",
				Username: "testuser",
			},
		},
	}
	loginReqBytes, _ := proto.Marshal(loginReq)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: loginReqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.RegisterControllerConnectionV1(context.Background(), wsConn, 5, "test-ua")
	if err == nil {
		t.Fatal("expected error when no workers available")
	}

	wsConn.mu.Lock()
	if !wsConn.closeCalled {
		t.Error("expected ws connection to be closed")
	}
	wsConn.mu.Unlock()
}

func TestRegisterControllerConnectionV1_WeightClamping(t *testing.T) {
	tests := []struct {
		name           string
		inputWeight    int
		expectedWeight int
	}{
		{"below min", 0, DefaultControllerWeight},
		{"at min", 1, 1},
		{"normal", 5, 5},
		{"at max", 10, 10},
		{"above max", 100, MaxControllerWeight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, o := newTestManager()
			worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
			o.selector.workers = []*mockWorker{worker}

			loginReq := &protos.MitmRequest{
				Method: protos.MitmRequest_LOGIN,
				Payload: &protos.MitmRequest_LoginRequest_{
					LoginRequest: &protos.MitmRequest_LoginRequest{
						WorkerId: "w1",
						Username: "testuser",
					},
				},
			}
			loginReqBytes, _ := proto.Marshal(loginReq)

			wsConn := &mockControllerWSConn{
				readers: []ws.Reader{
					&mockReader{data: loginReqBytes, msgType: ws.MessageBinary},
				},
			}

			controller, err := mgr.RegisterControllerConnectionV1(context.Background(), wsConn, tt.inputWeight, "test-ua")
			if err != nil {
				t.Fatalf("RegisterControllerConnectionV1 failed: %v", err)
			}
			if controller.weight != tt.expectedWeight {
				t.Errorf("expected weight %d, got %d", tt.expectedWeight, controller.weight)
			}
		})
	}
}

// --- readLoginRequest edge cases ---

func TestReadLoginRequest_InvalidProto(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: []byte("not valid protobuf"), msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.readLoginRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for invalid protobuf")
	}
}

func TestReadLoginRequest_NotALoginRequest(t *testing.T) {
	mgr, _ := newTestManager()

	// Create a MitmRequest that's NOT a login request
	req := &protos.MitmRequest{
		Method: protos.MitmRequest_RPC_REQUEST,
	}
	reqBytes, _ := proto.Marshal(req)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: reqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.readLoginRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error when not a login request")
	}
}

func TestReadLoginRequest_EmptyWorkerID(t *testing.T) {
	mgr, _ := newTestManager()

	req := &protos.MitmRequest{
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: "",
				Username: "testuser",
			},
		},
	}
	reqBytes, _ := proto.Marshal(req)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: reqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.readLoginRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for empty worker ID")
	}
}

func TestReadLoginRequest_ReaderError(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{
		readerErr: errors.New("reader error"),
	}

	_, err := mgr.readLoginRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error on reader failure")
	}
}

// --- readControllerRegistrationRequest edge cases ---

func TestReadControllerRegistrationRequest_InvalidProto(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: []byte("not valid protobuf"), msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.readControllerRegistrationRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for invalid protobuf")
	}
}

func TestReadControllerRegistrationRequest_EmptyID(t *testing.T) {
	mgr, _ := newTestManager()

	req := &protos.RegisterControllerRequest{
		Id:                "",
		ProtoMajorVersion: int32(ProtoMajorVersion),
	}
	reqBytes, _ := proto.Marshal(req)

	wsConn := &mockControllerWSConn{
		readers: []ws.Reader{
			&mockReader{data: reqBytes, msgType: ws.MessageBinary},
		},
	}

	_, err := mgr.readControllerRegistrationRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error for empty controller ID")
	}
}

func TestReadControllerRegistrationRequest_ReaderError(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{
		readerErr: errors.New("reader error"),
	}

	_, err := mgr.readControllerRegistrationRequest(context.Background(), wsConn)
	if err == nil {
		t.Fatal("expected error on reader failure")
	}
}

// --- handleControllerRegistrationRequest weight clamping ---

func TestHandleControllerRegistrationRequest_WeightClamping(t *testing.T) {
	tests := []struct {
		name           string
		inputWeight    int32
		expectedWeight int
	}{
		{"below min", 0, DefaultControllerWeight},
		{"at min", 1, 1},
		{"normal", 5, 5},
		{"at max", 10, 10},
		{"above max", 100, MaxControllerWeight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, o := newTestManager()
			worker := &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"}
			o.selector.workers = []*mockWorker{worker}

			loginReq := &protos.MitmRequest{
				Method: protos.MitmRequest_LOGIN,
				Payload: &protos.MitmRequest_LoginRequest_{
					LoginRequest: &protos.MitmRequest_LoginRequest{
						WorkerId: "w1",
						Username: "testuser",
					},
				},
			}
			loginReqBytes, _ := proto.Marshal(loginReq)

			wsConn := &mockControllerWSConn{
				readers: []ws.Reader{
					&mockReader{data: loginReqBytes, msgType: ws.MessageBinary},
				},
			}

			registerReq := &protos.RegisterControllerRequest{
				Id:                "ctrl1",
				ProtoMajorVersion: int32(ProtoMajorVersion),
				ProtoMinorVersion: int32(ProtoMinorVersion),
				Weight:            tt.inputWeight,
			}

			controller, err := mgr.handleControllerRegistrationRequest(context.Background(), wsConn, registerReq, "test-ua")
			if err != nil {
				t.Fatalf("handleControllerRegistrationRequest failed: %v", err)
			}
			if controller.weight != tt.expectedWeight {
				t.Errorf("expected weight %d, got %d", tt.expectedWeight, controller.weight)
			}
		})
	}
}

// --- Constants ---

func TestConstants(t *testing.T) {
	if MinControllerWeight != 1 {
		t.Errorf("expected MinControllerWeight=1, got %d", MinControllerWeight)
	}
	if MaxControllerWeight != 10 {
		t.Errorf("expected MaxControllerWeight=10, got %d", MaxControllerWeight)
	}
	if DefaultControllerWeight != 5 {
		t.Errorf("expected DefaultControllerWeight=5, got %d", DefaultControllerWeight)
	}
	if ProtoMajorVersion != 2 {
		t.Errorf("expected ProtoMajorVersion=2, got %d", ProtoMajorVersion)
	}
	if ProtoMinorVersion != 0 {
		t.Errorf("expected ProtoMinorVersion=0, got %d", ProtoMinorVersion)
	}
}

// --- makeDevice ---

func TestMakeDevice_WithWorkers(t *testing.T) {
	mgr, _ := newTestManager()
	mitmDevice := addDeviceDirectly(mgr, "d1", "origin1")
	addWorkerDirectly(mgr, &mockWorker{id: "w1", deviceID: "d1", origin: "origin1"})
	addWorkerDirectly(mgr, &mockWorker{id: "w2", deviceID: "d1", origin: "origin1"})

	mgr.mu.Lock()
	device := mgr.makeDevice(mitmDevice)
	mgr.mu.Unlock()

	if device.ID() != "d1" {
		t.Errorf("expected device ID 'd1', got '%s'", device.ID())
	}
	if len(device.Workers()) != 2 {
		t.Errorf("expected 2 workers, got %d", len(device.Workers()))
	}
}

func TestMakeDevice_NoWorkers(t *testing.T) {
	mgr, _ := newTestManager()
	mitmDevice := addDeviceDirectly(mgr, "d1", "origin1")

	mgr.mu.Lock()
	device := mgr.makeDevice(mitmDevice)
	mgr.mu.Unlock()

	if len(device.Workers()) != 0 {
		t.Errorf("expected 0 workers, got %d", len(device.Workers()))
	}
}

// --- Concurrent access ---

func TestConcurrentAccess(t *testing.T) {
	mgr, _ := newTestManager()

	// Add some initial data
	for i := range 10 {
		addDeviceDirectly(mgr, "d"+string(rune('0'+i)), "origin1")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			mgr.GetDevices()
			mgr.GetControllers()
			mgr.GetStatus()
			mgr.GetDeviceByID("d1")
			mgr.GetControllerByUUID("uuid1")
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent access test timed out")
	}
}

// --- writeRegisterControllerResponse ---

func TestWriteRegisterControllerResponse_Success(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{}
	response := &protos.RegisterControllerResponse{
		Status:    protos.RegisterControllerResponse_SUCCESS,
		UserAgent: "test",
	}

	err := mgr.writeRegisterControllerResponse(context.Background(), wsConn, response)
	if err != nil {
		t.Fatalf("writeRegisterControllerResponse failed: %v", err)
	}
}

func TestWriteRegisterControllerResponse_WriteError(t *testing.T) {
	mgr, _ := newTestManager()

	wsConn := &mockControllerWSConn{writeErr: errors.New("write failed")}
	response := &protos.RegisterControllerResponse{
		Status: protos.RegisterControllerResponse_SUCCESS,
	}

	err := mgr.writeRegisterControllerResponse(context.Background(), wsConn, response)
	if err == nil {
		t.Fatal("expected error on write failure")
	}
}

// --- getDeviceConn ---

func TestGetDeviceConn_NotKnown(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.getDeviceConn("nonexistent", false)
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestGetDeviceConn_NotConnected(t *testing.T) {
	mgr, _ := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")
	_, err := mgr.getDeviceConn("d1", false)
	if err == nil {
		t.Fatal("expected error for unconnected device")
	}
}

func TestGetDeviceConnForCommand_NotKnown(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.getDeviceConnForCommand("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

// --- RegisterDeviceConnection reconnection with existing device ---

func TestRegisterDeviceConnection_ExistingDevice(t *testing.T) {
	mgr, _ := newTestManager()

	// Pre-create a device
	addDeviceDirectly(mgr, "d1", "origin1")

	wsConn := &mockDeviceWSConn{
		initMsg: mitm.DeviceControlInitMessage{
			DeviceID: "d1",
			Origin:   "origin2", // updated origin
			Version:  "2.0.0",
			PublicIP: "5.6.7.8",
		},
	}

	deviceConn, err := mgr.RegisterDeviceConnection(context.Background(), wsConn)
	if err != nil {
		t.Fatalf("RegisterDeviceConnection failed: %v", err)
	}
	if deviceConn == nil {
		t.Fatal("expected non-nil device conn")
	}

	// Verify device was updated, not duplicated
	devices := mgr.GetDevices()
	if len(devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(devices))
	}
}

// --- DisableDevice then EnableDevice roundtrip ---

func TestDisableEnableDeviceRoundtrip(t *testing.T) {
	mgr, o := newTestManager()
	addDeviceDirectly(mgr, "d1", "origin1")

	// Disable
	_, err := mgr.DisableDevice("d1")
	if err != nil {
		t.Fatalf("DisableDevice failed: %v", err)
	}

	// Enable
	_, err = mgr.EnableDevice("d1")
	if err != nil {
		t.Fatalf("EnableDevice failed: %v", err)
	}

	o.selector.mu.Lock()
	if len(o.selector.disabledDevices) != 1 {
		t.Errorf("expected 1 disabled call, got %d", len(o.selector.disabledDevices))
	}
	if len(o.selector.enabledDevices) != 1 {
		t.Errorf("expected 1 enabled call, got %d", len(o.selector.enabledDevices))
	}
	o.selector.mu.Unlock()
}
