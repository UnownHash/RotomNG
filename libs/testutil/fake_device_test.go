package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestFakeDevice_Options(t *testing.T) {
	// Test that NewFakeDevice with no options has sensible defaults
	fd := NewFakeDevice()
	if fd.DeviceID() == "" {
		t.Fatal("expected non-empty default deviceID")
	}
	if fd.Origin() != "test-origin" {
		t.Fatalf("expected default origin 'test-origin', got %q", fd.Origin())
	}

	// Test WithDeviceID
	fd2 := NewFakeDevice(WithDeviceID("custom-id"))
	if fd2.DeviceID() != "custom-id" {
		t.Fatalf("expected deviceID 'custom-id', got %q", fd2.DeviceID())
	}

	// Test WithOrigin
	fd3 := NewFakeDevice(WithOrigin("my-origin"))
	if fd3.Origin() != "my-origin" {
		t.Fatalf("expected origin 'my-origin', got %q", fd3.Origin())
	}

	// Test WithVersion
	fd4 := NewFakeDevice(WithVersion("2.0"))
	_ = fd4 // version is internal, just verify no panic

	// Test WithPublicIP
	fd5 := NewFakeDevice(WithPublicIP("10.0.0.1"))
	_ = fd5 // publicIP is internal, just verify no panic

	// Test WithDeviceAuthSecret
	fd6 := NewFakeDevice(WithDeviceAuthSecret("secret123"))
	_ = fd6 // authSecret is internal, just verify no panic

	// Test Connected() is false before Connect
	if fd.Connected() {
		t.Fatal("expected Connected() to be false before Connect()")
	}
}

func TestFakeDevice_Connect(t *testing.T) {
	ctx := context.Background()

	te, err := NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}

	if err := te.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer te.Stop()

	if err := te.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	// Connect a FakeDevice
	fd := NewFakeDevice(WithDeviceID("test-device-1"))
	if err := fd.Connect(ctx, te.DeviceAddr); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer fd.Close()

	if !fd.Connected() {
		t.Fatal("expected Connected() to be true after Connect()")
	}

	// Give the server a moment to register the device
	time.Sleep(200 * time.Millisecond)

	// Verify server-side registration via HTTP API
	resp, err := http.Get(fmt.Sprintf("http://%s/api/devices", te.HTTPAddr))
	if err != nil {
		t.Fatalf("HTTP GET /api/devices: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}

	// Parse the response to verify device is registered
	var devices []map[string]any
	if err := json.Unmarshal(body, &devices); err != nil {
		// Try as object with data field
		var wrapper map[string]any
		if err2 := json.Unmarshal(body, &wrapper); err2 != nil {
			t.Fatalf("failed to parse devices response: %v (body: %s)", err, string(body))
		}
		// Check if there's a data field that's an array
		if data, ok := wrapper["data"]; ok {
			if arr, ok := data.([]any); ok {
				if len(arr) == 0 {
					t.Fatal("expected at least 1 device registered, got 0")
				}
			}
		}
	} else if len(devices) == 0 {
		t.Fatal("expected at least 1 device registered, got 0")
	}

	// Close cleanly
	if err := fd.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if fd.Connected() {
		t.Fatal("expected Connected() to be false after Close()")
	}
}
