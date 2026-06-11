package mitm

import "errors"

var (
	errDeviceCommandsUnavailable = errors.New("device unable to run commands")
)

// IsErrDeviceCommandsUnavailable checks if the error is a device-cannot-run-commands error.
func IsErrDeviceCommandsUnavailable(err error) bool {
	return errors.Is(err, errDeviceCommandsUnavailable)
}
