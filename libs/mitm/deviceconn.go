package mitm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/settings"
	"github.com/UnownHash/RotomNG/libs/ws"
)

const (
	// DefaultDeviceMonitorInterval is the default interval between device monitor checks.
	DefaultDeviceMonitorInterval = time.Minute
)

// DeviceMonitorSettings holds configuration for the device monitor.
type DeviceMonitorSettings struct {
	Interval time.Duration
}

// Validate checks the DeviceMonitorSettings for correctness.
func (s DeviceMonitorSettings) Validate() error {
	return nil
}

// GetDeviceMonitorDefaultSettings returns the default DeviceMonitorSettings.
func GetDeviceMonitorDefaultSettings() DeviceMonitorSettings {
	return DeviceMonitorSettings{
		Interval: DefaultDeviceMonitorInterval,
	}
}

type deviceMonitorSettingsContainer = settings.Container[DeviceMonitorSettings]

// DeviceMonitorConfig wraps a settings container for device monitor configuration.
type DeviceMonitorConfig struct {
	*deviceMonitorSettingsContainer
}

// Init initializes the DeviceMonitorConfig with the given settings.
func (cfg *DeviceMonitorConfig) Init(s DeviceMonitorSettings) (err error) {
	cfg.deviceMonitorSettingsContainer, err = settings.NewContainer(s)
	return
}

// FlexibleString is a string type that can be unmarshalled from either a JSON string or number.
type FlexibleString string

// UnmarshalJSON implements the json.Unmarshaler interface for FlexibleString.
func (s *FlexibleString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return errors.New("cannot marshal empty bytes into string")
	}
	if b[0] == '"' {
		target := (*string)(s)
		if err := json.Unmarshal(b, target); err != nil {
			return fmt.Errorf("unmarshal string: %w", err)
		}
		return nil
	}
	var i int64
	if err := json.Unmarshal(b, &i); err != nil {
		return fmt.Errorf("unmarshal int: %w", err)
	}
	*s = FlexibleString(strconv.FormatInt(i, 10))
	return nil
}

// DeviceControlInitMessage is the initial message sent by a device upon connecting.
type DeviceControlInitMessage struct {
	DeviceID string         `json:"deviceId,omitempty"`
	Version  FlexibleString `json:"version"`
	Origin   string         `json:"origin"`
	PublicIP string         `json:"publicIp"`
}

// DeviceCommandRequest represents a command sent to a device.
type DeviceCommandRequest struct {
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Payload any    `json:"payload"`
}

