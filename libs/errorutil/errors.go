// Package errorutil provides shared error types and WebSocket close codes.
package errorutil

import "errors"

var (
	errDeviceNotKnown            = errors.New("device not known")
	errDeviceNotConnected        = errors.New("device control not connected")
	errDeviceIsConnected         = errors.New("device control connected")
	errDeviceCommandsUnavailable = errors.New("device commands unavailable: device is stopping or starting")
	errDeviceHasWorkersConnected = errors.New("device has mitm workers connected")
	errControllerNotFound        = errors.New("controller not found")
)

// NewErrDeviceNotKnown returns an error indicating the device is not known.
func NewErrDeviceNotKnown() error {
	return errDeviceNotKnown
}

// NewErrDeviceNotConnected returns an error indicating the device control is not connected.
func NewErrDeviceNotConnected() error {
	return errDeviceNotConnected
}

// NewErrDeviceIsConnected returns an error indicating the device control is already connected.
func NewErrDeviceIsConnected() error {
	return errDeviceIsConnected
}

// NewErrDeviceCommandsUnavailable returns an error indicating device commands are unavailable.
func NewErrDeviceCommandsUnavailable() error {
	return errDeviceCommandsUnavailable
}

// NewErrDeviceHasWorkersConnected returns an error indicating the device has workers connected.
func NewErrDeviceHasWorkersConnected() error {
	return errDeviceHasWorkersConnected
}

// NewErrControllerNotFound returns an error indicating the controller was not found.
func NewErrControllerNotFound() error {
	return errControllerNotFound
}

// IsErrDeviceNotKnown reports whether the error is a device-not-known error.
func IsErrDeviceNotKnown(err error) bool {
	return errors.Is(err, errDeviceNotKnown)
}

// IsErrDeviceNotConnected reports whether the error is a device-not-connected error.
func IsErrDeviceNotConnected(err error) bool {
	return errors.Is(err, errDeviceNotConnected)
}

// IsErrDeviceIsConnected reports whether the error is a device-is-connected error.
func IsErrDeviceIsConnected(err error) bool {
	return errors.Is(err, errDeviceIsConnected)
}

// IsErrControllerNotFound reports whether the error is a controller-not-found error.
func IsErrControllerNotFound(err error) bool {
	return errors.Is(err, errControllerNotFound)
}

// IsErrDeviceHasWorkersConnected reports whether the error is a device-has-workers-connected error.
func IsErrDeviceHasWorkersConnected(err error) bool {
	return errors.Is(err, errDeviceHasWorkersConnected)
}
