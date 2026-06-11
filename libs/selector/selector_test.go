package selector

import (
	"testing"
	"time"
)

// MockWorker implements the Worker interface for testing.
type MockWorker struct {
	id       string
	deviceID string
}

func (w *MockWorker) ID() string {
	return w.id
}

func (w *MockWorker) DeviceID() string {
	return w.deviceID
}

func (w *MockWorker) IsZero() bool {
	return w == nil
}

func calculateMaxWeightCapacity(d *selectableDevice[*MockWorker], weight int) int {
	totalWorkers := len(d.workerIDsSeen)
	if totalWorkers == 0 {
		return 0
	}
	return (MaximumWeight * totalWorkers) / weight
}

// getRemainingCapacity gets the remaining capacity for a specific weight on this device.
func getRemainingCapacity(d *selectableDevice[*MockWorker], weight int) int {
	return calculateMaxWeightCapacity(d, weight) - d.weightUsed[weight]
}

func TestBalancedSelector_GetAvailableWorker(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Test with no workers
	worker, err := selector.GetAvailableWorker(5)
	if err == nil {
		t.Error("Expected error when no workers available")
	}
	if worker != nil {
		t.Error("Expected nil worker when no workers available")
	}

	// Add workers to different devices
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}
	worker3 := &MockWorker{id: "worker3", deviceID: "device2"}
	worker4 := &MockWorker{id: "worker4", deviceID: "device2"}

	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)
	selector.SetWorkerAvailable(worker3)
	selector.SetWorkerAvailable(worker4)

	// Test getting workers with different weights
	selectedWorker1, err := selector.GetAvailableWorker(1) // High capacity weight
	if err != nil {
		t.Errorf("Expected a worker to be selected for weight 1, got error: %v", err)
	}
	if selectedWorker1 == nil {
		t.Error("Expected a worker to be selected for weight 1")
	}
	selectedWorker2, err := selector.GetAvailableWorker(10) // Low capacity weight
	if err != nil {
		t.Errorf("Expected a worker to be selected for weight 10, got error: %v", err)
	}
	if selectedWorker2 == nil {
		t.Error("Expected a worker to be selected for weight 10")
	}
}

func TestBalancedSelector_WeightCapacityTracking(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Add workers to a single device
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}

	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	// Device1 has 2 workers, so capacity for weight 1 should be (10 * 2) / 1 = 20
	// Capacity for weight 10 should be (10 * 2) / 10 = 2

	// Test capacity calculation
	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("Device1 should exist")
	}

	capacity1 := calculateMaxWeightCapacity(device, 1)
	capacity10 := calculateMaxWeightCapacity(device, 10)

	if capacity1 != 20 {
		t.Errorf("Expected capacity for weight 1 to be 20, got %d", capacity1)
	}
	if capacity10 != 2 {
		t.Errorf("Expected capacity for weight 10 to be 2, got %d", capacity10)
	}

	// Select workers and verify capacity decreases
	_, _ = selector.GetAvailableWorker(10)
	remaining := getRemainingCapacity(device, 10)
	if remaining != 1 {
		t.Errorf("Expected remaining capacity for weight 10 to be 1, got %d", remaining)
	}

	_, _ = selector.GetAvailableWorker(10)
	remaining = getRemainingCapacity(device, 10)
	if remaining != 0 {
		t.Errorf("Expected remaining capacity for weight 10 to be 0, got %d", remaining)
	}
}

func TestBalancedSelector_SetWorkerUnavailable(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Select the worker
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("Expected a worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected a worker to be selected")
	}

	// Make worker unavailable
	selector.SetWorkerUnavailable(worker1)
}

func TestBalancedSelector_WorkerForgottenWhenUnavailable(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}

	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	// Select a worker
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("Expected a worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected a worker to be selected")
	}

	// Make the worker unavailable
	selector.SetWorkerUnavailable(selectedWorker)

	// Worker is removed from available/assigned tracking but stays in workerIDsSeen
	// until PruneWorkerIDsSeen is called.
	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("Device should still exist since there's another worker")
	}
	if device.AssignedWorkersCount() != 0 {
		t.Errorf("Expected device to have 0 assigned workers, got %d", device.AssignedWorkersCount())
	}
	if device.AvailableWorkersCount() != 1 {
		t.Errorf("Expected device to have 1 available worker (the other one), got %d", device.AvailableWorkersCount())
	}
}

func TestBalancedSelector_RateLimit(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{
		DeviceRateLimit: SelectionHistoryConfig{
			Enabled: true, MaxSelections: 1, Duration: 100 * time.Millisecond,
		},
	})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Mock time for deterministic testing
	mockTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	currentMockTime := &mockTime
	selector.SetCurrentTimeFunc(func() time.Time {
		return *currentMockTime
	})

	// Use only one device to ensure we test rate limiting
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}

	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	// Select first worker
	selectedWorker1, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Errorf("Expected first worker to be selected, got error: %v", err)
	}
	if selectedWorker1 == nil {
		t.Error("Expected first worker to be selected")
	}

	// Return worker to make it available again
	selector.SetWorkerUnavailable(selectedWorker1)
	selector.SetWorkerAvailable(selectedWorker1)

	// Second selection should be rate limited since we're at the limit
	selectedWorker2, _ := selector.GetAvailableWorker(5)
	if selectedWorker2 != nil {
		t.Error("Expected second selection to be rate limited")
	}

	// Advance mock time beyond rate limit duration
	newTime := mockTime.Add(150 * time.Millisecond)
	currentMockTime = &newTime

	// Should be able to select again after rate limit expires
	selectedWorker3, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Errorf("Expected worker to be selected after rate limit expires, got error: %v", err)
	}
	if selectedWorker3 == nil {
		t.Error("Expected worker to be selected after rate limit expires")
	}
}

