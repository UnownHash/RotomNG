package ws

import (
	"context"
	"net/http"

	"github.com/fasthttp/websocket"
)

// dialConfig holds the resolved dial options.
type dialConfig struct {
	subprotocols []string
	httpHeader   http.Header
	compression  bool
	bufferPool   BufferPool
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

	wsConn := NewConn(conn, WithBufferPoolOpt(cfg.bufferPool))
	return wsConn, resp, nil
}
