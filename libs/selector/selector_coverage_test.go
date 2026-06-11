package selector

import (
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/settings"
)

// --- PruneWorkerIDsSeen ---

func TestPruneWorkerIDsSeen(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}

	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	// Make worker1 unavailable — this sets lastUnavailable to time.Now()
	selector.SetWorkerUnavailable(worker1)

	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("device should exist")
	}
	if _, ok := device.workerIDsSeen["worker1"]; !ok {
		t.Fatal("worker1 should still be in workerIDsSeen after SetWorkerUnavailable")
	}

	// Prune with a long duration — worker1's lastUnavailable is recent, should NOT be pruned
	selector.PruneWorkerIDsSeen(time.Hour)
	if _, ok := device.workerIDsSeen["worker1"]; !ok {
		t.Fatal("worker1 should not be pruned (lastUnavailable is too recent)")
	}

	// Manually set lastUnavailable to the past to simulate time passing
	device.workerIDsSeen["worker1"].lastUnavailable = time.Now().Add(-2 * time.Hour)

	selector.PruneWorkerIDsSeen(time.Hour)
	if _, ok := device.workerIDsSeen["worker1"]; ok {
		t.Error("worker1 should have been pruned after sufficient time")
	}

	// worker2 should still be tracked (lastUnavailable is zero — still available)
	if _, ok := device.workerIDsSeen["worker2"]; !ok {
		t.Error("worker2 should not be pruned (lastUnavailable is zero)")
	}
}

// --- RemoveDeadDevice with assigned worker ---

func TestRemoveDeadDevice_WithAssignedWorker(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Select the worker — this assigns weight to the device
	_, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("GetAvailableWorker() error = %v", err)
	}

	// Try to remove — should fail because worker has assignedWeight > 0
	selector.RemoveDeadDevice("device1")
	if _, ok := selector.devices["device1"]; !ok {
		t.Error("device should NOT be removed when worker has assignedWeight > 0")
	}
}

func TestRemoveDeadDevice_NonExistentDevice(_ *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Should not panic
	selector.RemoveDeadDevice("nonexistent")
}

// --- DisableDevice creates device if not exists ---

func TestDisableDevice_CreatesDevice(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// DisableDevice on nonexistent device should create it disabled
	selector.DisableDevice("device1")

	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("expected device to be created")
	}
	if device.selectionEnabled {
		t.Error("expected device to be created with selection disabled")
	}
}

// --- SelectionHistory: Prune, Reset, UpdateConfig ---

func TestSelectionHistory_Prune(t *testing.T) {
	cfg := SelectionHistoryConfig{
		Enabled:       true,
		MaxSelections: 5,
		Duration:      100 * time.Millisecond,
	}
	container, err := newHistorySettingsContainer(cfg)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	h := NewSelectionHistory(container)

	now := time.Now()
	h.Record(now.Add(-200 * time.Millisecond)) // expired
	h.Record(now.Add(-50 * time.Millisecond))  // not expired
	h.Record(now)                              // not expired

	h.Prune(now)

	// Only the two non-expired entries should remain
	if len(h.history) != 2 {
		t.Errorf("expected 2 entries after prune, got %d", len(h.history))
	}
}

func TestSelectionHistory_Reset(t *testing.T) {
	cfg := SelectionHistoryConfig{
		Enabled:       true,
		MaxSelections: 5,
		Duration:      time.Minute,
	}
	container, err := newHistorySettingsContainer(cfg)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	h := NewSelectionHistory(container)

	now := time.Now()
	h.Record(now)
	h.Record(now)
	h.Record(now)

	if len(h.history) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(h.history))
	}

	h.Reset()

	if len(h.history) != 0 {
		t.Errorf("expected 0 entries after reset, got %d", len(h.history))
	}
}

func TestSelectionHistory_UpdateConfig(t *testing.T) {
	cfg := SelectionHistoryConfig{
		Enabled:       true,
		MaxSelections: 5,
		Duration:      time.Minute,
	}
	container, err := newHistorySettingsContainer(cfg)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	h := NewSelectionHistory(container)

	newCfg := SelectionHistoryConfig{
		Enabled:       true,
		MaxSelections: 10,
		Duration:      2 * time.Minute,
	}
	err = h.UpdateConfig(newCfg)
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	// Verify the new config is active
	got := h.settingsContainer.GetSettings()
	if got.MaxSelections != 10 {
		t.Errorf("MaxSelections = %d, want 10", got.MaxSelections)
	}
	if got.Duration != 2*time.Minute {
		t.Errorf("Duration = %v, want 2m", got.Duration)
	}
}

func TestSelectionHistory_UpdateConfig_Invalid(t *testing.T) {
	cfg := SelectionHistoryConfig{
		Enabled:       true,
		MaxSelections: 5,
		Duration:      time.Minute,
	}
	container, err := newHistorySettingsContainer(cfg)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}
	h := NewSelectionHistory(container)

	// Invalid config should return error
	err = h.UpdateConfig(SelectionHistoryConfig{
		Enabled:       true,
		MaxSelections: 0,
		Duration:      time.Minute,
	})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

// --- newBaseSelector Notify callback ---

func TestNewBaseSelector_NotifyUpdatesRateLimitConfig(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{
		DeviceRateLimit: SelectionHistoryConfig{
			Enabled:       true,
			MaxSelections: 5,
			Duration:      time.Minute,
		},
	})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Update the settings — should trigger the Notify callback
	cfg.PutSettings(Settings{
		DeviceRateLimit: SelectionHistoryConfig{
			Enabled:       true,
			MaxSelections: 20,
			Duration:      5 * time.Minute,
		},
	})

	// Verify the device rate limit settings were updated
	got := selector.deviceRateLimitSettingsContainer.GetSettings()
	if got.MaxSelections != 20 {
		t.Errorf("MaxSelections = %d, want 20", got.MaxSelections)
	}
	if got.Duration != 5*time.Minute {
		t.Errorf("Duration = %v, want 5m", got.Duration)
	}
}

// --- Helper to create settings container for SelectionHistoryConfig ---

func newHistorySettingsContainer(cfg SelectionHistoryConfig) (*settings.Container[SelectionHistoryConfig], error) {
	return settings.NewContainer(cfg)
}

// --- SetWorkerAvailable with re-registration path ---

func TestSetWorkerAvailable_ReRegistration(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Select to make it assigned
	_, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("GetAvailableWorker() error = %v", err)
	}

	// Re-register the same worker ID with a new connection
	worker1New := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1New)

	// The new worker should be available
	device := selector.devices["device1"]
	if device.AvailableWorkersCount() != 1 {
		t.Errorf("expected 1 available worker after re-registration, got %d", device.AvailableWorkersCount())
	}
}

// --- SetWorkerUnavailable with different worker object ---

func TestSetWorkerUnavailable_DifferentWorkerObject(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Try to make unavailable with a different object with same ID
	worker1Different := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerUnavailable(worker1Different)

	// Should be a no-op since it's a different object
	device := selector.devices["device1"]
	if device.AvailableWorkersCount() != 1 {
		t.Errorf("expected 1 available worker (different object should be ignored), got %d", device.AvailableWorkersCount())
	}
}

// --- GetAvailableWorker weight clamping ---

func TestGetAvailableWorker_WeightClamping(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Weight below minimum should be clamped to default
	w, err := selector.GetAvailableWorker(0)
	if err != nil {
		t.Fatalf("GetAvailableWorker(0) error = %v", err)
	}
	if w == nil {
		t.Fatal("expected worker")
	}
}
