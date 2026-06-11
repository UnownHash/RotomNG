package connections

import "github.com/UnownHash/RotomNG/libs/mitm"

type deviceStatsCollector = mitm.DeviceStatsCollector

// NoOpStatsCollector is a no-op implementation of the StatsCollector interface
// that does nothing when stats collection is disabled.
type NoOpStatsCollector struct {
	deviceStatsCollector
}

// NewNoOpStatsCollector creates a new no-op stats collector.
func NewNoOpStatsCollector() NoOpStatsCollector {
	return NoOpStatsCollector{
		deviceStatsCollector: mitm.NewNoOpStatsCollector(),
	}
}

// IncrDeviceRegistrationFails is a no-op.
func (n NoOpStatsCollector) IncrDeviceRegistrationFails() {}

// IncrDeviceRegistrations is a no-op.
func (n NoOpStatsCollector) IncrDeviceRegistrations(_ string) {}

// IncrDevicesConnected is a no-op.
func (n NoOpStatsCollector) IncrDevicesConnected(_ string) {}

// DecrDevicesConnected is a no-op.
func (n NoOpStatsCollector) DecrDevicesConnected(_ string) {}

// IncrDevicesTotal is a no-op.
func (n NoOpStatsCollector) IncrDevicesTotal(_ string) {}

// DecrDevicesTotal is a no-op.
func (n NoOpStatsCollector) DecrDevicesTotal(_ string, _ int) {}

// IncrWorkerRegistrationFails is a no-op.
func (n NoOpStatsCollector) IncrWorkerRegistrationFails() {}

// IncrWorkerRegistrations is a no-op.
func (n NoOpStatsCollector) IncrWorkerRegistrations(_ string) {}

// IncrWorkersInUse is a no-op.
func (n NoOpStatsCollector) IncrWorkersInUse(_ string) {}

// DecrWorkersInUse is a no-op.
func (n NoOpStatsCollector) DecrWorkersInUse(_ string) {}
