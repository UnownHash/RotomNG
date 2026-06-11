package connections

import "testing"

func TestNoOpStatsCollector(_ *testing.T) {
	collector := NewNoOpStatsCollector()

	// Verify all methods execute without panic
	collector.IncrDeviceRegistrationFails()
	collector.IncrDeviceRegistrations("origin1")
	collector.IncrDevicesConnected("origin1")
	collector.DecrDevicesConnected("origin1")
	collector.IncrDevicesTotal("origin1")
	collector.DecrDevicesTotal("origin1", 5)
	collector.IncrWorkerRegistrationFails()
	collector.IncrWorkerRegistrations("origin1")
	collector.IncrWorkersInUse("origin1")
	collector.DecrWorkersInUse("origin1")

	// Verify the embedded device stats collector methods
	collector.SetDeviceMemoryFree("origin1", 100.0)
	collector.SetDeviceMemoryMITM("origin1", 50.0)
	collector.SetDeviceMemoryStart("origin1", 200.0)
	collector.IncrDeviceCommandExecuted("origin1", "restart")
	collector.IncrDeviceCommandSuccess("origin1", "restart")
	collector.IncrDeviceCommandError("origin1", "restart")
}

func TestNoOpStatsCollector_ImplementsInterface(_ *testing.T) {
	var _ StatsCollector = NewNoOpStatsCollector()
}
