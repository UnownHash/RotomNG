package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// CommandHandler is a callback for handling device commands sent by the server.
type CommandHandler func(mitm.DeviceCommandRequest) mitm.DeviceCommandReply

// DeviceOption is a functional option for configuring a FakeDevice.
type DeviceOption func(*deviceConfig)

// deviceConfig holds the configuration for a FakeDevice.
type deviceConfig struct {
	deviceID   string
	origin     string
	version    string
	publicIP   string
	authSecret string
	cmdHandler CommandHandler
}

// FakeDevice simulates a real MITM device that connects to a RotomNG device
// listener via WebSocket. It sends the DeviceControlInitMessage handshake and
// handles incoming device commands in a background read loop.
type FakeDevice struct {
	cfg       deviceConfig
	wsConn    *ws.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	connected atomic.Bool
	closeMu   sync.Mutex
}

// WithDeviceID sets the device ID for the FakeDevice.
func WithDeviceID(id string) DeviceOption {
	return func(cfg *deviceConfig) {
		cfg.deviceID = id
	}
}

// WithOrigin sets the origin for the FakeDevice.
func WithOrigin(origin string) DeviceOption {
	return func(cfg *deviceConfig) {
		cfg.origin = origin
	}
}

// WithVersion sets the version string for the FakeDevice.
func WithVersion(v string) DeviceOption {
	return func(cfg *deviceConfig) {
		cfg.version = v
	}
}

// WithPublicIP sets the public IP address for the FakeDevice.
func WithPublicIP(ip string) DeviceOption {
	return func(cfg *deviceConfig) {
		cfg.publicIP = ip
	}
}

// WithCommandHandler sets a custom command handler for the FakeDevice.
func WithCommandHandler(fn CommandHandler) DeviceOption {
	return func(cfg *deviceConfig) {
		cfg.cmdHandler = fn
	}
}

// WithDeviceAuthSecret sets the auth secret for the FakeDevice.
func WithDeviceAuthSecret(secret string) DeviceOption {
	return func(cfg *deviceConfig) {
		cfg.authSecret = secret
	}
}

// NewFakeDevice creates a new FakeDevice with the given options applied over defaults.
func NewFakeDevice(opts ...DeviceOption) *FakeDevice {
	cfg := deviceConfig{
		deviceID:   uuid.New().String(),
		origin:     defaultTestOrigin,
		version:    "1.0",
		publicIP:   "127.0.0.1",
		cmdHandler: defaultCommandHandler,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &FakeDevice{
		cfg: cfg,
	}
}

// Connect dials the RotomNG device listener at the given address, sends the
// DeviceControlInitMessage handshake, and starts a background read loop for
// handling incoming device commands.
func (fd *FakeDevice) Connect(ctx context.Context, addr string) error {
	url := "ws://" + addr + "/control"

	var dialOpts []ws.DialOption
	if fd.cfg.authSecret != "" {
		header := http.Header{}
		header.Set("X-Rotom-Secret", fd.cfg.authSecret)
		dialOpts = append(dialOpts, ws.WithDialHTTPHeader(header))
	}

	wsConn, resp, err := ws.Dial(ctx, url, dialOpts...)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return err
	}

	// Send the device control init message (synchronous write)
	initMsg := mitm.DeviceControlInitMessage{
		DeviceID: fd.cfg.deviceID,
		Version:  mitm.FlexibleString(fd.cfg.version),
		Origin:   fd.cfg.origin,
		PublicIP: fd.cfg.publicIP,
	}

	if err := wsConn.WriteJSON(ctx, initMsg); err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to send init message")
		return err
	}

	fd.wsConn = wsConn
	fd.ctx, fd.cancel = context.WithCancel(ctx)
	fd.done = make(chan struct{})
	fd.connected.Store(true)

	go fd.readLoop()

	return nil
}

// Close disconnects the FakeDevice cleanly.
func (fd *FakeDevice) Close() error {
	fd.closeMu.Lock()
	defer fd.closeMu.Unlock()

	if !fd.connected.Load() {
		return nil
	}

	fd.cancel()
	fd.wsConn.Close(ws.StatusNormalClosure, "")
	<-fd.done
	fd.connected.Store(false)

	return nil
}

// DeviceID returns the device ID.
func (fd *FakeDevice) DeviceID() string {
	return fd.cfg.deviceID
}

// Origin returns the origin.
func (fd *FakeDevice) Origin() string {
	return fd.cfg.origin
}

// Connected returns whether the FakeDevice is currently connected.
func (fd *FakeDevice) Connected() bool {
	return fd.connected.Load()
}

// readLoop reads incoming device commands and dispatches them to the command handler.
func (fd *FakeDevice) readLoop() {
	defer close(fd.done)

	for {
		reader, err := fd.wsConn.Reader(fd.ctx)
		if err != nil {
			return
		}

		data := reader.Bytes()
		reader.Done()

		var cmd mitm.DeviceCommandRequest
		if err := json.Unmarshal(data, &cmd); err != nil {
			continue
		}

		reply := fd.cfg.cmdHandler(cmd)
		_ = fd.wsConn.WriteJSONAsync(fd.ctx, reply)
	}
}

// defaultCommandHandler provides default responses for standard device commands.
func defaultCommandHandler(cmd mitm.DeviceCommandRequest) mitm.DeviceCommandReply {
	switch cmd.Method {
	case "getMemoryUsage":
		body, _ := json.Marshal(mitm.DeviceMemory{ //nolint:errchkjson
			Free:  4000000,
			Mitm:  500000,
			Start: 8000000,
		})
		return mitm.DeviceCommandReply{
			ID:     cmd.ID,
			Status: 200,
			Body:   body,
		}
	case "getScreenSize":
		body, _ := json.Marshal(struct { //nolint:errchkjson
			Width  int `json:"width"`
			Height int `json:"height"`
		}{
			Width:  1080,
			Height: 1920,
		})
		return mitm.DeviceCommandReply{
			ID:     cmd.ID,
			Status: 200,
			Body:   body,
		}
	default:
		return mitm.DeviceCommandReply{
			ID:     cmd.ID,
			Status: 200,
			Body:   json.RawMessage("{}"),
		}
	}
}
