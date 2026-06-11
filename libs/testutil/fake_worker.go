package testutil

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// RequestHandler is a callback for handling MitmRequests sent to a worker.
// It receives a request and returns a response.
type RequestHandler func(*protos.MitmRequest) *protos.MitmResponse

// WorkerOption is a functional option for configuring a FakeWorker.
type WorkerOption func(*workerConfig)

// workerConfig holds the configuration for a FakeWorker.
type workerConfig struct {
	workerID    string
	deviceID    string
	origin      string
	versionCode int32
	versionName string
	userAgent   string
	platform    protos.WelcomeMessage_Platform
	authSecret  string
	reqHandler  RequestHandler
}

// FakeWorker simulates a real MITM worker that connects to a RotomNG device
// listener via WebSocket at the root path "/". It sends a protobuf WelcomeMessage
// handshake and handles incoming MitmRequests in a background read loop.
type FakeWorker struct {
	cfg       workerConfig
	wsConn    *ws.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	connected atomic.Bool
	closeMu   sync.Mutex
}

// WithWorkerID sets the worker ID for the FakeWorker.
func WithWorkerID(id string) WorkerOption {
	return func(cfg *workerConfig) {
		cfg.workerID = id
	}
}

// WithWorkerDeviceID sets the device ID for the FakeWorker.
func WithWorkerDeviceID(id string) WorkerOption {
	return func(cfg *workerConfig) {
		cfg.deviceID = id
	}
}

// WithWorkerOrigin sets the origin for the FakeWorker.
func WithWorkerOrigin(origin string) WorkerOption {
	return func(cfg *workerConfig) {
		cfg.origin = origin
	}
}

// WithRequestHandler sets a custom request handler for the FakeWorker.
func WithRequestHandler(fn RequestHandler) WorkerOption {
	return func(cfg *workerConfig) {
		cfg.reqHandler = fn
	}
}

// WithPlatform sets the platform for the FakeWorker.
func WithPlatform(p protos.WelcomeMessage_Platform) WorkerOption {
	return func(cfg *workerConfig) {
		cfg.platform = p
	}
}

// WithWorkerAuthSecret sets the auth secret for the FakeWorker.
func WithWorkerAuthSecret(secret string) WorkerOption {
	return func(cfg *workerConfig) {
		cfg.authSecret = secret
	}
}

// NewFakeWorker creates a new FakeWorker with the given options applied over defaults.
func NewFakeWorker(opts ...WorkerOption) *FakeWorker {
	cfg := workerConfig{
		workerID:    uuid.New().String(),
		deviceID:    "",
		origin:      defaultTestOrigin,
		versionCode: 1,
		versionName: defaultTestVersion,
		userAgent:   "fake-worker",
		platform:    protos.WelcomeMessage_ANDROID,
		reqHandler:  defaultRequestHandler,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &FakeWorker{
		cfg: cfg,
	}
}

// Connect dials the RotomNG device listener at the given address on the root
// path "/", sends a protobuf WelcomeMessage handshake as a binary frame, and
// starts a background read loop for handling incoming MitmRequests.
func (fw *FakeWorker) Connect(ctx context.Context, addr string) error {
	url := "ws://" + addr + "/"

	var dialOpts []ws.DialOption
	if fw.cfg.authSecret != "" {
		header := http.Header{}
		header.Set("X-Rotom-Secret", fw.cfg.authSecret)
		dialOpts = append(dialOpts, ws.WithDialHTTPHeader(header))
	}

	wsConn, resp, err := ws.Dial(ctx, url, dialOpts...)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return err
	}

	// Build and send the WelcomeMessage handshake (synchronous binary frame)
	welcomeMsg := &protos.WelcomeMessage{
		WorkerId:    fw.cfg.workerID,
		Origin:      fw.cfg.origin,
		DeviceId:    fw.cfg.deviceID,
		VersionCode: fw.cfg.versionCode,
		VersionName: fw.cfg.versionName,
		Useragent:   fw.cfg.userAgent,
		Platform:    fw.cfg.platform,
	}

	data, err := proto.Marshal(welcomeMsg)
	if err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to marshal welcome message")
		return err
	}

	if err := wsConn.Write(ctx, ws.MessageBinary, data); err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to send welcome message")
		return err
	}

	fw.wsConn = wsConn
	fw.ctx, fw.cancel = context.WithCancel(ctx)
	fw.done = make(chan struct{})
	fw.connected.Store(true)

	go fw.readLoop()

	return nil
}

// Close disconnects the FakeWorker cleanly.
func (fw *FakeWorker) Close() error {
	fw.closeMu.Lock()
	defer fw.closeMu.Unlock()

	if !fw.connected.Load() {
		return nil
	}

	fw.cancel()
	fw.wsConn.Close(ws.StatusNormalClosure, "")
	<-fw.done
	fw.connected.Store(false)

	return nil
}

// WorkerID returns the worker ID.
func (fw *FakeWorker) WorkerID() string {
	return fw.cfg.workerID
}

// DeviceID returns the device ID.
func (fw *FakeWorker) DeviceID() string {
	return fw.cfg.deviceID
}

// Connected returns whether the FakeWorker is currently connected.
func (fw *FakeWorker) Connected() bool {
	return fw.connected.Load()
}

// readLoop reads incoming MitmRequests and dispatches them to the request handler.
func (fw *FakeWorker) readLoop() {
	defer close(fw.done)

	for {
		reader, err := fw.wsConn.Reader(fw.ctx)
		if err != nil {
			return
		}

		payload := reader.Bytes()
		reader.Done()

		var mitmReq protos.MitmRequest
		if err := proto.Unmarshal(payload, &mitmReq); err != nil {
			continue
		}

		resp := fw.cfg.reqHandler(&mitmReq)
		if resp == nil {
			continue
		}

		respData, err := proto.Marshal(resp)
		if err != nil {
			continue
		}

		_ = fw.wsConn.WriteAsync(fw.ctx, ws.MessageBinary, respData)
	}
}

// defaultRequestHandler returns a MitmResponse with SUCCESS status for any request.
func defaultRequestHandler(req *protos.MitmRequest) *protos.MitmResponse {
	return &protos.MitmResponse{
		Id:     req.Id,
		Status: protos.MitmResponse_SUCCESS,
	}
}

// NewWorker creates a new FakeWorker pre-configured with the parent device's
// deviceID, origin, and auth secret. Additional options can be provided to
// override these defaults.
func (fd *FakeDevice) NewWorker(opts ...WorkerOption) *FakeWorker {
	baseOpts := make([]WorkerOption, 0, 3+len(opts))
	baseOpts = append(baseOpts,
		WithWorkerDeviceID(fd.DeviceID()),
		WithWorkerOrigin(fd.Origin()),
		WithWorkerAuthSecret(fd.cfg.authSecret),
	)

	allOpts := slices.Concat(baseOpts, opts)
	return NewFakeWorker(allOpts...)
}