func TestBalancedSelector_WeightBounds(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Test weight below minimum
	selectedWorker, err := selector.GetAvailableWorker(0)
	if err != nil {
		t.Errorf("Expected worker to be selected with default weight for weight 0, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Error("Expected worker to be selected with default weight for weight 0")
	}

	// Return worker
	selector.SetWorkerUnavailable(selectedWorker)
	selector.SetWorkerAvailable(worker1)

	// Test weight above maximum
	selectedWorker, err = selector.GetAvailableWorker(15)
	if err != nil {
		t.Errorf("Expected worker to be selected with capped weight for weight 15, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Error("Expected worker to be selected with capped weight for weight 15")
	}
}

func TestDeviceCountMethods(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Test empty device
	_, _ = selector.GetAvailableWorker(5)

	// Add workers to test device count methods
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}
	worker3 := &MockWorker{id: "worker3", deviceID: "device2"}

	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)
	selector.SetWorkerAvailable(worker3)

	// Test device count methods
	device1 := selector.devices["device1"]
	device2 := selector.devices["device2"]

	if device1.TotalWorkersCount() != 2 {
		t.Errorf("Expected device1 to have 2 total workers, got %d", device1.TotalWorkersCount())
	}

	if device1.AvailableWorkersCount() != 2 {
		t.Errorf("Expected device1 to have 2 available workers, got %d", device1.AvailableWorkersCount())
	}

	selector.DisableDevice(device1.id)
	if device1.AvailableWorkersCount() != 0 {
		t.Errorf("Expected device1 to have 0 available workers when disabled, got %d", device1.AvailableWorkersCount())
	}
	selector.EnableDevice(device1.id)

	if device1.AssignedWorkersCount() != 0 {
		t.Errorf("Expected device1 to have 0 assigned workers, got %d", device1.AssignedWorkersCount())
	}

	if device2.TotalWorkersCount() != 1 {
		t.Errorf("Expected device2 to have 1 total worker, got %d", device2.TotalWorkersCount())
	}

	// Select a worker and test assigned count
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("Expected a worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected a worker to be selected")
	}

	// Find which device the worker belongs to
	var selectedDevice *selectableDevice[*MockWorker]
	if selectedWorker.DeviceID() == "device1" {
		selectedDevice = device1
	} else {
		selectedDevice = device2
	}

	if selectedDevice.AssignedWorkersCount() != 1 {
		t.Errorf("Expected selected device to have 1 assigned worker, got %d", selectedDevice.AssignedWorkersCount())
	}
	if selectedDevice.AvailableWorkersCount() != selectedDevice.TotalWorkersCount()-1 {
		t.Errorf("Expected available workers to be total - assigned")
	}
}

func TestGetWeightInfo(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	device := selector.devices["device1"]

	// Test initial weight info
	currentWeight, maxWeight, ratio := device.GetWeightInfo()
	if currentWeight != 0 {
		t.Errorf("Expected current weight to be 0, got %d", currentWeight)
	}
	if maxWeight != 20 { // 10 * 2 workers
		t.Errorf("Expected max weight to be 20, got %d", maxWeight)
	}
	if ratio != 0.0 {
		t.Errorf("Expected ratio to be 0.0, got %f", ratio)
	}

	// Select workers with different weights
	_, _ = selector.GetAvailableWorker(3)
	_, _ = selector.GetAvailableWorker(7)

	currentWeight, maxWeight, ratio = device.GetWeightInfo()
	if currentWeight != 10 { // 3 + 7
		t.Errorf("Expected current weight to be 10, got %d", currentWeight)
	}
	if maxWeight != 20 {
		t.Errorf("Expected max weight to be 20, got %d", maxWeight)
	}
	if ratio != 0.5 { // 10/20
		t.Errorf("Expected ratio to be 0.5, got %f", ratio)
	}
}

func TestRemoveDeadDevice(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Test making unavailable a worker from non-existent device - should not panic
	nonExistentWorker := &MockWorker{id: "nonexistent", deviceID: "nonexistent"}
	selector.SetWorkerUnavailable(nonExistentWorker) // Should not panic

	// Test making unavailable a non-existent worker from existing device - should not panic
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	nonExistentWorkerSameDevice := &MockWorker{id: "nonexistent", deviceID: "device1"}
	selector.SetWorkerUnavailable(nonExistentWorkerSameDevice) // Should not panic

	// Verify worker1 is still available
	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("Device should exist")
	}
	if device.AvailableWorkersCount() != 1 {
		t.Errorf("Expected 1 available worker, got %d", device.AvailableWorkersCount())
	}

	// Test making unavailable a worker that's in use - should forget the worker completely
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("Expected a worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected a worker to be selected")
	}

	device = selector.devices["device1"]
	initialAssigned := device.AssignedWorkersCount()
	selector.SetWorkerUnavailable(selectedWorker)
	finalAssigned := device.AssignedWorkersCount()

	if finalAssigned != initialAssigned-1 {
		t.Errorf("Expected assigned count to decrease by 1, got %d -> %d", initialAssigned, finalAssigned)
	}

	// Device is sticky until explicitly removed
	if len(selector.devices) != 1 {
		t.Error("Expected device to still exist after last worker is made unavailable")
	}

	// Explicitly remove the dead device
	selector.RemoveDeadDevice("device1")
	if len(selector.devices) != 0 {
		t.Error("Expected device to be removed after RemoveDeadDevice call")
	}
}

