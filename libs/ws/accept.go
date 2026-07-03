// Package ws provides WebSocket connection utilities including reading, writing, and lifecycle management.
package ws

import (
	"net/http"
	"time"

	"github.com/fasthttp/websocket"
)

// acceptConfig holds the resolved accept options.
type acceptConfig struct {
	subprotocols    []string
	compression     bool
	bufferPool      BufferPool
	pingSettings    *PingSettings
	readDataTimeout *time.Duration
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

// WithAcceptPingSettings sets the initial ping keep-alive settings on the
// accepted connection. Negative durations are rejected.
func WithAcceptPingSettings(s PingSettings) AcceptOption {
	return func(c *acceptConfig) error {
		if !s.IsValid() {
			return errInvalidPingSettings
		}
		c.pingSettings = &s
		return nil
	}
}

// WithAcceptReadDataTimeout sets the initial data read timeout on the accepted
// connection. Zero disables it; a negative value is rejected.
func WithAcceptReadDataTimeout(d time.Duration) AcceptOption {
	return func(c *acceptConfig) error {
		if d < 0 {
			return errInvalidReadDataTimeout
		}
		c.readDataTimeout = &d
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

	connOpts := []ConnOption{WithBufferPoolOpt(cfg.bufferPool)}
	if cfg.pingSettings != nil {
		connOpts = append(connOpts, WithPingSettings(*cfg.pingSettings))
	}
	if cfg.readDataTimeout != nil {
		connOpts = append(connOpts, WithReadDataTimeout(*cfg.readDataTimeout))
	}

	wsConn := NewConn(conn, connOpts...)
	return wsConn, nil
}
