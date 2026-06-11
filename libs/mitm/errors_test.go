package mitm

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsErrDeviceCommandsUnavailable(t *testing.T) {
	if !IsErrDeviceCommandsUnavailable(errDeviceCommandsUnavailable) {
		t.Error("IsErrDeviceCommandsUnavailable should return true for errDeviceCommandsUnavailable")
	}

	wrapped := fmt.Errorf("wrap: %w", errDeviceCommandsUnavailable)
	if !IsErrDeviceCommandsUnavailable(wrapped) {
		t.Error("IsErrDeviceCommandsUnavailable should return true for wrapped errDeviceCommandsUnavailable")
	}

	other := errors.New("some other error")
	if IsErrDeviceCommandsUnavailable(other) {
		t.Error("IsErrDeviceCommandsUnavailable should return false for unrelated error")
	}

	if IsErrDeviceCommandsUnavailable(nil) {
		t.Error("IsErrDeviceCommandsUnavailable should return false for nil")
	}
}
