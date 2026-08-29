package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// Protocol version the controller advertises. Must match
// connections.ProtoMajorVersion / ProtoMinorVersion for v2 registration.
const (
	controllerProtoMajor = 2
	controllerProtoMinor = 0
)

// rpcMethods is a small rotation of fake POGO RPC method ids so the proxied
// traffic (and the resulting stats) looks varied.
var rpcMethods = []int32{2, 4, 106, 137, 145}

// controller simulates a controller connection (the /controller v2 endpoint).
// It registers, is assigned a worker, performs a login, then sends periodic RPC
// requests and reads the responses the worker proxies back.
type controller struct {
	cfg    Config
	logger *slog.Logger
	id     string
	nextID atomic.Uint32
}

func newController(cfg Config, logger *slog.Logger, id string) *controller {
	return &controller{cfg: cfg, logger: logger.With(slog.String("controller_id", id)), id: id}
}

func (c *controller) run(ctx context.Context) error {
	conn, err := dialWS(ctx, c.cfg.ControllerEndpoint, "/controller", c.cfg.ControllerSecret, "rotom-mock-controller")
	if err != nil {
		return err
	}
	defer conn.Close(ws.StatusNormalClosure, "")

	workerID, err := c.register(ctx, conn)
	if err != nil {
		return err
	}
	c.logger.Info("controller registered", slog.String("assigned_worker_id", workerID))

	if err := c.sendLogin(ctx, conn, workerID); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	return c.proxyLoop(ctx, conn)
}

// register performs the v2 registration handshake and returns the assigned
// worker id on success.
func (c *controller) register(ctx context.Context, conn *ws.Conn) (string, error) {
	req := &protos.RegisterControllerRequest{
		Id:                c.id,
		ProtoMajorVersion: controllerProtoMajor,
		ProtoMinorVersion: controllerProtoMinor,
		Weight:            int32(c.cfg.Weight), //nolint:gosec // range validated in Config.Validate
	}
	payload, err := proto.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal register request: %w", err)
	}
	if err := conn.WriteAsync(ctx, ws.MessageBinary, payload); err != nil {
		return "", fmt.Errorf("send register request: %w", err)
	}

	reader, err := conn.Reader(ctx)
	if err != nil {
		return "", fmt.Errorf("read register response: %w", err)
	}
	var resp protos.RegisterControllerResponse
	if err := proto.Unmarshal(reader.Bytes(), &resp); err != nil {
		reader.Done()
		return "", fmt.Errorf("decode register response: %w", err)
	}
	reader.Done()

	if resp.Status != protos.RegisterControllerResponse_SUCCESS {
		return "", fmt.Errorf("registration rejected: status=%s reason=%q", resp.Status, resp.StatusReason)
	}
	if resp.AssignedWorkerId == "" {
		return "", errors.New("registration succeeded but no worker was assigned")
	}
	return resp.AssignedWorkerId, nil
}

// sendLogin sends the initial LOGIN MitmRequest, which RotomNG reads as part of
// registration and proxies to the assigned worker.
func (c *controller) sendLogin(ctx context.Context, conn *ws.Conn, workerID string) error {
	req := &protos.MitmRequest{
		Id:     c.nextRequestID(),
		Method: protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{
			LoginRequest: &protos.MitmRequest_LoginRequest{
				Username: c.id + "@mock",
				//nolint:staticcheck
				Source:   protos.MitmRequest_LoginRequest_PTC,
				WorkerId: workerID,
			},
		},
	}
	payload, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal login: %w", err)
	}
	if err := conn.WriteAsync(ctx, ws.MessageBinary, payload); err != nil {
		return fmt.Errorf("write login: %w", err)
	}
	return nil
}

// proxyLoop concurrently reads proxied responses and sends periodic RPC
// requests until ctx is cancelled or the connection fails.
func (c *controller) proxyLoop(ctx context.Context, conn *ws.Conn) error {
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	readErr := make(chan error, 1)
	go func() { readErr <- c.readResponses(loopCtx, conn) }()

	ticker := time.NewTicker(c.cfg.RPCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-loopCtx.Done():
			return fmt.Errorf("controller loop ended: %w", loopCtx.Err())
		case err := <-readErr:
			return err
		case <-ticker.C:
			if err := c.sendRPC(loopCtx, conn); err != nil {
				return fmt.Errorf("send rpc: %w", err)
			}
		}
	}
}

// readResponses drains MitmResponse messages proxied back from the worker.
func (c *controller) readResponses(ctx context.Context, conn *ws.Conn) error {
	for {
		reader, err := conn.Reader(ctx)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		var resp protos.MitmResponse
		if err := proto.Unmarshal(reader.Bytes(), &resp); err == nil {
			c.logger.Debug("received response",
				slog.Uint64("id", uint64(resp.Id)),
				slog.String("status", resp.Status.String()),
			)
		}
		reader.Done()
	}
}

// sendRPC sends a single RPC MitmRequest carrying one rotating fake method.
func (c *controller) sendRPC(ctx context.Context, conn *ws.Conn) error {
	id := c.nextRequestID()
	method := rpcMethods[int(id)%len(rpcMethods)]

	req := &protos.MitmRequest{
		Id:     id,
		Method: protos.MitmRequest_RPC_REQUEST,
		Payload: &protos.MitmRequest_RpcRequest_{
			RpcRequest: &protos.MitmRequest_RpcRequest{
				Request: []*protos.MitmRequest_RpcRequest_SingleRpcRequest{
					{Method: method, Payload: []byte{}},
				},
				Lat: 40.0,
				Lon: -74.0,
			},
		},
	}
	payload, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal rpc: %w", err)
	}
	if err := conn.WriteAsync(ctx, ws.MessageBinary, payload); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}
	c.logger.Debug("sent rpc request", slog.Uint64("id", uint64(id)), slog.Int("method", int(method)))
	return nil
}

func (c *controller) nextRequestID() uint32 {
	return c.nextID.Add(1)
}
