package testutil

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// ProtocolVersion represents the controller protocol version.
type ProtocolVersion int

const (
	// V1 is the legacy controller protocol (connects to /, sends LoginRequest directly).
	V1 ProtocolVersion = 1
	// V2 is the current controller protocol (connects to /controller, sends RegisterControllerRequest + LoginRequest).
	V2 ProtocolVersion = 2
)

// ResponseHandler is a callback for handling MitmResponses received by a controller.
type ResponseHandler func(*protos.MitmResponse)

// ControllerOption is a functional option for configuring a FakeController.
type ControllerOption func(*controllerConfig)

// controllerConfig holds the configuration for a FakeController.
type controllerConfig struct {
	controllerID    string
	weight          int
	protocolVersion ProtocolVersion
	userAgent       string
	authSecret      string
	respHandler     ResponseHandler
}

// FakeController simulates a real controller that connects to a RotomNG controller
// listener via WebSocket with V1 or V2 protocol support. It handles the registration
// handshake, sends MitmRequests, and reads MitmResponses in a background read loop.
type FakeController struct {
	cfg        controllerConfig
	wsConn     *ws.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	connected  atomic.Bool
	closeMu    sync.Mutex
	pendingMu  sync.Mutex
	pending    map[uint32]chan *protos.MitmResponse
	lastResp   atomic.Pointer[protos.MitmResponse]
	reqCounter atomic.Uint32
}

// WithControllerID sets the controller ID for the FakeController.
func WithControllerID(id string) ControllerOption {
	return func(cfg *controllerConfig) {
		cfg.controllerID = id
	}
}

// WithWeight sets the weight for the FakeController.
func WithWeight(w int) ControllerOption {
	return func(cfg *controllerConfig) {
		cfg.weight = w
	}
}

// WithProtocolVersion sets the protocol version for the FakeController.
func WithProtocolVersion(v ProtocolVersion) ControllerOption {
	return func(cfg *controllerConfig) {
		cfg.protocolVersion = v
	}
}

// WithUserAgent sets the user agent for the FakeController.
func WithUserAgent(ua string) ControllerOption {
	return func(cfg *controllerConfig) {
		cfg.userAgent = ua
	}
}

// WithAuthSecret sets the auth secret for the FakeController.
func WithAuthSecret(secret string) ControllerOption {
	return func(cfg *controllerConfig) {
		cfg.authSecret = secret
	}
}

// WithResponseHandler sets a custom response handler for the FakeController.
func WithResponseHandler(fn ResponseHandler) ControllerOption {
	return func(cfg *controllerConfig) {
		cfg.respHandler = fn
	}
}

// NewFakeController creates a new FakeController with the given options applied over defaults.
func NewFakeController(opts ...ControllerOption) *FakeController {
	cfg := controllerConfig{
		controllerID:    uuid.New().String(),
		weight:          0,
		protocolVersion: V2,
		userAgent:       "fake-controller",
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &FakeController{
		cfg:     cfg,
		pending: make(map[uint32]chan *protos.MitmResponse),
	}
}

// Connect dials the RotomNG controller listener at the given address using the
// configured protocol version (V1 or V2).
func (fc *FakeController) Connect(ctx context.Context, addr string) error {
	if fc.cfg.protocolVersion == V1 {
		return fc.connectV1(ctx, addr)
	}
	return fc.connectV2(ctx, addr)
}

// Close disconnects the FakeController cleanly.
func (fc *FakeController) Close() error {
	fc.closeMu.Lock()
	defer fc.closeMu.Unlock()

	if !fc.connected.Load() {
		return nil
	}

	fc.cancel()
	_ = fc.wsConn.Close(ws.StatusNormalClosure, "")
	<-fc.done
	fc.connected.Store(false)

	return nil
}

// SendRequest sends a MitmRequest and returns the matching MitmResponse synchronously.
// If req.Id is 0, an auto-incrementing ID is assigned.
func (fc *FakeController) SendRequest(req *protos.MitmRequest) (*protos.MitmResponse, error) {
	if req.Id == 0 {
		req.Id = fc.reqCounter.Add(1)
	}

	ch := make(chan *protos.MitmResponse, 1)

	fc.pendingMu.Lock()
	fc.pending[req.Id] = ch
	fc.pendingMu.Unlock()

	defer func() {
		fc.pendingMu.Lock()
		delete(fc.pending, req.Id)
		fc.pendingMu.Unlock()
	}()

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	if err := fc.wsConn.Write(fc.ctx, ws.MessageBinary, data); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-fc.ctx.Done():
		return nil, fc.ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("SendRequest timed out waiting for response to request %d", req.Id)
	}
}

// ControllerID returns the controller ID.
func (fc *FakeController) ControllerID() string {
	return fc.cfg.controllerID
}

// Connected returns whether the FakeController is currently connected.
func (fc *FakeController) Connected() bool {
	return fc.connected.Load()
}