func TestSetWorkerUnavailable_EdgeCases(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Test making unavailable a worker from non-existent device
	nonExistentWorker := &MockWorker{id: "nonexistent", deviceID: "nonexistent"}
	selector.SetWorkerUnavailable(nonExistentWorker) // Should not panic

	// Test making unavailable a non-existent worker from existing device
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	nonExistentWorkerSameDevice := &MockWorker{id: "nonexistent", deviceID: "device1"}
	selector.SetWorkerUnavailable(nonExistentWorkerSameDevice) // Should not panic

	// Test making unavailable a worker that's available
	device := selector.devices["device1"]
	initialAvailable := device.AvailableWorkersCount()
	selector.SetWorkerUnavailable(worker1)
	finalAvailable := device.AvailableWorkersCount()

	if finalAvailable != initialAvailable-1 {
		t.Errorf("Expected available count to decrease by 1, got %d -> %d", initialAvailable, finalAvailable)
	}

	// Test making unavailable a worker that's in use
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}
	selector.SetWorkerAvailable(worker2)
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("Expected a worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected a worker to be selected")
	}

	device = selector.devices["device1"]
	initialAssigned := device.AssignedWorkersCount()
	selector.SetWorkerUnavailable(selectedWorker)
	finalAssigned := device.AssignedWorkersCount()

	if finalAssigned != initialAssigned-1 {
		t.Errorf("Expected assigned count to decrease by 1, got %d -> %d", initialAssigned, finalAssigned)
	}
}

func TestNewBaseSelector_RateLimitValidation(t *testing.T) {
	// Test with invalid rate limit configurations
	invalidConfigs := []Settings{
		{DeviceRateLimit: SelectionHistoryConfig{Enabled: true, MaxSelections: 0, Duration: 100 * time.Millisecond}},
		{DeviceRateLimit: SelectionHistoryConfig{Enabled: true, MaxSelections: 1, Duration: 0}},
		{DeviceRateLimit: SelectionHistoryConfig{Enabled: true, MaxSelections: -1, Duration: 100 * time.Millisecond}},
		{DeviceRateLimit: SelectionHistoryConfig{Enabled: true, MaxSelections: 1, Duration: -100 * time.Millisecond}},
	}

	for i, settings := range invalidConfigs {
		var cfg Config
		err := cfg.Init(settings)
		if err == nil {
			t.Errorf("Test case %d: Expected validation error for invalid config, got nil", i)
		}
	}

	// Test with valid rate limit configuration
	validSettings := Settings{
		DeviceRateLimit: SelectionHistoryConfig{Enabled: true, MaxSelections: 2, Duration: 100 * time.Millisecond},
	}
	var cfg Config
	err := cfg.Init(validSettings)
	if err != nil {
		t.Errorf("Expected no error for valid config, got %v", err)
	}
	selector := NewBalancedSelector[*MockWorker](cfg)
	if selector == nil {
		t.Error("Expected selector to be created for valid config")
	}
}

func TestBalancedSelector_CapacityExhaustion(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Add 2 workers to test capacity logic
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	// Device capacity for weight 10 should be (10 * 2) / 10 = 2
	// We should be able to select 2 workers with weight 10
	selectedWorker1, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Errorf("Expected first worker to be selected, got error: %v", err)
	}
	if selectedWorker1 == nil {
		t.Error("Expected first worker to be selected")
	}

	selectedWorker2, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Errorf("Expected second worker to be selected, got error: %v", err)
	}
	if selectedWorker2 == nil {
		t.Error("Expected second worker to be selected")
	}

	// Add a third worker
	worker3 := &MockWorker{id: "worker3", deviceID: "device1"}
	selector.SetWorkerAvailable(worker3)

	// Now capacity should be (10 * 3) / 10 = 3, but we already used 2, so 1 remaining
	selectedWorker3, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Errorf("Expected third worker to be selected, got error: %v", err)
	}
	if selectedWorker3 == nil {
		t.Error("Expected third worker to be selected")
	}

	// Add a fourth worker
	worker4 := &MockWorker{id: "worker4", deviceID: "device1"}
	selector.SetWorkerAvailable(worker4)

	// Now capacity should be (10 * 4) / 10 = 4, but we already used 3, so 1 remaining
	selectedWorker4, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Errorf("Expected fourth worker to be selected, got error: %v", err)
	}
	if selectedWorker4 == nil {
		t.Error("Expected fourth worker to be selected")
	}

	// Add a fifth worker and try to select - should work since capacity is now 5
	worker5 := &MockWorker{id: "worker5", deviceID: "device1"}
	selector.SetWorkerAvailable(worker5)

	selectedWorker5, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Errorf("Expected fifth worker to be selected, got error: %v", err)
	}
	if selectedWorker5 == nil {
		t.Error("Expected fifth worker to be selected")
	}

	// Now we should be at capacity (5 workers with weight 10, capacity = (10*5)/10 = 5)
	// Adding another worker and trying to select should still work since capacity increases
	worker6 := &MockWorker{id: "worker6", deviceID: "device1"}
	selector.SetWorkerAvailable(worker6)

	selectedWorker6, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Errorf("Expected sixth worker to be selected, got error: %v", err)
	}
	if selectedWorker6 == nil {
		t.Error("Expected sixth worker to be selected")
	}

	// Test capacity exhaustion with a fixed number of workers
	// Create a new selector with exactly 2 workers and try to exceed capacity
	var cfg2 Config
	cfg2.Init(Settings{})
	selector2 := NewBalancedSelector[*MockWorker](cfg2)
	w1 := &MockWorker{id: "w1", deviceID: "device2"}
	w2 := &MockWorker{id: "w2", deviceID: "device2"}
	selector2.SetWorkerAvailable(w1)
	selector2.SetWorkerAvailable(w2)

	// Select both workers with weight 10 (capacity = 2)
	sel1, err := selector2.GetAvailableWorker(10)
	sel2, err2 := selector2.GetAvailableWorker(10)

	if err != nil || err2 != nil || sel1 == nil || sel2 == nil {
		t.Error("Expected both workers to be selected")
	}

	// Now try to select a third worker with weight 10 - should fail
	// (no more available workers, and capacity is exhausted)
	sel3, _ := selector2.GetAvailableWorker(10)
	if sel3 != nil {
		t.Error("Expected third selection to fail - no available workers")
	}
}

