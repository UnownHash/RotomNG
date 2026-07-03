package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/fasthttp/websocket"
)

// dialConfig holds the resolved dial options.
type dialConfig struct {
	subprotocols    []string
	httpHeader      http.Header
	compression     bool
	bufferPool      BufferPool
	pingSettings    *PingSettings
	readDataTimeout *time.Duration
}

// DialOption configures the Dial call.
type DialOption func(*dialConfig) error

// WithDialSubprotocols sets the subprotocols for the WebSocket dial.
func WithDialSubprotocols(subprotocols ...string) DialOption {
	return func(c *dialConfig) error {
		c.subprotocols = subprotocols
		return nil
	}
}

// WithDialHTTPHeader sets the HTTP headers for the WebSocket dial.
func WithDialHTTPHeader(header http.Header) DialOption {
	return func(c *dialConfig) error {
		c.httpHeader = header
		return nil
	}
}

// WithDialCompression sets whether compression should be negotiated.
func WithDialCompression(enabled bool) DialOption {
	return func(c *dialConfig) error {
		c.compression = enabled
		return nil
	}
}

// WithDialBufferPoolOpt sets the buffer pool for the dialed connection.
func WithDialBufferPoolOpt(bufferPool BufferPool) DialOption {
	return func(c *dialConfig) error {
		c.bufferPool = bufferPool
		return nil
	}
}

// WithDialPingSettings sets the initial ping keep-alive settings on the dialed
// connection. Negative durations are rejected.
func WithDialPingSettings(s PingSettings) DialOption {
	return func(c *dialConfig) error {
		if !s.IsValid() {
			return errInvalidPingSettings
		}
		c.pingSettings = &s
		return nil
	}
}

// WithDialReadDataTimeout sets the initial data read timeout on the dialed
// connection. Zero disables it; a negative value is rejected.
func WithDialReadDataTimeout(d time.Duration) DialOption {
	return func(c *dialConfig) error {
		if d < 0 {
			return errInvalidReadDataTimeout
		}
		c.readDataTimeout = &d
		return nil
	}
}

// Dial dials a WebSocket connection and returns a Conn.
// Context is only used for the actual Dial, so one can provide a timeout.
// Later, one must call Close() to close the connection and clean up resources.
func Dial(ctx context.Context, u string, opts ...DialOption) (*Conn, *http.Response, error) {
	var cfg dialConfig
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, nil, err
		}
	}

	dialer := websocket.Dialer{
		Subprotocols:      cfg.subprotocols,
		EnableCompression: cfg.compression,
	}

	conn, resp, err := dialer.DialContext(ctx, u, cfg.httpHeader)
	if err != nil {
		return nil, resp, err
	}

	connOpts := []ConnOption{WithBufferPoolOpt(cfg.bufferPool)}
	if cfg.pingSettings != nil {
		connOpts = append(connOpts, WithPingSettings(*cfg.pingSettings))
	}
	if cfg.readDataTimeout != nil {
		connOpts = append(connOpts, WithReadDataTimeout(*cfg.readDataTimeout))
	}

	wsConn := NewConn(conn, connOpts...)
	return wsConn, resp, nil
}
