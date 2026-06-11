// Package mitm provides types and utilities for managing MITM proxy workers and device connections.
package mitm

import (
	"sync/atomic"

	"github.com/UnownHash/RotomNG/libs/ws"
)

// Device represents a connected MITM device with its connection state and metadata.
type Device struct {
	id       string
	origin   string
	version  string
	publicIP string

	lastMemory          *atomic.Pointer[DeviceMemory]
	previousWSConnStats ws.ConnStats

	deviceConn atomic.Pointer[DeviceConn]

	// selectionEnabled controls whether this device can be selected for new work
	selectionEnabled atomic.Bool
}

// NewDevice creates a new Device with the given ID and origin.
func NewDevice(id, origin string) *Device {
	device := &Device{
		id:         id,
		origin:     origin,
		lastMemory: &atomic.Pointer[DeviceMemory]{},
	}
	device.selectionEnabled.Store(true)
	return device
}

// AccumulateStats adds the given DeviceConn's websocket stats to the device's cumulative stats.
func (device *Device) AccumulateStats(deviceConn *DeviceConn) {
	if deviceConn != nil {
		device.previousWSConnStats.Add(deviceConn.GetConnStats())
	}
}

// Conn returns the current DeviceConn, or nil if not connected.
func (device *Device) Conn() *DeviceConn {
	return device.deviceConn.Load()
}

// SwapConn atomically swaps the current DeviceConn and returns the previous one.
func (device *Device) SwapConn(conn *DeviceConn) *DeviceConn {
	return device.deviceConn.Swap(conn)
}

// CompareAndSwapConn atomically swaps the DeviceConn if the current value matches old.
func (device *Device) CompareAndSwapConn(old, newConn *DeviceConn) bool {
	return device.deviceConn.CompareAndSwap(old, newConn)
}

// SetOrigin updates the device's origin.
func (device *Device) SetOrigin(origin string) {
	device.origin = origin
}

// SetVersion updates the device's version string.
func (device *Device) SetVersion(version string) {
	device.version = version
}

// SetPublicIP updates the device's public IP address.
func (device *Device) SetPublicIP(publicIP string) {
	device.publicIP = publicIP
}

// LastMemoryPointer returns the atomic pointer holding the last known device memory usage.
func (device *Device) LastMemoryPointer() *atomic.Pointer[DeviceMemory] {
	return device.lastMemory
}

// Clone returns a snapshot copy of the device.
func (device *Device) Clone() *Device {
	clone := &Device{
		id:                  device.id,
		origin:              device.origin,
		version:             device.version,
		publicIP:            device.publicIP,
		previousWSConnStats: device.previousWSConnStats,
		lastMemory:          device.lastMemory,
	}
	clone.selectionEnabled.Store(device.selectionEnabled.Load())
	clone.deviceConn.Store(device.deviceConn.Load())
	return clone
}

// IsConnected returns true if the device currently has an active connection.
func (device *Device) IsConnected() bool {
	return device.deviceConn.Load() != nil
}

// ID returns the device's unique identifier.
func (device *Device) ID() string {
	return device.id
}

// Origin returns the device's origin.
func (device *Device) Origin() string {
	return device.origin
}

// Version returns the device's version string.
func (device *Device) Version() string {
	return device.version
}

// PublicIP returns the device's public IP address.
func (device *Device) PublicIP() string {
	return device.publicIP
}

// WebsocketStats returns the current session and cumulative total websocket statistics.
func (device *Device) WebsocketStats() (session, total ws.ConnStats) {
	total = device.previousWSConnStats
	if conn := device.Conn(); conn != nil {
		session = conn.GetConnStats()
		total.Add(session)
	}
	return
}

// GetLastMemoryUsage returns the most recently recorded memory usage, or nil if none.
func (device *Device) GetLastMemoryUsage() *DeviceMemory {
	return device.lastMemory.Load()
}

// IsSelectionEnabled returns whether this device can be selected for new work.
func (device *Device) IsSelectionEnabled() bool {
	return device.selectionEnabled.Load()
}

// SetSelectionEnabled sets whether this device can be selected for new work
// Returns whether new value is different than old value.
func (device *Device) SetSelectionEnabled(enabled bool) bool {
	return device.selectionEnabled.CompareAndSwap(!enabled, enabled)
}
