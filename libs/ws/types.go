package ws

import (
	"bytes"

	"github.com/fasthttp/websocket"
)

// MessageType constants to match fasthttp websocket API.
type MessageType = int

// WebSocket message type constants.
const (
	MessageText   MessageType = websocket.TextMessage
	MessageBinary MessageType = websocket.BinaryMessage
)

// StatusCode constants to match coder websocket API.
type StatusCode = int

// WebSocket close status code constants.
const (
	StatusNormalClosure           StatusCode = websocket.CloseNormalClosure
	StatusGoingAway               StatusCode = websocket.CloseGoingAway
	StatusProtocolError           StatusCode = websocket.CloseProtocolError
	StatusUnsupportedData         StatusCode = websocket.CloseUnsupportedData
	StatusNoStatusRcvd            StatusCode = websocket.CloseNoStatusReceived
	StatusAbnormalClosure         StatusCode = websocket.CloseAbnormalClosure
	StatusInvalidFramePayloadData StatusCode = websocket.CloseInvalidFramePayloadData
	StatusPolicyViolation         StatusCode = websocket.ClosePolicyViolation
	StatusMessageTooBig           StatusCode = websocket.CloseMessageTooBig
	StatusMandatoryExtension      StatusCode = websocket.CloseMandatoryExtension
	StatusInternalServerError     StatusCode = websocket.CloseInternalServerErr
	StatusServiceRestart          StatusCode = websocket.CloseServiceRestart
	StatusTryAgainLater           StatusCode = websocket.CloseTryAgainLater
	StatusBadGateway              StatusCode = 1014 // Custom status code since gorilla doesn't have CloseBadGateway
)

// CloseError represents a WebSocket close frame with a status code and text.
type CloseError = websocket.CloseError

// BufferPool is an interface for getting and putting *bytes.Buffer instances.
type BufferPool interface {
	Get() *bytes.Buffer
	Put(buf *bytes.Buffer)
}
