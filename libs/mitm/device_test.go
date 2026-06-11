package mitm

import (
	"encoding/json"
	"testing"
)

func TestDeviceControlInitMessage_Marshal(t *testing.T) {
	tests := []struct {
		name    string
		msg     DeviceControlInitMessage
		wantErr bool
	}{
		{
			name: "marshal with string version",
			msg: DeviceControlInitMessage{
				DeviceID: "test-device-1",
				Version:  "1.2.3",
				Origin:   "test-origin",
				PublicIP: "192.168.1.1",
			},
			wantErr: false,
		},
		{
			name: "marshal with numeric-like string version",
			msg: DeviceControlInitMessage{
				DeviceID: "test-device-2",
				Version:  "123",
				Origin:   "test-origin",
				PublicIP: "192.168.1.2",
			},
			wantErr: false,
		},
		{
			name: "marshal with empty version",
			msg: DeviceControlInitMessage{
				DeviceID: "test-device-3",
				Version:  "",
				Origin:   "test-origin",
				PublicIP: "192.168.1.3",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			// Verify that the marshalled JSON contains version as a string
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("Failed to unmarshal to raw map: %v", err)
			}

			version, ok := raw["version"]
			if !ok {
				t.Error("version field not found in marshalled JSON")
				return
			}

			// Verify version is a string in the JSON
			versionStr, ok := version.(string)
			if !ok {
				t.Errorf("version in JSON is not a string, got type %T", version)
				return
			}

			if versionStr != string(tt.msg.Version) {
				t.Errorf("version mismatch: got %q, want %q", versionStr, tt.msg.Version)
			}
		})
	}
}

func TestDeviceControlInitMessage_UnmarshalVersionAsString(t *testing.T) {
	tests := []struct {
		name        string
		jsonInput   string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "unmarshal version as string",
			jsonInput:   `{"deviceId":"test-1","version":"1.2.3","origin":"test","publicIp":"192.168.1.1"}`,
			wantVersion: "1.2.3",
			wantErr:     false,
		},
		{
			name:        "unmarshal version as numeric string",
			jsonInput:   `{"deviceId":"test-2","version":"456","origin":"test","publicIp":"192.168.1.2"}`,
			wantVersion: "456",
			wantErr:     false,
		},
		{
			name:        "unmarshal version as empty string",
			jsonInput:   `{"deviceId":"test-3","version":"","origin":"test","publicIp":"192.168.1.3"}`,
			wantVersion: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg DeviceControlInitMessage
			err := json.Unmarshal([]byte(tt.jsonInput), &msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if string(msg.Version) != tt.wantVersion {
				t.Errorf("Version = %q, want %q", msg.Version, tt.wantVersion)
			}
		})
	}
}

func TestDeviceControlInitMessage_UnmarshalVersionAsInteger(t *testing.T) {
	tests := []struct {
		name        string
		jsonInput   string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "unmarshal version as integer",
			jsonInput:   `{"deviceId":"test-1","version":123,"origin":"test","publicIp":"192.168.1.1"}`,
			wantVersion: "123",
			wantErr:     false,
		},
		{
			name:        "unmarshal version as zero",
			jsonInput:   `{"deviceId":"test-2","version":0,"origin":"test","publicIp":"192.168.1.2"}`,
			wantVersion: "0",
			wantErr:     false,
		},
		{
			name:        "unmarshal version as negative integer",
			jsonInput:   `{"deviceId":"test-3","version":-42,"origin":"test","publicIp":"192.168.1.3"}`,
			wantVersion: "-42",
			wantErr:     false,
		},
		{
			name:        "unmarshal version as large integer",
			jsonInput:   `{"deviceId":"test-4","version":9999999999,"origin":"test","publicIp":"192.168.1.4"}`,
			wantVersion: "9999999999",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg DeviceControlInitMessage
			err := json.Unmarshal([]byte(tt.jsonInput), &msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if string(msg.Version) != tt.wantVersion {
				t.Errorf("Version = %q, want %q", msg.Version, tt.wantVersion)
			}
		})
	}
}

func TestDeviceControlInitMessage_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  DeviceControlInitMessage
	}{
		{
			name: "round trip with version string",
			msg: DeviceControlInitMessage{
				DeviceID: "device-1",
				Version:  "2.5.1",
				Origin:   "origin-1",
				PublicIP: "10.0.0.1",
			},
		},
		{
			name: "round trip with numeric version string",
			msg: DeviceControlInitMessage{
				DeviceID: "device-2",
				Version:  "789",
				Origin:   "origin-2",
				PublicIP: "10.0.0.2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal
			var decoded DeviceControlInitMessage
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// Compare
			if decoded.DeviceID != tt.msg.DeviceID {
				t.Errorf("DeviceId = %q, want %q", decoded.DeviceID, tt.msg.DeviceID)
			}
			if decoded.Version != tt.msg.Version {
				t.Errorf("Version = %q, want %q", decoded.Version, tt.msg.Version)
			}
			if decoded.Origin != tt.msg.Origin {
				t.Errorf("Origin = %q, want %q", decoded.Origin, tt.msg.Origin)
			}
			if decoded.PublicIP != tt.msg.PublicIP {
				t.Errorf("PublicIp = %q, want %q", decoded.PublicIP, tt.msg.PublicIP)
			}
		})
	}
}
