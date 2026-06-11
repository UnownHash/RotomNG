package handlers

var _ ControllerStatsCollector = NoOpStatsCollector{}
var _ DeviceStatsCollector = NoOpStatsCollector{}

// NoOpStatsCollector is a stats collector that does nothing.
type NoOpStatsCollector struct{}

// NewNoOpStatsCollector creates a new no-op stats collector.
func NewNoOpStatsCollector() NoOpStatsCollector {
	return NoOpStatsCollector{}
}

// IncrDeviceControlAccepts is a no-op.
func (n NoOpStatsCollector) IncrDeviceControlAccepts() {}

// IncrDeviceControlAcceptFails is a no-op.
func (n NoOpStatsCollector) IncrDeviceControlAcceptFails() {}

// IncrControllerAccepts is a no-op.
func (n NoOpStatsCollector) IncrControllerAccepts() {}

// IncrControllerAcceptFails is a no-op.
func (n NoOpStatsCollector) IncrControllerAcceptFails() {}

// IncrControllerConnections is a no-op.
func (n NoOpStatsCollector) IncrControllerConnections(string) {}

// DecrControllerConnections is a no-op.
func (n NoOpStatsCollector) DecrControllerConnections(string) {}
