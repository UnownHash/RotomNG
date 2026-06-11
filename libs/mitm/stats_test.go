package mitm

import (
	"testing"
	"time"
)

func TestNoOpStatsCollector_Implements_Interfaces(_ *testing.T) {
	var _ DeviceStatsCollector = NoOpStatsCollector{}
	var _ WorkerStatsCollector = NoOpStatsCollector{}
}

func TestNoOpStatsCollector_DoesNotPanic(_ *testing.T) {
	n := NewNoOpStatsCollector()

	// Device stats - should all be no-ops
	n.SetDeviceMemoryFree("origin", 100.0)
	n.SetDeviceMemoryMITM("origin", 200.0)
	n.SetDeviceMemoryStart("origin", 300.0)
	n.IncrDeviceCommandExecuted("origin", "cmd")
	n.IncrDeviceCommandSuccess("origin", "cmd")
	n.IncrDeviceCommandError("origin", "cmd")

	// Worker stats - should all be no-ops
	n.IncrWorkerRequests("GET")
	n.IncrWorkerDroppedResponses()
	n.IncrWorkerResponses(time.Second, "GET", "200", "")
	n.IncrRPCRequests(time.Second)
	n.IncrWorkerRegistrationFails()
	n.IncrWorkerRegistrations("origin")
	n.IncrWorkersConnected("origin")
	n.DecrWorkersConnected("origin")
	n.IncrWorkersInUse("origin")
	n.DecrWorkersInUse("origin")
}

func TestNewNoOpStatsCollector(t *testing.T) {
	n := NewNoOpStatsCollector()
	if n != (NoOpStatsCollector{}) {
		t.Error("NewNoOpStatsCollector should return zero value")
	}
}
