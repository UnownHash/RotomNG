package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/UnownHash/RotomNG/libs/ws"
)

// deviceControlInitMessage mirrors mitm.DeviceControlInitMessage, the JSON the
// device sends immediately after connecting to /control.
type deviceControlInitMessage struct {
	DeviceID string `json:"deviceId"`
	Version  string `json:"version"`
	Origin   string `json:"origin"`
	PublicIP string `json:"publicIp"`
}

// deviceCommandRequest mirrors mitm.DeviceCommandRequest sent by RotomNG.
type deviceCommandRequest struct {
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Payload any    `json:"payload"`
}

// deviceCommandReply mirrors mitm.DeviceCommandReply the device sends back.
type deviceCommandReply struct {
	ID     int64           `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// device simulates a device control connection (the /control endpoint). It
// answers the management commands RotomNG issues (memory usage, screen size,
// etc.) so the device is reported as connected and able to run commands.
type device struct {
	cfg    Config
	logger *slog.Logger
	id     string
}

func newDevice(cfg Config, logger *slog.Logger, id string) *device {
	return &device{cfg: cfg, logger: logger.With(slog.String("device_id", id)), id: id}
}

func (d *device) run(ctx context.Context) error {
	conn, err := dialWS(ctx, d.cfg.DeviceEndpoint, "/control", d.cfg.DeviceSecret, "rotom-mock-device")
	if err != nil {
		return err
	}
	defer conn.Close(ws.StatusNormalClosure, "")

	init := deviceControlInitMessage{
		DeviceID: d.id,
		Version:  d.cfg.Version,
		Origin:   d.cfg.Origin,
		PublicIP: "127.0.0.1",
	}
	if err := conn.WriteJSON(ctx, init); err != nil {
		return fmt.Errorf("send device init: %w", err)
	}
	d.logger.Info("device control connected")

	// Reader does not observe ctx; expire the read deadline on cancellation so
	// the loop below unblocks and the tool can shut down.
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetReadDeadline(time.Now())
	})
	defer stop()

	for {
		reader, err := conn.Reader(ctx)
		if err != nil {
			return fmt.Errorf("read command: %w", err)
		}

		var cmd deviceCommandRequest
		if err := json.Unmarshal(reader.Bytes(), &cmd); err != nil {
			reader.Done()
			d.logger.Debug("ignoring undecodable device command", slog.String("error", err.Error()))
			continue
		}
		reader.Done()

		reply := deviceCommandReply{
			ID:     cmd.ID,
			Status: 200,
			Body:   replyBodyForCommand(cmd.Method),
		}
		if err := conn.WriteJSON(ctx, reply); err != nil {
			return fmt.Errorf("send command reply: %w", err)
		}
		d.logger.Debug("answered device command", slog.String("method", cmd.Method))
	}
}

// replyBodyForCommand returns a plausible JSON body for the commands RotomNG's
// device monitor and API can issue.
func replyBodyForCommand(method string) json.RawMessage {
	switch method {
	case "getMemoryUsage":
		// Matches mitm.DeviceMemory.
		return json.RawMessage(`{"memFree":524288000,"memMitm":104857600,"memStart":52428800}`)
	case "getScreenSize":
		return json.RawMessage(`{"width":1080,"height":1920}`)
	case "getLogcat":
		return json.RawMessage(`{"zipData":""}`)
	case "runJob":
		return json.RawMessage(`{"status":"ok","message":"mock job complete"}`)
	default:
		// restartApp, reboot, and any unknown command: a 200 with empty body.
		return json.RawMessage(`{}`)
	}
}