// LastResponse returns the most recently received MitmResponse.
func (fc *FakeController) LastResponse() *protos.MitmResponse {
	return fc.lastResp.Load()
}

// connectV2 connects using the V2 protocol: sends RegisterControllerRequest,
// reads RegisterControllerResponse, then sends LoginRequest.
func (fc *FakeController) connectV2(ctx context.Context, addr string) error {
	url := "ws://" + addr + "/controller"

	var dialOpts []ws.DialOption
	if fc.cfg.authSecret != "" {
		header := http.Header{}
		header.Set("X-Rotom-Secret", fc.cfg.authSecret)
		dialOpts = append(dialOpts, ws.WithDialHTTPHeader(header))
	}

	wsConn, resp, err := ws.Dial(ctx, url, dialOpts...)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return err
	}

	// Step 1: Send RegisterControllerRequest
	regReq := &protos.RegisterControllerRequest{
		Id:                fc.cfg.controllerID,
		ProtoMajorVersion: 2,
		ProtoMinorVersion: 0,
		Weight:            int32(fc.cfg.weight),
	}

	data, err := proto.Marshal(regReq)
	if err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to marshal registration request")
		return err
	}

	if err := wsConn.Write(ctx, ws.MessageBinary, data); err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to send registration request")
		return err
	}

	// Step 2: Read RegisterControllerResponse
	reader, err := wsConn.Reader(ctx)
	if err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to read registration response")
		return err
	}

	var regResp protos.RegisterControllerResponse
	if err := proto.Unmarshal(reader.Bytes(), &regResp); err != nil {
		reader.Done()
		wsConn.Close(ws.StatusProtocolError, "failed to unmarshal registration response")
		return err
	}
	reader.Done()

	if regResp.Status != protos.RegisterControllerResponse_SUCCESS {
		wsConn.Close(ws.StatusNormalClosure, "registration failed")
		return fmt.Errorf("registration failed: %s (%s)", regResp.Status, regResp.StatusReason)
	}

	// Step 3: Send LoginRequest
	loginReq := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: fc.cfg.controllerID,
				Username: defaultTestUser,
				Source:   protos.MitmRequest_LoginRequest_PTC,
			},
		},
	}

	data, err = proto.Marshal(loginReq)
	if err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to marshal login request")
		return err
	}

	if err := wsConn.Write(ctx, ws.MessageBinary, data); err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to send login request")
		return err
	}

	fc.wsConn = wsConn
	fc.ctx, fc.cancel = context.WithCancel(ctx)
	fc.done = make(chan struct{})
	fc.connected.Store(true)

	go fc.readLoop()

	return nil
}

// connectV1 connects using the V1 protocol: connects to / with optional weight
// query param and sends LoginRequest directly.
func (fc *FakeController) connectV1(ctx context.Context, addr string) error {
	url := "ws://" + addr + "/"
	if fc.cfg.weight > 0 {
		url += "?weight=" + strconv.Itoa(fc.cfg.weight)
	}

	var dialOpts []ws.DialOption
	if fc.cfg.authSecret != "" {
		header := http.Header{}
		header.Set("X-Rotom-Secret", fc.cfg.authSecret)
		dialOpts = append(dialOpts, ws.WithDialHTTPHeader(header))
	}

	wsConn, resp, err := ws.Dial(ctx, url, dialOpts...)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return err
	}

	// Send LoginRequest directly (no registration step in V1)
	loginReq := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				WorkerId: fc.cfg.controllerID,
				Username: defaultTestUser,
				Source:   protos.MitmRequest_LoginRequest_PTC,
			},
		},
	}

	data, err := proto.Marshal(loginReq)
	if err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to marshal login request")
		return err
	}

	if err := wsConn.Write(ctx, ws.MessageBinary, data); err != nil {
		wsConn.Close(ws.StatusProtocolError, "failed to send login request")
		return err
	}

	fc.wsConn = wsConn
	fc.ctx, fc.cancel = context.WithCancel(ctx)
	fc.done = make(chan struct{})
	fc.connected.Store(true)

	go fc.readLoop()

	return nil
}

// readLoop reads incoming MitmResponses and dispatches them to pending channels
// or the configured response handler.
func (fc *FakeController) readLoop() {
	defer close(fc.done)

	for {
		reader, err := fc.wsConn.Reader(fc.ctx)
		if err != nil {
			return
		}

		payload := reader.Bytes()

		var resp protos.MitmResponse
		err = proto.Unmarshal(payload, &resp)
		reader.Done()
		if err != nil {
			continue
		}

		fc.lastResp.Store(&resp)

		// Check if there's a pending request waiting for this response
		fc.pendingMu.Lock()
		ch, ok := fc.pending[resp.Id]
		if ok {
			ch <- &resp
			delete(fc.pending, resp.Id)
		}
		fc.pendingMu.Unlock()

		if fc.cfg.respHandler != nil {
			fc.cfg.respHandler(&resp)
		}
	}
}
