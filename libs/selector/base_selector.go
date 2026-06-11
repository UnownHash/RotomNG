package selector

import (
	"fmt"
	"sync"
	"time"

	"github.com/UnownHash/RotomNG/libs/settings"
)

// Weight bounds for worker selection.
const (
	MinimumWeight = 1
	MaximumWeight = 10
	DefaultWeight = 5
)

// Settings holds configuration for the worker selector.
type Settings struct {
	DeviceRateLimit SelectionHistoryConfig
}

// Validate checks that all nested configuration is valid.
func (s Settings) Validate() error {
	if err := s.DeviceRateLimit.Validate(); err != nil {
		return fmt.Errorf("invalid device rate limit: %w", err)
	}
	return nil
}

type selectorSettingsContainer = settings.Container[Settings]

// Config wraps the settings container for selector configuration.
type Config struct {
	*selectorSettingsContainer
}

// Init initializes the Config with the given settings.
func (cfg *Config) Init(s Settings) (err error) {
	cfg.selectorSettingsContainer, err = settings.NewContainer(s)
	return
}

// MITMWorker defines the interface that worker implementations must satisfy.
type MITMWorker interface {
	comparable

	IsZero() bool
	ID() string
	DeviceID() string
}

type selectableDevice[W MITMWorker] struct {
	id string

	workersInUse      int
	currentWeight     int // Total weight of all assigned controllers
	availableWorkers  []*selectableWorker[W]
	workerIDsSeen     map[string]*selectableWorker[W]
	lastSelectionTime time.Time
	selectionHistory  *SelectionHistory

	weightUsed [MaximumWeight + 1]int

	// selectionEnabled controls whether this device can be selected for new work
	selectionEnabled bool
}

// IsRateLimited returns whether the device has exceeded its rate limit.
func (d *selectableDevice[W]) IsRateLimited(now time.Time) bool {
	return d.selectionHistory.IsAtMaxSelections(now)
}

func (d *selectableDevice[W]) TotalWorkersCount() int {
	return len(d.workerIDsSeen)
}

func (d *selectableDevice[W]) AssignedWorkersCount() int {
	return d.workersInUse
}

func (d *selectableDevice[W]) AvailableWorkersCount() int {
	if d.selectionEnabled {
		return len(d.availableWorkers)
	}
	return 0
}

// GetWeightInfo returns current weight and maximum weight for the device.
func (d *selectableDevice[W]) GetWeightInfo() (int, int, float64) {
	maxWeight := MaximumWeight * d.TotalWorkersCount()
	var ratio float64
	if maxWeight > 0 {
		ratio = float64(d.currentWeight) / float64(maxWeight)
	}
	return d.currentWeight, maxWeight, ratio
}

type selectableWorker[W MITMWorker] struct {
	id string

	worker          W
	device          *selectableDevice[W]
	assignedWeight  int
	lastUnavailable time.Time
}

// baseSelector manages worker selection and device state for load balancing.
type baseSelector[W MITMWorker] struct {
	mu  sync.Mutex
	cfg Config

	devices                          map[string]*selectableDevice[W]
	deviceRateLimitSettingsContainer *settings.Container[SelectionHistoryConfig]

	getCurrentTime func() time.Time
}

func (ws *baseSelector[W]) claimWorkerFromDevice(device *selectableDevice[W], weight int, now time.Time) *selectableWorker[W] {
	worker := device.availableWorkers[0]
	device.availableWorkers = device.availableWorkers[1:]

	device.workersInUse++
	device.weightUsed[weight]++
	device.currentWeight += weight
	device.lastSelectionTime = now
	device.selectionHistory.Record(now)

	worker.assignedWeight = weight
	return worker
}

func (ws *baseSelector[W]) removeWorkerFromDevice(device *selectableDevice[W], worker *selectableWorker[W]) {
	if worker.assignedWeight > 0 {
		device.workersInUse--
		device.weightUsed[worker.assignedWeight]--
		device.currentWeight -= worker.assignedWeight
		worker.assignedWeight = 0
		return
	}

	// Check if worker exists in availableWorkers slice and remove it
	for i, w := range device.availableWorkers {
		if w == worker {
			device.availableWorkers = append(device.availableWorkers[:i], device.availableWorkers[i+1:]...)
			break
		}
	}
}

