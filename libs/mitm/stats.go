package mitm

import "time"

// NoOpStatsCollector is a no-op implementation of both the WorkerStatsCollector
// and DeviceStatsCollector interfaces that does nothing when stats collection
// is disabled.
type NoOpStatsCollector struct{}

// NewNoOpStatsCollector creates a new no-op stats collector.
func NewNoOpStatsCollector() NoOpStatsCollector {
	return NoOpStatsCollector{}
}

// SetDeviceMemoryFree is a no-op implementation.
func (n NoOpStatsCollector) SetDeviceMemoryFree(_ string, _ float64) {}

// SetDeviceMemoryMITM is a no-op implementation.
func (n NoOpStatsCollector) SetDeviceMemoryMITM(_ string, _ float64) {}

// SetDeviceMemoryStart is a no-op implementation.
func (n NoOpStatsCollector) SetDeviceMemoryStart(_ string, _ float64) {}

// IncrDeviceCommandExecuted is a no-op implementation.
func (n NoOpStatsCollector) IncrDeviceCommandExecuted(_ string, _ string) {}

// IncrDeviceCommandSuccess is a no-op implementation.
func (n NoOpStatsCollector) IncrDeviceCommandSuccess(_ string, _ string) {}

// IncrDeviceCommandError is a no-op implementation.
func (n NoOpStatsCollector) IncrDeviceCommandError(_ string, _ string) {}

// IncrWorkerRequests is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkerRequests(_ string) {}

// IncrWorkerDroppedResponses is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkerDroppedResponses() {}

// IncrWorkerResponses is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkerResponses(_ time.Duration, _ string, _ string, _ string) {
}

// IncrRPCRequests is a no-op implementation.
func (n NoOpStatsCollector) IncrRPCRequests(_ time.Duration) {}

// IncrWorkerRegistrationFails is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkerRegistrationFails() {}

// IncrWorkerRegistrations is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkerRegistrations(_ string) {}

// IncrWorkersConnected is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkersConnected(_ string) {}

// DecrWorkersConnected is a no-op implementation.
func (n NoOpStatsCollector) DecrWorkersConnected(_ string) {}

// IncrWorkersInUse is a no-op implementation.
func (n NoOpStatsCollector) IncrWorkersInUse(_ string) {}

// DecrWorkersInUse is a no-op implementation.
func (n NoOpStatsCollector) DecrWorkersInUse(_ string) {}