// DeviceCommandReply represents a device's response to a command.
type DeviceCommandReply struct {
	ID     int64           `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// DeviceMemory represents device memory usage information.
type DeviceMemory struct {
	Free  int64     `json:"memFree"`
	Mitm  int64     `json:"memMitm"`
	Start int64     `json:"memStart"`
	Time  time.Time `json:"-"`
}

// LogcatResponse represents logcat data response.
type LogcatResponse struct {
	ZipData string `json:"zipData"`
}

// DeviceConn manages a WebSocket connection to a MITM device.
type DeviceConn struct {
	id       string
	origin   string
	version  string
	publicIP string

	messageID  atomic.Int64
	messages   map[int64]chan DeviceCommandReply
	messagesMu sync.RWMutex

	lastMemory *atomic.Pointer[DeviceMemory]

	wsConn         DeviceWSConn
	statsCollector DeviceStatsCollector

	canRunCommands atomic.Bool
	onCloseOnce    sync.Once
	onClose        func()
}

// NewDeviceConn creates a new DeviceConn with the given parameters.
func NewDeviceConn(
	id string,
	origin string,
	version string,
	publicIP string,
	wsConn DeviceWSConn,
	statsCollector DeviceStatsCollector,
	lastMemory *atomic.Pointer[DeviceMemory],
) *DeviceConn {
	if statsCollector == nil {
		statsCollector = NewNoOpStatsCollector()
	}
	return &DeviceConn{
		id:             id,
		origin:         origin,
		version:        version,
		publicIP:       publicIP,
		wsConn:         wsConn,
		messages:       make(map[int64]chan DeviceCommandReply),
		statsCollector: statsCollector,
		lastMemory:     lastMemory,
	}
}

// ID returns the device connection identifier.
func (deviceConn *DeviceConn) ID() string {
	return deviceConn.id
}

// Origin returns the device origin.
func (deviceConn *DeviceConn) Origin() string {
	return deviceConn.origin
}

// Version returns the device version string.
func (deviceConn *DeviceConn) Version() string {
	return deviceConn.version
}

// PublicIP returns the device's public IP address.
func (deviceConn *DeviceConn) PublicIP() string {
	return deviceConn.publicIP
}

// CanRunCommands reports whether the device is ready to accept commands.
func (deviceConn *DeviceConn) CanRunCommands() bool {
	return deviceConn.canRunCommands.Load()
}

// GetConnStats returns the WebSocket connection statistics for this device.
func (deviceConn *DeviceConn) GetConnStats() ws.ConnStats {
	return deviceConn.wsConn.GetStats()
}

// GetMemoryUsage retrieves current memory usage statistics from the device.
func (deviceConn *DeviceConn) GetMemoryUsage(ctx context.Context) (*DeviceMemory, error) {
	if !deviceConn.CanRunCommands() {
		return nil, errDeviceCommandsUnavailable
	}

	commandName := "getMemoryUsage"

	deviceConn.statsCollector.IncrDeviceCommandExecuted(deviceConn.origin, commandName)

	reply, err := deviceConn.executeCommand(ctx, commandName, nil, 5*time.Second)
	if err != nil {
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to get memory usage: %w", err)
	}

	if reply.Status != 200 {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("command failed with status %d", reply.Status)
	}

	var memory DeviceMemory
	if err := json.Unmarshal([]byte(reply.Body), &memory); err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to unmarshal memory status: %w", err)
	}
	memory.Time = time.Now()
	deviceConn.lastMemory.Store(&memory)

	// Set memory usage gauges and track command success if stats collector is available
	deviceConn.statsCollector.SetDeviceMemoryFree(deviceConn.origin, float64(memory.Free))
	deviceConn.statsCollector.SetDeviceMemoryMITM(deviceConn.origin, float64(memory.Mitm))
	deviceConn.statsCollector.SetDeviceMemoryStart(deviceConn.origin, float64(memory.Start))
	deviceConn.statsCollector.IncrDeviceCommandSuccess(deviceConn.origin, commandName)

	return &memory, nil
}

// ScreenSize represents device screen dimensions.
type ScreenSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// GetScreenSize retrieves the device screen dimensions.
func (deviceConn *DeviceConn) GetScreenSize(ctx context.Context) (*ScreenSize, error) {
	if !deviceConn.CanRunCommands() {
		return nil, errDeviceCommandsUnavailable
	}

	commandName := "getScreenSize"

	deviceConn.statsCollector.IncrDeviceCommandExecuted(deviceConn.origin, commandName)

	reply, err := deviceConn.executeCommand(ctx, commandName, nil, 5*time.Second)
	if err != nil {
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to get screen size: %w", err)
	}

	if reply.Status != 200 {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("command failed with status %d", reply.Status)
	}

	var screenSize ScreenSize
	if err := json.Unmarshal(reply.Body, &screenSize); err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to unmarshal screen size: %w", err)
	}

	// Track command success
	deviceConn.statsCollector.IncrDeviceCommandSuccess(deviceConn.origin, commandName)

	return &screenSize, nil
}

// RestartApp restarts the application on the device.
func (deviceConn *DeviceConn) RestartApp(ctx context.Context) error {
	if !deviceConn.CanRunCommands() {
		return errDeviceCommandsUnavailable
	}

	commandName := "restartApp"

	// Track command execution
	deviceConn.statsCollector.IncrDeviceCommandExecuted(deviceConn.origin, commandName)

	reply, err := deviceConn.executeCommand(ctx, commandName, nil, 5*time.Second)
	if err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return fmt.Errorf("failed to restart app: %w", err)
	}

	if reply.Status != 200 {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return fmt.Errorf("command failed with status %d", reply.Status)
	}

	// Track command success
	deviceConn.statsCollector.IncrDeviceCommandSuccess(deviceConn.origin, commandName)

	return nil
}

// Reboot reboots the device.
func (deviceConn *DeviceConn) Reboot(ctx context.Context) error {
	if !deviceConn.CanRunCommands() {
		return errDeviceCommandsUnavailable
	}

	commandName := "reboot"

	// Track command execution
	deviceConn.statsCollector.IncrDeviceCommandExecuted(deviceConn.origin, commandName)

	reply, err := deviceConn.executeCommand(ctx, commandName, nil, 5*time.Second)
	if err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return fmt.Errorf("failed to reboot device: %w", err)
	}

	if reply.Status != 200 {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return fmt.Errorf("command failed with status %d", reply.Status)
	}

	// Track command success
	deviceConn.statsCollector.IncrDeviceCommandSuccess(deviceConn.origin, commandName)

	return nil
}

// GetLogcat retrieves device logcat data as a zip file.
func (deviceConn *DeviceConn) GetLogcat(ctx context.Context) ([]byte, error) {
	if !deviceConn.CanRunCommands() {
		return nil, errDeviceCommandsUnavailable
	}

	commandName := "getLogcat"

	// Track command execution
	deviceConn.statsCollector.IncrDeviceCommandExecuted(deviceConn.origin, commandName)

	reply, err := deviceConn.executeCommand(ctx, commandName, nil, 30*time.Second)
	if err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to get logcat: %w", err)
	}

	if reply.Status != 200 {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("command failed with status %d", reply.Status)
	}

	var logcatResponse LogcatResponse
	if err := json.Unmarshal(reply.Body, &logcatResponse); err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to unmarshal logcat response: %w", err)
	}

	deviceConn.statsCollector.IncrDeviceCommandSuccess(deviceConn.origin, commandName)

	decoded, err := base64.StdEncoding.DecodeString(logcatResponse.ZipData)
	if err != nil {
		return nil, fmt.Errorf("decode logcat zip data: %w", err)
	}
	return decoded, nil
}

// RunJob executes a job command on the device.
func (deviceConn *DeviceConn) RunJob(ctx context.Context, command string) (*jobs.RunJobResponse, error) {
	if !deviceConn.CanRunCommands() {
		return nil, errDeviceCommandsUnavailable
	}

	commandName := "runJob"

	// Track command execution
	deviceConn.statsCollector.IncrDeviceCommandExecuted(deviceConn.origin, commandName)

	payload := map[string]string{
		"command": command,
	}

	reply, err := deviceConn.executeCommand(ctx, commandName, payload, 12*time.Second)
	if err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to run job: %w", err)
	}

	if reply.Status != 200 {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("job failed with status %d", reply.Status)
	}

	var jobResponse jobs.RunJobResponse
	if err := json.Unmarshal(reply.Body, &jobResponse); err != nil {
		// Track command error
		deviceConn.statsCollector.IncrDeviceCommandError(deviceConn.origin, commandName)
		return nil, fmt.Errorf("failed to unmarshal job response: %w", err)
	}

	// Track command success
	deviceConn.statsCollector.IncrDeviceCommandSuccess(deviceConn.origin, commandName)

	return &jobResponse, nil
}

// ReadInitMessage reads and returns the device control initialization message.
func (deviceConn *DeviceConn) ReadInitMessage(ctx context.Context) (DeviceControlInitMessage, error) {
	var initMsg DeviceControlInitMessage
	if err := deviceConn.wsConn.ReadJSON(ctx, &initMsg); err != nil {
		return initMsg, fmt.Errorf("failed to read control init message from device: %w", err)
	}
	return initMsg, nil
}

// Run starts the device connection's main processing loop and monitor.
func (deviceConn *DeviceConn) Run(ctx context.Context, monitorConfig DeviceMonitorConfig) {
	var wg sync.WaitGroup

	logger := logging.LoggerFromContext(ctx)

	// wait for any started goroutines to exit
	defer logger.LogAttrs(ctx, slog.LevelDebug, "device control done waiting on goroutines")
	defer wg.Wait()
	defer logger.LogAttrs(ctx, slog.LevelDebug, "device control waiting on goroutines during exit")

	// Create a context that can be cancelled to stop the goroutines
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// Start the monitor goroutine
	wg.Go(func() {
		deviceConn.runMonitor(runCtx, monitorConfig)
	})

	deviceConn.canRunCommands.Store(true)
	defer deviceConn.canRunCommands.Store(false)

	// Main message processing loop
	for {
		reader, err := deviceConn.wsConn.Reader(ctx)
		if err != nil {
			ws.LogWebsocketReadError(ctx, err)
			return
		}
		if err := deviceConn.processWebsocketMessage(ctx, reader); err != nil {
			if !errors.Is(err, context.Canceled) {
				logger := logging.LoggerFromContext(ctx)
				if logger != nil {
					logger.LogAttrs(ctx, slog.LevelError, "error processing websocket message", slog.String("error", err.Error()))
				}
			}
			return
		}
	}
}

// SetCloseHandler sets a function to be called when the device connection is closed.
func (deviceConn *DeviceConn) SetCloseHandler(fn func()) {
	deviceConn.onClose = fn
}

// Close closes the device connection with the given status code and text.
func (deviceConn *DeviceConn) Close(code ws.StatusCode, text string) error {
	deviceConn.onCloseOnce.Do(func() {
		if fn := deviceConn.onClose; fn != nil {
			fn()
		}
	})
	return deviceConn.wsConn.Close(code, text)
}

func (deviceConn *DeviceConn) processCommandReply(ctx context.Context, reply DeviceCommandReply) {
	messageID := reply.ID

	replyChan := func() chan<- DeviceCommandReply {
		deviceConn.messagesMu.Lock()
		defer deviceConn.messagesMu.Unlock()

		replyChan, ok := deviceConn.messages[messageID]
		if ok {
			delete(deviceConn.messages, messageID)
		}
		return replyChan
	}()

	if replyChan != nil {
		replyChan <- reply
		close(replyChan)
	} else {
		logger := logging.LoggerFromContext(ctx)
		if logger != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "command reply being dropped", slog.Int64("command_id", messageID), slog.String("device_id", deviceConn.id))
		}
	}
}

func (deviceConn *DeviceConn) processWebsocketMessage(ctx context.Context, reader io.Reader) error {
	var msg DeviceCommandReply

	decoder := json.NewDecoder(reader)
	err := decoder.Decode(&msg)
	if err == nil {
		deviceConn.processCommandReply(ctx, msg)
		return nil
	}
	ws.LogWebsocketReadError(ctx, err)
	return fmt.Errorf("decode device message: %w", err)
}

func (deviceConn *DeviceConn) executeCommand(ctx context.Context, method string, payload any, timeout time.Duration) (*DeviceCommandReply, error) {
	messageID := deviceConn.messageID.Add(1)

	request := DeviceCommandRequest{
		ID:      messageID,
		Method:  method,
		Payload: payload,
	}

	replyChan := make(chan DeviceCommandReply, 1)

	defer func() {
		deviceConn.messagesMu.Lock()
		defer deviceConn.messagesMu.Unlock()
		if _, ok := deviceConn.messages[messageID]; ok {
			delete(deviceConn.messages, messageID)
			close(replyChan)
		}
	}()

	deviceConn.messagesMu.Lock()
	deviceConn.messages[messageID] = replyChan
	deviceConn.messagesMu.Unlock()

	err := deviceConn.wsConn.WriteJSONAsync(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to send command request: %w", err)
	}

	select {
	case reply := <-replyChan:
		// Reply received
		return &reply, nil
	case <-time.After(timeout):
		// Timeout occurred
		return nil, fmt.Errorf("command timeout after %s", timeout)
	case <-ctx.Done():
		// Context cancelled
		return nil, fmt.Errorf("execute command: %w", ctx.Err())
	}
}

func (deviceConn *DeviceConn) checkDevice(ctx context.Context, _ DeviceMonitorSettings) {
	logger := logging.LoggerFromContext(ctx)

	memory, err := deviceConn.GetMemoryUsage(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) && logger != nil {
			logger.LogAttrs(ctx, slog.LevelError, "monitor failed to get memory usage", slog.String("error", err.Error()))
		}
		return
	}
	if logger != nil {
		logger.LogAttrs(ctx, slog.LevelDebug, "device memory usage", slog.Int64("free", memory.Free), slog.Int64("mitm", memory.Mitm), slog.Int64("start", memory.Start))
	}
}

// runMonitor periodically queries memory usage and optionally performs device management.
func (deviceConn *DeviceConn) runMonitor(ctx context.Context, config DeviceMonitorConfig) {
	wakeUp := make(chan struct{}, 1)
	defer close(wakeUp)

	dereg := config.Notify(func(_ DeviceMonitorSettings) {
		select {
		case wakeUp <- struct{}{}:
		default:
		}
	})
	defer dereg()

	var timer *time.Timer
	var timerCh <-chan time.Time

	maybeDoTimer := func(interval time.Duration) {
		if interval <= 0 {
			timer = nil
			timerCh = nil
			return
		}
		timer = time.NewTimer(interval)
		timerCh = timer.C
	}

	settings := config.GetSettings()
	// always run once
	deviceConn.checkDevice(ctx, settings)

	for {
		maybeDoTimer(settings.Interval)
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-timerCh:
			deviceConn.checkDevice(ctx, settings)
		case <-wakeUp:
			if timer != nil {
				timer.Stop()
			}
		}
		settings = config.GetSettings()
	}
}

// ReadDeviceControlInitMessage reads the device control initialization message from a WebSocket connection.
func ReadDeviceControlInitMessage(ctx context.Context, wsConn DeviceWSConn) (DeviceControlInitMessage, error) {
	var initMsg DeviceControlInitMessage
	if err := wsConn.ReadJSON(ctx, &initMsg); err != nil {
		return initMsg, fmt.Errorf("failed to read control init message from device: %w", err)
	}
	return initMsg, nil
}
