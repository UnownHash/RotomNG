package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/ws"
)

func TestDeviceHandlerSettings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		s       DeviceHandlerSettings
		wantErr bool
	}{
		{
			name: "valid",
			s:    DeviceHandlerSettings{PingInterval: 30 * time.Second, PongWait: 10 * time.Second},
		},
		{
			name:    "zero ping interval",
			s:       DeviceHandlerSettings{PingInterval: 0, PongWait: 10 * time.Second},
			wantErr: true,
		},
		{
			name:    "zero pong wait",
			s:       DeviceHandlerSettings{PingInterval: 30 * time.Second, PongWait: 0},
			wantErr: true,
		},
		{
			name:    "negative values",
			s:       DeviceHandlerSettings{PingInterval: -1, PongWait: -1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestDeviceHandlerConfig_InitAndGetSettings(t *testing.T) {
	cfg := DeviceHandlerConfig{}
	want := DeviceHandlerSettings{PingInterval: 25 * time.Second, PongWait: 9 * time.Second}
	if err := cfg.Init(want); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.GetSettings(); got != want {
		t.Errorf("GetSettings() = %+v, want %+v", got, want)
	}

	var bad DeviceHandlerConfig
	if err := bad.Init(DeviceHandlerSettings{}); err == nil {
		t.Error("Init with zero settings = nil, want error")
	}
}

// newPingTestDeviceHandler builds a DeviceHandler whose settings container is returned
// so tests can drive reloads via PutSettings.
func newPingTestDeviceHandler(t *testing.T, s DeviceHandlerSettings) (*DeviceHandler, *deviceHandlerSettingsContainer) {
	t.Helper()
	cfg := DeviceHandlerConfig{
		Logger:     logging.NewDiscardLogger(),
		BufferPool: bufferpool.New(8 * 1024),
	}
	if err := cfg.Init(s); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return NewDeviceHandler(context.Background(), cfg), cfg.deviceHandlerSettingsContainer
}

// dialWSConn returns a client *ws.Conn connected to an idle server that reads until close.
func dialWSConn(t *testing.T) *ws.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := ws.Accept(w, r)
		if err != nil {
			return
		}
		defer s.Close(ws.StatusNormalClosure, "")
		for {
			reader, err := s.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, resp, err := ws.Dial(context.Background(), url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	t.Cleanup(func() { c.Close(ws.StatusNormalClosure, "") })
	return c
}

// TestDeviceHandler_StartManagedPingLoop_RestartsOnSettingsChange verifies that a
// settings change delivered through the container restarts the ping loop with the
// new interval. The loop starts with a 1h interval (no ping), then a reload to 20ms
// must produce pings.
func TestDeviceHandler_StartManagedPingLoop_RestartsOnSettingsChange(t *testing.T) {
	handler, container := newPingTestDeviceHandler(t, DeviceHandlerSettings{
		PingInterval: time.Hour,
		PongWait:     time.Minute,
	})

	loopCtx, cancel := context.WithCancel(context.Background())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sConn, err := ws.Accept(w, r)
		if err != nil {
			return
		}
		defer sConn.Close(ws.StatusNormalClosure, "")
		handler.startManagedPingLoop(loopCtx, sConn)
	}))
	// Cancel the loop before tearing down the server so the handler returns.
	defer srv.Close()
	defer cancel()

	pings := make(chan struct{}, 8)
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	client.SetPingHandler(func(string) error {
		select {
		case pings <- struct{}{}:
		default:
		}
		return nil
	})
	go func() {
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// With a 1h interval, no ping should arrive. This window also ensures the loop has
	// subscribed to settings changes before we call PutSettings.
	select {
	case <-pings:
		t.Fatal("unexpected ping with 1h interval")
	case <-time.After(150 * time.Millisecond):
	}

	if err := container.PutSettings(DeviceHandlerSettings{
		PingInterval: 20 * time.Millisecond,
		PongWait:     100 * time.Millisecond,
	}); err != nil {
		t.Fatalf("PutSettings: %v", err)
	}

	select {
	case <-pings:
	case <-time.After(2 * time.Second):
		t.Fatal("no ping after settings change — loop did not restart")
	}
}

// TestDeviceHandler_StartManagedPingLoop_ReturnsOnContextCancel verifies the managed
// loop exits promptly when its context is cancelled. Run under -race this also exercises
// the StartPingLoop / WaitGroup interaction.
func TestDeviceHandler_StartManagedPingLoop_ReturnsOnContextCancel(t *testing.T) {
	wsConn := dialWSConn(t)
	handler, _ := newPingTestDeviceHandler(t, DeviceHandlerSettings{
		PingInterval: 50 * time.Millisecond,
		PongWait:     50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		handler.startManagedPingLoop(ctx, wsConn)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startManagedPingLoop did not return after context cancel")
	}
}
