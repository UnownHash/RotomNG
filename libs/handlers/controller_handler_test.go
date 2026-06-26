package handlers

import (
	"testing"
	"time"
)

func TestControllerHandlerSettings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		s       ControllerHandlerSettings
		wantErr bool
	}{
		{
			name: "valid",
			s:    ControllerHandlerSettings{PingInterval: 30 * time.Second, PongWait: 10 * time.Second, RegistrationTimeout: 60 * time.Second},
		},
		{
			name:    "zero ping interval",
			s:       ControllerHandlerSettings{PingInterval: 0, PongWait: 10 * time.Second, RegistrationTimeout: 60 * time.Second},
			wantErr: true,
		},
		{
			name:    "negative ping interval",
			s:       ControllerHandlerSettings{PingInterval: -1, PongWait: 10 * time.Second, RegistrationTimeout: 60 * time.Second},
			wantErr: true,
		},
		{
			name:    "zero pong wait",
			s:       ControllerHandlerSettings{PingInterval: 30 * time.Second, PongWait: 0, RegistrationTimeout: 60 * time.Second},
			wantErr: true,
		},
		{
			name:    "negative pong wait",
			s:       ControllerHandlerSettings{PingInterval: 30 * time.Second, PongWait: -1, RegistrationTimeout: 60 * time.Second},
			wantErr: true,
		},
		{
			name:    "zero registration timeout",
			s:       ControllerHandlerSettings{PingInterval: 30 * time.Second, PongWait: 10 * time.Second, RegistrationTimeout: 0},
			wantErr: true,
		},
		{
			name:    "negative registration timeout",
			s:       ControllerHandlerSettings{PingInterval: 30 * time.Second, PongWait: 10 * time.Second, RegistrationTimeout: -1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// Note: the ControllerHandlerConfig settings-container plumbing (Init / GetSettings /
// PutSettings / Notify) wraps the same settings.Container used by DeviceHandlerConfig,
// whose behavior is covered in device_handler_test.go without the generic type-argument
// boilerplate (the Controller constraint interface can't be used as a type argument).
