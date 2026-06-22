package main

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// worker simulates a MITM worker connection (the / endpoint on the device
// listener). After sending its welcome message it waits to be assigned to a
// controller, then echoes back a successful response for every MitmRequest it
// is proxied (login first, then RPC requests).
type worker struct {
	cfg      Config
	logger   *slog.Logger
	deviceID string
	id       string
}

func newWorker(cfg Config, logger *slog.Logger, deviceID, id string) *worker {
	return &worker{
		cfg:      cfg,
		logger:   logger.With(slog.String("worker_id", id)),
		deviceID: deviceID,
		id:       id,
	}
}

func (w *worker) run(ctx context.Context) error {
	conn, err := dialWS(ctx, w.cfg.DeviceEndpoint, "/", w.cfg.DeviceSecret, "rotom-mock-worker")
	if err != nil {
		return err
	}
	defer conn.Close(ws.StatusNormalClosure, "")

	welcome := &protos.WelcomeMessage{
		WorkerId:    w.id,
		Origin:      w.cfg.Origin,
		DeviceId:    w.deviceID,
		VersionCode: int32(w.cfg.VersionCode), //nolint:gosec // range validated in Config.Validate
		VersionName: w.cfg.Version,
		Useragent:   "rotom-mock-worker/" + w.cfg.Version,
		Platform:    protos.WelcomeMessage_ANDROID,
	}
	payload, err := proto.Marshal(welcome)
	if err != nil {
		return fmt.Errorf("marshal welcome: %w", err)
	}
	if err := conn.WriteAsync(ctx, ws.MessageBinary, payload); err != nil {
		return fmt.Errorf("send welcome: %w", err)
	}
	w.logger.Info("worker connected")

	for {
		reader, err := conn.Reader(ctx)
		if err != nil {
			return fmt.Errorf("read request: %w", err)
		}

		var req protos.MitmRequest
		if err := proto.Unmarshal(reader.Bytes(), &req); err != nil {
			reader.Done()
			w.logger.Debug("ignoring undecodable request", slog.String("error", err.Error()))
			continue
		}
		reader.Done()

		resp := w.responseFor(&req)
		respPayload, err := proto.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		if err := conn.WriteAsync(ctx, ws.MessageBinary, respPayload); err != nil {
			return fmt.Errorf("send response: %w", err)
		}
		w.logger.Debug("answered request",
			slog.String("method", protos.MITMRequestMethodName(req.Method)),
			slog.Uint64("id", uint64(req.Id)),
		)
	}
}

// responseFor builds a successful MitmResponse for the given request, echoing
// the request id so RotomNG can correlate it.
func (w *worker) responseFor(req *protos.MitmRequest) *protos.MitmResponse {
	resp := &protos.MitmResponse{
		Id:     req.Id,
		Status: protos.MitmResponse_SUCCESS,
	}

	if req.GetLoginRequest() != nil {
		resp.Payload = &protos.MitmResponse_LoginResponse_{
			LoginResponse: &protos.MitmResponse_LoginResponse{
				WorkerId:  w.id,
				Status:    protos.AuthStatus_AUTH_STATUS_GOT_AUTH_TOKEN,
				Useragent: "rotom-mock-worker/" + w.cfg.Version,
			},
		}
		return resp
	}

	rpcReq := req.GetRpcRequest()
	rpcResp := &protos.MitmResponse_RpcResponse{
		RpcStatus: protos.RpcStatus_RPC_STATUS_SUCCESS,
	}
	if rpcReq != nil {
		for _, single := range rpcReq.Request {
			rpcResp.Response = append(rpcResp.Response, &protos.MitmResponse_RpcResponse_SingleRpcResponse{
				Method:  single.Method,
				Payload: []byte{},
			})
		}
	}
	resp.Payload = &protos.MitmResponse_RpcResponse_{RpcResponse: rpcResp}
	return resp
}