func TestBalancedSelector_UnclaimWorkerFromDevice(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Add two workers so device doesn't get removed when we unclaim one
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	worker2 := &MockWorker{id: "worker2", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)
	selector.SetWorkerAvailable(worker2)

	// Select worker to put it in use
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Fatalf("Expected worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected worker to be selected")
	}

	device := selector.devices["device1"]
	selWorker := device.workerIDsSeen[selectedWorker.ID()]

	// Test the unclaim method directly
	initialAssigned := device.AssignedWorkersCount()
	selector.removeWorkerFromDevice(device, selWorker)

	if device.AssignedWorkersCount() != initialAssigned-1 {
		t.Errorf("Expected assigned count to decrease by 1, got %d -> %d", initialAssigned, device.AssignedWorkersCount())
	}

	// Verify device still exists since there's another worker
	device = selector.devices["device1"]
	if device == nil {
		t.Fatal("Device should still exist since there's another worker")
	}

	// Test unclaiming a worker that's not in use (assignedWeight = 0)
	// Get the other worker that should still be available
	var otherWorker *selectableWorker[*MockWorker]
	for _, sw := range device.workerIDsSeen {
		if sw.assignedWeight == 0 { // Find the worker that's not in use
			otherWorker = sw
			break
		}
	}
	if otherWorker == nil {
		t.Fatal("Should have found a worker that's not in use")
	}

	initialAssigned = device.AssignedWorkersCount()
	selector.removeWorkerFromDevice(device, otherWorker)

	if device.AssignedWorkersCount() != initialAssigned {
		t.Errorf("Expected assigned count to stay same for worker not in use, got %d -> %d", initialAssigned, device.AssignedWorkersCount())
	}
}

