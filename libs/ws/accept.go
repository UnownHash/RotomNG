// Package ws provides WebSocket connection utilities including reading, writing, and lifecycle management.
package ws

import (
	"net/http"

	"github.com/fasthttp/websocket"
)

// acceptConfig holds the resolved accept options.
type acceptConfig struct {
	subprotocols []string
	compression  bool
	bufferPool   BufferPool
}

// AcceptOption configures the Accept call.
type AcceptOption func(*acceptConfig) error

// WithAcceptSubprotocols sets the subprotocols for the WebSocket handshake.
func WithAcceptSubprotocols(subprotocols ...string) AcceptOption {
	return func(c *acceptConfig) error {
		c.subprotocols = subprotocols
		return nil
	}
}

// WithAcceptCompressionThreshold sets whether compression should be negotiated.
func WithAcceptCompressionThreshold(enabled bool) AcceptOption {
	return func(c *acceptConfig) error {
		c.compression = enabled
		return nil
	}
}

// WithAcceptBufferPoolOpt sets the buffer pool for the accepted connection.
func WithAcceptBufferPoolOpt(bufferPool BufferPool) AcceptOption {
	return func(c *acceptConfig) error {
		c.bufferPool = bufferPool
		return nil
	}
}

// Accept accepts a WebSocket handshake from a client and returns a Conn.
func Accept(w http.ResponseWriter, r *http.Request, opts ...AcceptOption) (*Conn, error) {
	var cfg acceptConfig
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			// TODO: Implement origin checking based on cfg.originPatterns
			return true
		},
		Subprotocols:      cfg.subprotocols,
		EnableCompression: cfg.compression,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}

	wsConn := NewConn(conn, WithBufferPoolOpt(cfg.bufferPool))
	return wsConn, nil
}
