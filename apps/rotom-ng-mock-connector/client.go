package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/UnownHash/RotomNG/libs/ws"
)

const dialTimeout = 10 * time.Second

// dialWS opens a WebSocket connection to endpoint+path, attaching the
// X-Rotom-Secret header and an identifying User-Agent when provided.
func dialWS(ctx context.Context, endpoint, path, secret, userAgent string) (*ws.Conn, error) {
	url := strings.TrimRight(endpoint, "/") + path

	header := http.Header{}
	if secret != "" {
		header.Set("X-Rotom-Secret", secret)
	}
	if userAgent != "" {
		header.Set("User-Agent", userAgent)
	}

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	conn, resp, err := ws.Dial(dialCtx, url, ws.WithDialHTTPHeader(header))
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial %s: %w (http %s)", url, err, resp.Status)
		}
		return nil, fmt.Errorf("dial %s: %w", url, err)
	}
	return conn, nil
}