func TestBalancedSelector_MultipleDeviceSelection(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Create workers on different devices
	device1Workers := []*MockWorker{
		{id: "d1w1", deviceID: "device1"},
		{id: "d1w2", deviceID: "device1"},
	}
	device2Workers := []*MockWorker{
		{id: "d2w1", deviceID: "device2"},
		{id: "d2w2", deviceID: "device2"},
		{id: "d2w3", deviceID: "device2"},
	}

	// Add all workers
	for _, worker := range device1Workers {
		selector.SetWorkerAvailable(worker)
	}
	for _, worker := range device2Workers {
		selector.SetWorkerAvailable(worker)
	}

	// With the new ratio-based selection: ratio = (weight * d.weightUsed[weight]) / (10 * len(d.workerIDsSeen))
	// Initially, both devices have ratio = 0 since weightUsed[weight] = 0
	// Device1: ratio = (1 * 0) / (10 * 2) = 0
	// Device2: ratio = (1 * 0) / (10 * 3) = 0
	// Since ratios are equal, tie-breaking uses lastSelectionTime (both are zero initially)
	// The selection will be deterministic based on map iteration order or device creation order

	selectedWorker, err := selector.GetAvailableWorker(1)
	if err != nil {
		t.Fatalf("Expected worker to be selected, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Fatal("Expected worker to be selected")
	}

	// Now test with weight 10 on the same device that was selected first
	// The device that was selected first now has: ratio = (10 * 0) / (10 * workers) = 0
	// The other device still has: ratio = (10 * 0) / (10 * workers) = 0
	// Since ratios are still equal, the device with earlier lastSelectionTime should be selected
	// But the first device now has a more recent lastSelectionTime, so the other device should be preferred
	selectedWorker2, err := selector.GetAvailableWorker(10)
	if err != nil {
		t.Fatalf("Expected second worker to be selected, got error: %v", err)
	}
	if selectedWorker2 == nil {
		t.Fatal("Expected second worker to be selected")
	}

	// Now let's create a scenario where ratios differ
	// Select more workers with weight 1 from the first device to increase its ratio
	selectedWorker3, err := selector.GetAvailableWorker(1) // This should go to the device with lower ratio for weight 1
	if err != nil {
		t.Fatalf("Expected third worker to be selected, got error: %v", err)
	}
	if selectedWorker3 == nil {
		t.Fatal("Expected third worker to be selected")
	}

	// After this selection, one device will have weightUsed[1] = 1, the other = 0
	// The device with weightUsed[1] = 1 will have ratio = (1 * 1) / (10 * workers) > 0
	// The device with weightUsed[1] = 0 will have ratio = (1 * 0) / (10 * workers) = 0
	// So the device with ratio = 0 should be preferred for the next weight 1 selection
	selectedWorker4, err := selector.GetAvailableWorker(1)
	if err != nil {
		t.Fatalf("Expected fourth worker to be selected, got error: %v", err)
	}
	if selectedWorker4 == nil {
		t.Fatal("Expected fourth worker to be selected")
	}

	// Verify that the ratio-based selection is working by checking that devices are being balanced
	// We can't predict exact device selection due to tie-breaking, but we can verify workers are being assigned
	totalAssigned := 0
	totalAvailable := 0
	for _, device := range selector.devices {
		totalAssigned += device.AssignedWorkersCount()
		totalAvailable += device.AvailableWorkersCount()
	}
	if totalAssigned != 4 {
		t.Errorf("Expected 4 workers assigned, got %d", totalAssigned)
	}
	if totalAvailable != 1 { // 5 total workers - 4 assigned = 1 available
		t.Errorf("Expected 1 worker available, got %d", totalAvailable)
	}
}

// TestWorkerObjectIdentityHandling tests how the selector handles multiple calls with same worker ID but different objects.
func TestWorkerObjectIdentityHandling(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Test 1: Multiple calls to SetWorkerAvailable() with same worker ID but different MockWorker objects
	// should result in accurate counts (only 1 worker available)
	t.Run("SetWorkerAvailable_SameID_DifferentObjects", func(t *testing.T) {
		// Reset selector for clean test
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
		worker1Duplicate := &MockWorker{id: "worker1", deviceID: "device1"} // Same ID, different object

		// Add first worker
		selector.SetWorkerAvailable(worker1)
		device := selector.devices["device1"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("After first SetWorkerAvailable: expected 1 available, got %d", device.AvailableWorkersCount())
		}

		// Add duplicate worker with same ID but different object
		selector.SetWorkerAvailable(worker1Duplicate)
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("After duplicate SetWorkerAvailable: expected 1 available, got %d", device.AvailableWorkersCount())
		}

		// Verify device has only 1 worker total
		if device.TotalWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 total worker, got %d", device.TotalWorkersCount())
		}
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 available worker, got %d", device.AvailableWorkersCount())
		}

		// Verify the worker object was updated to the latest one
		selWorker := device.workerIDsSeen["worker1"]
		if selWorker == nil {
			t.Fatal("Worker should exist in workerIDsSeen")
		}
		if selWorker.worker != worker1Duplicate {
			t.Error("Worker object should be updated to the latest one")
		}
	})

	// Test 2: Multiple calls to SetWorkerUnavailable() for different worker objects but same workerId
	// should result in unclaim being skipped if different object, even if worker id is same
	t.Run("SetWorkerUnavailable_SameID_DifferentObjects", func(t *testing.T) {
		// Reset selector for clean test
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker2 := &MockWorker{id: "worker2", deviceID: "device2"}
		worker2Different := &MockWorker{id: "worker2", deviceID: "device2"} // Same ID, different object

		// Add worker
		selector.SetWorkerAvailable(worker2)
		device := selector.devices["device2"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("After SetWorkerAvailable: expected 1 available, got %d", device.AvailableWorkersCount())
		}

		// Try to make unavailable using different object with same ID - should be skipped
		selector.SetWorkerUnavailable(worker2Different)
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("After SetWorkerUnavailable with different object: expected 1 available, got %d", device.AvailableWorkersCount())
		}

		// Verify worker is still available
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 available worker, got %d", device.AvailableWorkersCount())
		}

		// Now make unavailable using correct object - should work
		selector.SetWorkerUnavailable(worker2)
		if device.AvailableWorkersCount() != 0 {
			t.Errorf("After SetWorkerUnavailable with correct object: expected 0 available, got %d", device.AvailableWorkersCount())
		}
	})

	// Test 3: Worker is in-use and then SetWorkerAvailable() is called for same worker id but different worker object
	// This should result in there being 1 worker available and none in use
	t.Run("SetWorkerAvailable_InUseWorker_DifferentObject", func(t *testing.T) {
		// Reset selector for clean test
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker3 := &MockWorker{id: "worker3", deviceID: "device3"}
		worker3Replacement := &MockWorker{id: "worker3", deviceID: "device3"} // Same ID, different object

		// Add worker and select it (put it in use)
		selector.SetWorkerAvailable(worker3)
		selectedWorker, err := selector.GetAvailableWorker(5)
		if err != nil {
			t.Fatalf("Expected worker to be selected, got error: %v", err)
		}
		if selectedWorker == nil {
			t.Fatal("Expected worker to be selected")
		}
		device := selector.devices["device3"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AssignedWorkersCount() != 1 {
			t.Errorf("After GetAvailableWorker: expected 1 assigned, got %d", device.AssignedWorkersCount())
		}

		// Verify worker is in use
		if device.AssignedWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 assigned worker, got %d", device.AssignedWorkersCount())
		}
		if device.AvailableWorkersCount() != 0 {
			t.Errorf("Expected device to have 0 available workers, got %d", device.AvailableWorkersCount())
		}

		// Now call SetWorkerAvailable with same ID but different object
		// This should unclaim the in-use worker and make the new one available
		selector.SetWorkerAvailable(worker3Replacement)

		// Get the device again since it might have been recreated
		device = selector.devices["device3"]
		if device == nil {
			t.Fatal("Device should exist after SetWorkerAvailable")
		}

		// Verify device state
		if device.AssignedWorkersCount() != 0 {
			t.Errorf("Expected device to have 0 assigned workers, got %d", device.AssignedWorkersCount())
		}
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 available worker, got %d", device.AvailableWorkersCount())
		}
		if device.TotalWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 total worker, got %d", device.TotalWorkersCount())
		}

		// Verify the worker object was updated to the replacement
		selWorker := device.workerIDsSeen["worker3"]
		if selWorker == nil {
			t.Fatal("Worker should exist in workerIDsSeen")
		}
		if selWorker.worker != worker3Replacement {
			t.Error("Worker object should be updated to the replacement")
		}
		if selWorker.assignedWeight != 0 {
			t.Errorf("Worker should have assignedWeight 0, got %d", selWorker.assignedWeight)
		}
	})

	// Test 4: Additional test for SetWorkerUnavailable with in-use worker using different object
	t.Run("SetWorkerUnavailable_InUseWorker_DifferentObject", func(t *testing.T) {
		// Reset selector for clean test
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker4 := &MockWorker{id: "worker4", deviceID: "device4"}
		worker4Different := &MockWorker{id: "worker4", deviceID: "device4"} // Same ID, different object

		// Add worker and select it (put it in use)
		selector.SetWorkerAvailable(worker4)
		selectedWorker, err := selector.GetAvailableWorker(7)
		if err != nil {
			t.Fatalf("Expected worker to be selected, got error: %v", err)
		}
		if selectedWorker == nil {
			t.Fatal("Expected worker to be selected")
		}
		device := selector.devices["device4"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AssignedWorkersCount() != 1 {
			t.Errorf("After GetAvailableWorker: expected 1 assigned, got %d", device.AssignedWorkersCount())
		}

		// Try to make unavailable using different object with same ID - should be skipped
		selector.SetWorkerUnavailable(worker4Different)
		if device.AssignedWorkersCount() != 1 {
			t.Errorf("After SetWorkerUnavailable with different object: expected 1 assigned, got %d", device.AssignedWorkersCount())
		}

		// Verify worker is still in use
		if device.AssignedWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 assigned worker, got %d", device.AssignedWorkersCount())
		}

		// Verify the original worker object is still referenced
		selWorker := device.workerIDsSeen["worker4"]
		if selWorker == nil {
			t.Fatal("Worker should exist in workerIDsSeen")
		}
		if selWorker.worker != worker4 {
			t.Error("Worker object should still be the original one")
		}
		if selWorker.assignedWeight != 7 {
			t.Errorf("Worker should have assignedWeight 7, got %d", selWorker.assignedWeight)
		}

		// Now make unavailable using correct object - should work
		selector.SetWorkerUnavailable(worker4)
		if device.AssignedWorkersCount() != 0 {
			t.Errorf("After SetWorkerUnavailable with correct object: expected 0 assigned, got %d", device.AssignedWorkersCount())
		}
	})

	// Test 5: Complex scenario with multiple workers and mixed operations
	t.Run("ComplexScenario_MultipleWorkers_MixedOperations", func(t *testing.T) {
		// Reset selector for clean test
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		// Create multiple worker objects with same IDs
		worker5a := &MockWorker{id: "worker5", deviceID: "device5"}
		worker5b := &MockWorker{id: "worker5", deviceID: "device5"} // Same ID, different object
		worker6a := &MockWorker{id: "worker6", deviceID: "device5"}
		worker6b := &MockWorker{id: "worker6", deviceID: "device5"} // Same ID, different object

		// Add workers
		selector.SetWorkerAvailable(worker5a)
		selector.SetWorkerAvailable(worker6a)
		device := selector.devices["device5"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AvailableWorkersCount() != 2 {
			t.Errorf("After adding 2 workers: expected 2 available, got %d", device.AvailableWorkersCount())
		}

		// Select one worker
		selectedWorker, err := selector.GetAvailableWorker(3)
		if err != nil {
			t.Fatalf("Expected worker to be selected, got error: %v", err)
		}
		if selectedWorker == nil {
			t.Fatal("Expected worker to be selected")
		}
		selectedWorkerID := selectedWorker.ID()

		// Replace both workers with different objects
		// This will unclaim the in-use worker and make both workers available with new objects
		selector.SetWorkerAvailable(worker5b)
		selector.SetWorkerAvailable(worker6b)

		// Verify device state
		if device.TotalWorkersCount() != 2 {
			t.Errorf("Expected device to have 2 total workers, got %d", device.TotalWorkersCount())
		}
		if device.AvailableWorkersCount() != 2 {
			t.Errorf("Expected device to have 2 available workers, got %d", device.AvailableWorkersCount())
		}
		if device.AssignedWorkersCount() != 0 {
			t.Errorf("Expected device to have 0 assigned workers, got %d", device.AssignedWorkersCount())
		}

		// Verify worker objects were updated
		selWorker5 := device.workerIDsSeen["worker5"]
		selWorker6 := device.workerIDsSeen["worker6"]
		if selWorker5 == nil || selWorker6 == nil {
			t.Fatal("Both workers should exist in workerIDsSeen")
		}
		if selWorker5.worker != worker5b {
			t.Error("Worker5 object should be updated to worker5b")
		}
		if selWorker6.worker != worker6b {
			t.Error("Worker6 object should be updated to worker6b")
		}

		// Try to make unavailable using old objects - should be skipped
		var oldWorkerObject *MockWorker
		if selectedWorkerID == "worker5" {
			oldWorkerObject = worker5a
		} else {
			oldWorkerObject = worker6a
		}
		selector.SetWorkerUnavailable(oldWorkerObject)
		if device.AvailableWorkersCount() != 2 {
			t.Errorf("After SetWorkerUnavailable with old object: expected 2 available, got %d", device.AvailableWorkersCount())
		}
	})
}

