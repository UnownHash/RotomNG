package ws

import (
	"context"
	"errors"
	"log/slog"

	"github.com/UnownHash/RotomNG/libs/logging"
)

var errWebsocketAlreadyClosed = errors.New("websocket is already closed")
var errWebsocketClosed = errors.New("websocket closed")

// IsErrWebsocketAlreadyClosed reports whether err indicates the websocket was already closed.
func IsErrWebsocketAlreadyClosed(err error) bool {
	return errors.Is(err, errWebsocketAlreadyClosed)
}

// IsErrWebsocketClosed reports whether err indicates the websocket was closed.
func IsErrWebsocketClosed(err error) bool {
	return errors.Is(err, errWebsocketClosed)
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
