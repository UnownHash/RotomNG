package ws

import (
	"context"
	"errors"
	"log/slog"

	"github.com/UnownHash/RotomNG/libs/logging"
)

var errWebsocketAlreadyClosed = errors.New("websocket is already closed")
var errWebsocketClosed = errors.New("websocket closed")

// errReadTimeout is returned by Reader when the ping timeout elapses without a
// pong or a data message being received (the connection is considered dead).
var errReadTimeout = errors.New("websocket read timeout: no pong or data received within timeout")

// errReadDataTimeout is returned by Reader when the data timeout elapses without
// a data message being received (the connection is considered dead).
var errReadDataTimeout = errors.New("websocket read data timeout: no data received within timeout")

// errInvalidPingSettings is returned by SetPingSettings for negative durations.
var errInvalidPingSettings = errors.New("invalid ping settings: durations must not be negative")

// errInvalidReadDataTimeout is returned by SetReadDataTimeout for a negative timeout.
var errInvalidReadDataTimeout = errors.New("invalid read data timeout: must not be negative")

// IsErrWebsocketAlreadyClosed reports whether err indicates the websocket was already closed.
func IsErrWebsocketAlreadyClosed(err error) bool {
	return errors.Is(err, errWebsocketAlreadyClosed)
}

// IsErrWebsocketClosed reports whether err indicates the websocket was closed.
func IsErrWebsocketClosed(err error) bool {
	return errors.Is(err, errWebsocketClosed)
}

// IsErrReadTimeout reports whether err indicates the ping read timeout elapsed
// (no pong or data received within the configured timeout).
func IsErrReadTimeout(err error) bool {
	return errors.Is(err, errReadTimeout)
}

// IsErrReadDataTimeout reports whether err indicates the data read timeout
// elapsed (no data message received within the configured data timeout).
func IsErrReadDataTimeout(err error) bool {
	return errors.Is(err, errReadDataTimeout)
}

// GetWebsocketCloseError extracts a CloseError from err, or returns nil if none is present.
func GetWebsocketCloseError(err error) *CloseError {
	var closeError *CloseError
	if !errors.As(err, &closeError) {
		return nil
	}
	return closeError
}

// LogWebsocketReadError logs a websocket read error using the context logger.
func LogWebsocketReadError(ctx context.Context, err error) {
	logger := logging.LoggerFromContext(ctx)
	if logger == nil {
		return
	}
	closeError := GetWebsocketCloseError(err)
	if closeError == nil {
		if !errors.Is(err, context.Canceled) {
			logger.LogAttrs(ctx, slog.LevelError, "failed to read from websocket", slog.String("error", err.Error()))
		}
		return
	}
	if closeError.Code == StatusGoingAway {
		logger.LogAttrs(ctx, slog.LevelInfo, "websocket found closed normally")
		return
	}
	logger.LogAttrs(ctx, slog.LevelWarn, "websocket found closed", slog.String("error", closeError.Error()))
}

// LogWebsocketWriteError logs a websocket write error for the given target using the context logger.
func LogWebsocketWriteError(ctx context.Context, targetName string, err error) {
	logger := logging.LoggerFromContext(ctx)
	if logger == nil {
		return
	}
	closeError := GetWebsocketCloseError(err)
	if closeError == nil {
		if !errors.Is(err, context.Canceled) {
			logger.LogAttrs(ctx, slog.LevelError, "failed to write to websocket", slog.String("target", targetName), slog.String("error", err.Error()))
		}
		return
	}
	if closeError.Code == StatusGoingAway {
		logger.LogAttrs(ctx, slog.LevelInfo, "websocket found closed normally", slog.String("target", targetName))
		return
	}
	logger.LogAttrs(ctx, slog.LevelWarn, "websocket found closed", slog.String("target", targetName), slog.String("error", closeError.Error()))
}