// TestWorkerForgottenWhenUnavailableWithoutReplacement tests the specific scenario where
// workers are forgotten about when made unavailable and another worker with the same id
// has not been made available in its place. Devices persist until RemoveDeadDevice is called.
func TestWorkerForgottenWhenUnavailableWithoutReplacement(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Test 1: Single worker made unavailable should be completely forgotten
	t.Run("SingleWorkerForgotten", func(t *testing.T) {
		worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
		selector.SetWorkerAvailable(worker1)

		// Verify worker is available
		device := selector.devices["device1"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AvailableWorkersCount() != 1 {
			t.Errorf("Expected 1 available, got %d", device.AvailableWorkersCount())
		}

		// Make worker unavailable - should be completely forgotten
		selector.SetWorkerUnavailable(worker1)

		// Verify worker is forgotten
		if device.AvailableWorkersCount() != 0 {
			t.Errorf("Expected 0 available after worker unavailable, got %d", device.AvailableWorkersCount())
		}

		// Device is no longer automatically removed - it persists until RemoveDeadDevice is called
		if len(selector.devices) != 1 {
			t.Error("Expected device to still exist after last worker is made unavailable")
		}

		// Explicitly remove the dead device
		selector.RemoveDeadDevice("device1")
		if len(selector.devices) != 0 {
			t.Error("Expected device to be removed after RemoveDeadDevice call")
		}
	})

	// Test 2: In-use worker made unavailable should be completely forgotten
	t.Run("InUseWorkerForgotten", func(t *testing.T) {
		// Reset selector
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker2 := &MockWorker{id: "worker2", deviceID: "device2"}
		selector.SetWorkerAvailable(worker2)

		// Select the worker (put it in use)
		selectedWorker, err := selector.GetAvailableWorker(7)
		if err != nil {
			t.Fatalf("Expected worker to be selected, got error: %v", err)
		}
		if selectedWorker == nil {
			t.Fatal("Expected worker to be selected")
		}
		device := selector.devices["device2"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AssignedWorkersCount() != 1 {
			t.Errorf("Expected 1 assigned after selection, got %d", device.AssignedWorkersCount())
		}

		// Make the in-use worker unavailable - should be completely forgotten
		selector.SetWorkerUnavailable(selectedWorker)

		// Verify worker is forgotten
		if device.AssignedWorkersCount() != 0 {
			t.Errorf("Expected 0 assigned after in-use worker unavailable, got %d", device.AssignedWorkersCount())
		}

		// Device is no longer automatically removed - it persists until RemoveDeadDevice is called
		if len(selector.devices) != 1 {
			t.Error("Expected device to still exist after last worker is made unavailable")
		}

		// Explicitly remove the dead device
		selector.RemoveDeadDevice("device2")
		if len(selector.devices) != 0 {
			t.Error("Expected device to be removed after RemoveDeadDevice call")
		}
	})

	// Test 3: Multiple workers, one made unavailable, others remain
	t.Run("OneWorkerForgottenOthersRemain", func(t *testing.T) {
		// Reset selector
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker3a := &MockWorker{id: "worker3a", deviceID: "device3"}
		worker3b := &MockWorker{id: "worker3b", deviceID: "device3"}
		worker3c := &MockWorker{id: "worker3c", deviceID: "device3"}

		selector.SetWorkerAvailable(worker3a)
		selector.SetWorkerAvailable(worker3b)
		selector.SetWorkerAvailable(worker3c)

		// Verify all workers are available
		device := selector.devices["device3"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AvailableWorkersCount() != 3 {
			t.Errorf("Expected 3 available, got %d", device.AvailableWorkersCount())
		}

		// Select one worker
		selectedWorker, err := selector.GetAvailableWorker(5)
		if err != nil {
			t.Fatalf("Expected worker to be selected, got error: %v", err)
		}
		if selectedWorker == nil {
			t.Fatal("Expected worker to be selected")
		}

		// Make the selected worker unavailable - should be forgotten
		selector.SetWorkerUnavailable(selectedWorker)

		// Verify counts: selected worker removed from available/assigned tracking
		// (stays in workerIDsSeen until PruneWorkerIDsSeen is called)
		if device.AvailableWorkersCount() != 2 {
			t.Errorf("Expected 2 available after one worker unavailable, got %d", device.AvailableWorkersCount())
		}
		if device.AssignedWorkersCount() != 0 {
			t.Errorf("Expected 0 assigned after worker made unavailable, got %d", device.AssignedWorkersCount())
		}
	})

	// Test 4: Worker made unavailable, then same ID made available again (different object)
	t.Run("WorkerReplacedWithSameID", func(t *testing.T) {
		// Reset selector
		var cfg Config
		cfg.Init(Settings{})
		selector = NewBalancedSelector[*MockWorker](cfg)

		worker4a := &MockWorker{id: "worker4", deviceID: "device4"}
		selector.SetWorkerAvailable(worker4a)

		// Select the worker
		selectedWorker, err := selector.GetAvailableWorker(3)
		if err != nil {
			t.Fatalf("Expected worker to be selected, got error: %v", err)
		}
		if selectedWorker == nil {
			t.Fatal("Expected worker to be selected")
		}

		// Make worker unavailable - should be forgotten
		selector.SetWorkerUnavailable(selectedWorker)

		// Verify worker is forgotten but device persists
		device := selector.devices["device4"]
		if device == nil {
			t.Fatal("Device should exist")
		}
		if device.AvailableWorkersCount() != 0 {
			t.Errorf("Expected 0 available after worker unavailable, got %d", device.AvailableWorkersCount())
		}
		if len(selector.devices) != 1 {
			t.Error("Expected device to still exist after worker is made unavailable")
		}

		// Explicitly remove the dead device
		selector.RemoveDeadDevice("device4")
		if len(selector.devices) != 0 {
			t.Error("Expected device to be removed after RemoveDeadDevice call")
		}

		// Now add a new worker with the same ID (simulating reconnection)
		worker4b := &MockWorker{id: "worker4", deviceID: "device4"} // Same ID, different object
		selector.SetWorkerAvailable(worker4b)

		// Verify device is recreated
		device = selector.devices["device4"]
		if device == nil {
			t.Fatal("Device should be recreated when new worker is added")
		}
		if device.TotalWorkersCount() != 1 {
			t.Errorf("Expected device to have 1 total worker, got %d", device.TotalWorkersCount())
		}

		// Verify the new worker object is stored
		selWorker := device.workerIDsSeen["worker4"]
		if selWorker == nil {
			t.Fatal("Worker should exist in workerIDsSeen")
		}
		if selWorker.worker != worker4b {
			t.Error("Worker object should be the new one (worker4b)")
		}
	})
}

// TestBalancedSelector_DisabledDeviceNotSelectable tests that a disabled device's workers
// cannot be selected even after being made available again.
func TestBalancedSelector_DisabledDeviceNotSelectable(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Step 1: SetWorkerAvailable() a worker with deviceId "device1"
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Step 2: Disable "device1"
	selector.DisableDevice("device1")

	// Step 3: Check the worker is not available/selectable
	selectedWorker, _ := selector.GetAvailableWorker(5)
	if selectedWorker != nil {
		t.Error("Expected no worker to be selected from disabled device")
	}

	// Step 4: SetWorkerUnavailable() the worker
	selector.SetWorkerUnavailable(worker1)

	// Step 5: SetWorkerAvailable() the worker again
	selector.SetWorkerAvailable(worker1)

	// Verify worker is still not available
	selectedWorker, _ = selector.GetAvailableWorker(5)
	if selectedWorker != nil {
		t.Error("Expected no worker to be selected from disabled device")
	}
	selectedWorker, _ = selector.GetAvailableWorker(5)
	if selectedWorker != nil {
		t.Error("Expected no worker to be selected from disabled device after re-adding worker")
	}

	// Verify device is still disabled
	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("Device should exist")
	}
	if device.selectionEnabled {
		t.Error("Device should still be disabled")
	}

	// Bonus: Enable the device and verify worker can now be selected
	selector.EnableDevice("device1")
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Errorf("Expected worker to be selected after enabling device, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Error("Expected worker to be selected after enabling device")
	}
	if selectedWorker != nil && selectedWorker.ID() != "worker1" {
		t.Errorf("Expected worker1 to be selected, got %s", selectedWorker.ID())
	}
}

// TestBalancedSelector_DisabledDeviceWithDifferentWorkerObject tests that a disabled device's
// workers cannot be selected even when SetWorkerAvailable is called with a different worker object.
func TestBalancedSelector_DisabledDeviceWithDifferentWorkerObject(t *testing.T) {
	var cfg Config
	cfg.Init(Settings{})
	selector := NewBalancedSelector[*MockWorker](cfg)

	// Step 1: SetWorkerAvailable() a worker with deviceId "device1"
	worker1 := &MockWorker{id: "worker1", deviceID: "device1"}
	selector.SetWorkerAvailable(worker1)

	// Step 2: Disable "device1"
	selector.DisableDevice("device1")

	// Verify worker is not available
	selectedWorker, _ := selector.GetAvailableWorker(5)
	if selectedWorker != nil {
		t.Error("Expected no worker to be selected from disabled device")
	}

	// Step 4: Skip SetWorkerUnavailable and directly call SetWorkerAvailable with a different object
	// but same worker ID
	worker1Different := &MockWorker{id: "worker1", deviceID: "device1"} // Same ID, different object
	selector.SetWorkerAvailable(worker1Different)

	// Verify worker is still not available
	selectedWorker, _ = selector.GetAvailableWorker(5)
	if selectedWorker != nil {
		t.Error("Expected no worker to be selected from disabled device")
	}

	// Verify device is still disabled
	device := selector.devices["device1"]
	if device == nil {
		t.Fatal("Device should exist")
	}
	if device.selectionEnabled {
		t.Error("Device should still be disabled")
	}

	// Verify the worker object was updated to the new one
	selWorker := device.workerIDsSeen["worker1"]
	if selWorker == nil {
		t.Fatal("Worker should exist in workerIDsSeen")
	}
	if selWorker.worker != worker1Different {
		t.Error("Worker object should be updated to the new one")
	}

	// Bonus: Enable the device and verify worker can now be selected
	selector.EnableDevice("device1")
	selectedWorker, err := selector.GetAvailableWorker(5)
	if err != nil {
		t.Errorf("Expected worker to be selected after enabling device, got error: %v", err)
	}
	if selectedWorker == nil {
		t.Error("Expected worker to be selected after enabling device")
	}
	if selectedWorker != nil && selectedWorker.ID() != "worker1" {
		t.Errorf("Expected worker1 to be selected, got %s", selectedWorker.ID())
	}
	// Verify it's the new worker object that was selected
	if selectedWorker != worker1Different {
		t.Error("Expected the new worker object to be selected")
	}
}
