package mitm

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/ws"
)

func TestNewDevice(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")
	if d.ID() != "dev-1" {
		t.Errorf("Id() = %q, want %q", d.ID(), "dev-1")
	}
	if d.Origin() != "origin-1" {
		t.Errorf("Origin() = %q, want %q", d.Origin(), "origin-1")
	}
	if d.Version() != "" {
		t.Errorf("Version() = %q, want empty", d.Version())
	}
	if d.PublicIP() != "" {
		t.Errorf("PublicIP() = %q, want empty", d.PublicIP())
	}
	if !d.IsSelectionEnabled() {
		t.Error("selection should be enabled by default")
	}
	if d.IsConnected() {
		t.Error("should not be connected initially")
	}
}

func TestDevice_SettersAndGetters(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	d.SetOrigin("new-origin")
	if d.Origin() != "new-origin" {
		t.Errorf("Origin() = %q, want %q", d.Origin(), "new-origin")
	}

	d.SetVersion("2.0.0")
	if d.Version() != "2.0.0" {
		t.Errorf("Version() = %q, want %q", d.Version(), "2.0.0")
	}

	d.SetPublicIP("10.0.0.1")
	if d.PublicIP() != "10.0.0.1" {
		t.Errorf("PublicIP() = %q, want %q", d.PublicIP(), "10.0.0.1")
	}
}

func TestDevice_Conn(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	if d.Conn() != nil {
		t.Error("Conn() should be nil initially")
	}

	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev-1", "origin-1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	old := d.SwapConn(dc)
	if old != nil {
		t.Error("SwapConn should return nil on first call")
	}
	if d.Conn() != dc {
		t.Error("Conn() should return the set connection")
	}
	if !d.IsConnected() {
		t.Error("IsConnected() should be true after SwapConn")
	}
}

func TestDevice_CompareAndSwapConn(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc1 := NewDeviceConn("dev-1", "origin-1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)
	dc2 := NewDeviceConn("dev-1", "origin-1", "2.0", "1.2.3.4", wsConn, sc, lastMemory)

	// CAS from nil to dc1
	ok := d.CompareAndSwapConn(nil, dc1)
	if !ok {
		t.Error("CAS from nil should succeed")
	}

	// CAS from nil should fail (current is dc1)
	ok = d.CompareAndSwapConn(nil, dc2)
	if ok {
		t.Error("CAS from nil should fail when current is dc1")
	}

	// CAS from dc1 to dc2 should succeed
	ok = d.CompareAndSwapConn(dc1, dc2)
	if !ok {
		t.Error("CAS from dc1 to dc2 should succeed")
	}

	if d.Conn() != dc2 {
		t.Error("Conn() should be dc2 after CAS")
	}
}

func TestDevice_SelectionEnabled(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	if !d.IsSelectionEnabled() {
		t.Error("selection should be enabled by default")
	}

	// Disable selection - should return true (value changed)
	changed := d.SetSelectionEnabled(false)
	if !changed {
		t.Error("SetSelectionEnabled(false) should return true when changing from true")
	}
	if d.IsSelectionEnabled() {
		t.Error("selection should be disabled")
	}

	// Disable again - should return false (no change)
	changed = d.SetSelectionEnabled(false)
	if changed {
		t.Error("SetSelectionEnabled(false) should return false when already false")
	}

	// Enable - should return true (value changed)
	changed = d.SetSelectionEnabled(true)
	if !changed {
		t.Error("SetSelectionEnabled(true) should return true when changing from false")
	}
	if !d.IsSelectionEnabled() {
		t.Error("selection should be enabled")
	}
}

func TestDevice_LastMemory(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	if d.GetLastMemoryUsage() != nil {
		t.Error("GetLastMemoryUsage() should be nil initially")
	}

	mem := &DeviceMemory{Free: 100, Mitm: 200, Start: 300, Time: time.Now()}
	d.LastMemoryPointer().Store(mem)

	got := d.GetLastMemoryUsage()
	if got != mem {
		t.Error("GetLastMemoryUsage() should return stored memory")
	}
	if got.Free != 100 || got.Mitm != 200 || got.Start != 300 {
		t.Errorf("memory values mismatch: Free=%d, Mitm=%d, Start=%d", got.Free, got.Mitm, got.Start)
	}
}

func TestDevice_WebsocketStats(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	// No connection - session should be zero, total should be previousWSConnStats
	session, total := d.WebsocketStats()
	if session.MessagesReceived != 0 {
		t.Errorf("session.MessagesReceived = %d, want 0", session.MessagesReceived)
	}
	if total.MessagesReceived != 0 {
		t.Errorf("total.MessagesReceived = %d, want 0", total.MessagesReceived)
	}
}

func TestDevice_AccumulateStats(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")

	now := time.Now()
	wsConn := &mockDeviceWSConn{
		stats: ws.ConnStats{
			ConnectedAt:      now,
			MessagesReceived: 10,
			BytesReceived:    1000,
			MessagesSent:     5,
			BytesSent:        500,
		},
	}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev-1", "origin-1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)

	d.AccumulateStats(dc)

	_, total := d.WebsocketStats()
	if total.MessagesReceived != 10 {
		t.Errorf("total.MessagesReceived = %d, want 10", total.MessagesReceived)
	}
	if total.BytesSent != 500 {
		t.Errorf("total.BytesSent = %d, want 500", total.BytesSent)
	}
}

func TestDevice_AccumulateStats_Nil(_ *testing.T) {
	d := NewDevice("dev-1", "origin-1")
	// Should not panic with nil
	d.AccumulateStats(nil)
}

func TestDevice_Clone(t *testing.T) {
	d := NewDevice("dev-1", "origin-1")
	d.SetVersion("3.0")
	d.SetPublicIP("192.168.1.1")
	d.SetSelectionEnabled(false)

	wsConn := &mockDeviceWSConn{}
	lastMemory := &atomic.Pointer[DeviceMemory]{}
	sc := newMockDeviceStatsCollector()
	dc := NewDeviceConn("dev-1", "origin-1", "1.0", "1.2.3.4", wsConn, sc, lastMemory)
	d.SwapConn(dc)

	clone := d.Clone()

	if clone.ID() != d.ID() {
		t.Errorf("clone.ID() = %q, want %q", clone.ID(), d.ID())
	}
	if clone.Origin() != d.Origin() {
		t.Errorf("clone.Origin() = %q, want %q", clone.Origin(), d.Origin())
	}
	if clone.Version() != d.Version() {
		t.Errorf("clone.Version() = %q, want %q", clone.Version(), d.Version())
	}
	if clone.PublicIP() != d.PublicIP() {
		t.Errorf("clone.PublicIP() = %q, want %q", clone.PublicIP(), d.PublicIP())
	}
	if clone.IsSelectionEnabled() != d.IsSelectionEnabled() {
		t.Errorf("clone.IsSelectionEnabled() = %v, want %v", clone.IsSelectionEnabled(), d.IsSelectionEnabled())
	}
	if clone.Conn() != d.Conn() {
		t.Error("clone.Conn() should match original")
	}

	// Clone shares lastMemory pointer
	mem := &DeviceMemory{Free: 42}
	d.LastMemoryPointer().Store(mem)
	if clone.GetLastMemoryUsage() != mem {
		t.Error("clone should share lastMemory pointer with original")
	}
}