func (ws *baseSelector[W]) addNewDevice(deviceID string, selectionEnabled bool) *selectableDevice[W] {
	device := &selectableDevice[W]{
		id:               deviceID,
		availableWorkers: make([]*selectableWorker[W], 0),
		workerIDsSeen:    make(map[string]*selectableWorker[W]),
		selectionHistory: NewSelectionHistory(ws.deviceRateLimitSettingsContainer),
		selectionEnabled: selectionEnabled,
	}
	ws.devices[deviceID] = device
	return device
}

func (ws *baseSelector[W]) SetWorkerAvailable(worker W) {
	if worker.IsZero() {
		return
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	deviceID := worker.DeviceID()
	device := ws.devices[deviceID]
	if device == nil {
		// Enable device selection by default
		device = ws.addNewDevice(deviceID, true)
	}
	workerID := worker.ID()
	selWorker := device.workerIDsSeen[workerID]
	if selWorker == nil {
		selWorker = &selectableWorker[W]{
			id:     workerID,
			worker: worker,
			device: device,
		}
		device.workerIDsSeen[selWorker.id] = selWorker
	} else {
		// remove does NOT remove from workerIDsSeen.
		ws.removeWorkerFromDevice(device, selWorker)
		selWorker.worker = worker
	}
	selWorker.lastUnavailable = time.Time{}
	// Add worker to end of availableWorkers slice
	device.availableWorkers = append(device.availableWorkers, selWorker)
}

func (ws *baseSelector[W]) SetWorkerUnavailable(worker W) {
	if worker.IsZero() {
		return
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	selDevice := ws.devices[worker.DeviceID()]
	if selDevice == nil {
		return
	}
	selWorker := selDevice.workerIDsSeen[worker.ID()]
	// skip if unknown worker or different worker connection.
	// different worker connection means we'll have already
	// removed from device, etc.
	if selWorker == nil || selWorker.worker != worker {
		return
	}
	selWorker.lastUnavailable = time.Now()
	ws.removeWorkerFromDevice(selDevice, selWorker)
}

// SetCurrentTimeFunc allows setting a custom time function for testing.
func (ws *baseSelector[W]) SetCurrentTimeFunc(timeFunc func() time.Time) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.getCurrentTime = timeFunc
}

// EnableDevice enables device selection for the specified device.
func (ws *baseSelector[W]) EnableDevice(deviceID string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	device := ws.devices[deviceID]
	if device != nil {
		device.selectionEnabled = true
	}
}

// DisableDevice disables device selection for the specified device.
func (ws *baseSelector[W]) DisableDevice(deviceID string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	device := ws.devices[deviceID]
	if device == nil {
		ws.addNewDevice(deviceID, false)
	} else {
		device.selectionEnabled = false
	}
}

func (ws *baseSelector[W]) RemoveDeadDevice(deviceID string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	device := ws.devices[deviceID]
	if device == nil {
		return
	}
	for _, selWorker := range device.workerIDsSeen {
		if selWorker.assignedWeight > 0 {
			// cannot remove the device.
			return
		}
	}
	delete(ws.devices, deviceID)
}

// PruneWorkerIDsSeen will prune each device's tracking of a workerId
// if the workerId has not been available for the specified period
// of time. (E.g., it has been disconnected for too long).
func (ws *baseSelector[W]) PruneWorkerIDsSeen(dur time.Duration) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	now := time.Now()
	for _, selDevice := range ws.devices {
		for id, selWorker := range selDevice.workerIDsSeen {
			if selWorker.lastUnavailable.IsZero() {
				continue
			}
			if now.Sub(selWorker.lastUnavailable) >= dur {
				delete(selDevice.workerIDsSeen, id)
			}
		}
	}
}

func newBaseSelector[W MITMWorker](cfg Config) *baseSelector[W] {
	currentSettings := cfg.GetSettings()

	deviceRateLimitSettingsContainer, _ := settings.NewContainer(
		currentSettings.DeviceRateLimit,
	)

	bs := &baseSelector[W]{
		devices:                          make(map[string]*selectableDevice[W]),
		cfg:                              cfg,
		getCurrentTime:                   time.Now,
		deviceRateLimitSettingsContainer: deviceRateLimitSettingsContainer,
	}

	// Update rate limiter configs when settings change (hot-reload support)
	cfg.Notify(func(updated Settings) {
		_ = deviceRateLimitSettingsContainer.PutSettings(updated.DeviceRateLimit)
	})

	return bs
}
