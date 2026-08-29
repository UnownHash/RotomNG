package ginutil

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// largeBody is comfortably over gzipMinLength and highly repetitive, standing in
// for the repeated JSON field names that dominate a real status reply.
var largeBody = strings.Repeat(`{"origin":"device","is_connected":true},`, 200)

func newGzipTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := NewEngineWithLogger(slog.New(slog.DiscardHandler))
	r.GET("/api/status", func(c *gin.Context) {
		c.String(http.StatusOK, largeBody)
	})
	r.GET("/api/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/api/device/abc/action/logcat", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/zip", []byte(largeBody))
	})
	r.GET("/assets/logo.png", func(c *gin.Context) {
		c.Data(http.StatusOK, "image/png", []byte(largeBody))
	})
	return r
}

func doGet(t *testing.T, r *gin.Engine, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGzipMiddlewareCompressesLargeResponses(t *testing.T) {
	r := newGzipTestEngine(t)

	w := doGet(t, r, "/api/status", map[string]string{"Accept-Encoding": "gzip"})

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}

	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("response is not valid gzip: %v", err)
	}
	defer reader.Close()

	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read gzip stream: %v", err)
	}
	if string(decoded) != largeBody {
		t.Error("decompressed body does not match the original")
	}
	if w.Body.Len() >= len(largeBody) {
		t.Errorf("compressed body (%d bytes) is not smaller than the original (%d bytes)",
			w.Body.Len(), len(largeBody))
	}
}

func TestGzipMiddlewareSkipsWhenNotAccepted(t *testing.T) {
	r := newGzipTestEngine(t)

	w := doGet(t, r, "/api/status", nil)

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty for a client that did not ask for gzip", got)
	}
	if w.Body.String() != largeBody {
		t.Error("body was altered for a client that did not ask for gzip")
	}
}

// A WebSocket upgrade must reach the handler untouched -- wrapping the
// ResponseWriter would break the hijack that the device and controller sockets
// depend on.
func TestGzipMiddlewareSkipsUpgradeRequests(t *testing.T) {
	r := newGzipTestEngine(t)

	w := doGet(t, r, "/api/status", map[string]string{
		"Accept-Encoding": "gzip",
		"Connection":      "Upgrade",
		"Upgrade":         "websocket",
	})

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty for an upgrade request", got)
	}
}

func TestGzipMiddlewareSkipsSmallResponses(t *testing.T) {
	r := newGzipTestEngine(t)

	w := doGet(t, r, "/api/ok", map[string]string{"Accept-Encoding": "gzip"})

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty for a body under %d bytes", got, gzipMinLength)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("small body was mangled: %q", w.Body.String())
	}
}

func TestGzipMiddlewareSkipsExcludedPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"logcat zip", "/api/device/abc/action/logcat"},
		{"png asset", "/assets/logo.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newGzipTestEngine(t)

			w := doGet(t, r, tt.path, map[string]string{"Accept-Encoding": "gzip"})

			if got := w.Header().Get("Content-Encoding"); got != "" {
				t.Errorf("Content-Encoding = %q, want empty for already-compressed content", got)
			}
			if w.Body.String() != largeBody {
				t.Error("body was altered for an excluded path")
			}
		})
	}
}
