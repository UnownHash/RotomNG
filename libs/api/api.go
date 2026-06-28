// Package api provides types and converters for the RotomNG HTTP API responses.
package api

import (
	"slices"
	"time"

	"github.com/UnownHash/RotomNG/libs/connections"
	"github.com/UnownHash/RotomNG/libs/jobs"
	"github.com/UnownHash/RotomNG/libs/stats"
	"github.com/UnownHash/RotomNG/libs/ws"
)

// Converter handles conversion between internal types and API response types.
type Converter[D MITMDevice[W], W MITMWorker, C Controller] struct{}

// NewConverter creates a new Converter instance.
func NewConverter[D MITMDevice[W], W MITMWorker, C Controller]() *Converter[D, W, C] {
	return &Converter[D, W, C]{}
}

func timeToMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// CommonStats contains common WebSocket connection statistics.
type CommonStats struct {
	MessageLastReceivedAtMs int64 `json:"message_last_received_at_ms"`
	MessagesReceived        int64 `json:"messages_received"`
	BytesReceived           int64 `json:"bytes_received"`

	MessageLastSentAtMs int64 `json:"message_last_sent_at_ms"`
	MessagesSent        int64 `json:"messages_sent"`
	BytesSent           int64 `json:"bytes_sent"`

	// LastSeenAtMs is the most recent activity on the connection, including
	// ping/pong keep-alive, not just data messages. It is the correct field for
	// a "last seen" display.
	LastSeenAtMs int64 `json:"last_seen_at_ms"`
}

// CommonStatsFromWebsocketStats converts WebSocket stats to CommonStats.
func (converter *Converter[D, W, C]) CommonStatsFromWebsocketStats(wsStats ws.ConnStats) CommonStats {
	return CommonStats{
		MessageLastReceivedAtMs: timeToMs(wsStats.LastReceivedAt),
		MessagesReceived:        wsStats.MessagesReceived,
		BytesReceived:           wsStats.BytesReceived,

		MessageLastSentAtMs: timeToMs(wsStats.LastSentAt),
		MessagesSent:        wsStats.MessagesSent,
		BytesSent:           wsStats.BytesSent,

		LastSeenAtMs: timeToMs(wsStats.LastSeenAt()),
	}
}

// DeviceSession represents a device's current session information.
type DeviceSession struct {
	CommonStats

	ConnectedAtMs int64 `json:"connected_at_ms"`
}

// DeviceMemory represents a device's memory usage statistics.
type DeviceMemory struct {
	Free  int64 `json:"free"`
	Mitm  int64 `json:"mitm"`
	Start int64 `json:"start"`
}

// Device represents a device in API responses.
type Device struct {
	CommonStats

	ID                       string         `json:"id"`
	Origin                   string         `json:"origin"`
	Version                  string         `json:"version"`
	PublicIP                 string         `json:"public_ip"`
	WorkerCount              int            `json:"worker_count"`
	WorkerInUseCount         int            `json:"worker_in_use_count"`
	WorkerInUsePercent       float64        `json:"worker_in_use_percent"`
	WorkerInUseWeight        int            `json:"worker_in_use_weight"`
	WorkerInUseWeightPercent float64        `json:"worker_in_use_weight_percent"`
	WorkerMaxWeight          int            `json:"worker_max_weight"`
	LastConnectedAtMs        int64          `json:"last_connected_at_ms"`
	Enabled                  bool           `json:"enabled"`
	IsConnected              bool           `json:"is_connected"`
	CanBeUsed                bool           `json:"can_be_used"`
	LastMemory               *DeviceMemory  `json:"last_memory,omitempty"`
	Session                  *DeviceSession `json:"session,omitempty"`
	Workers                  []Worker       `json:"workers,omitempty"`
	IsInUse                  bool           `json:"is_in_use"`
}

// GetLastMemoryFromMITMDevice gets the most recent memory stats from the device.
func (converter *Converter[D, W, C]) GetLastMemoryFromMITMDevice(device D) *DeviceMemory {
	if memStats := device.GetLastMemoryUsage(); memStats != nil {
		return &DeviceMemory{
			Free:  memStats.Free,
			Mitm:  memStats.Mitm,
			Start: memStats.Start,
		}
	}
	return nil
}

// NewDeviceFromDevice converts an internal device to an Device.
func (converter *Converter[D, W, C]) NewDeviceFromDevice(device D, includeWorkerStats bool, includeWorkers bool) Device {
	var deviceSession *DeviceSession

	sessionStats, totalStats := device.WebsocketStats()
	isConnected := device.IsConnected()
	if isConnected {
		deviceSession = &DeviceSession{
			CommonStats:   converter.CommonStatsFromWebsocketStats(sessionStats),
			ConnectedAtMs: timeToMs(sessionStats.ConnectedAt),
		}
	}

	canBeUsed := isConnected && device.IsSelectionEnabled()

	apiDevice := Device{
		CommonStats:       converter.CommonStatsFromWebsocketStats(totalStats),
		ID:                device.ID(),
		Origin:            device.Origin(),
		Version:           device.Version(),
		PublicIP:          device.PublicIP(),
		LastConnectedAtMs: timeToMs(totalStats.ConnectedAt),
		Enabled:           device.IsSelectionEnabled(),
		IsConnected:       deviceSession != nil,
		CanBeUsed:         canBeUsed,
		LastMemory:        converter.GetLastMemoryFromMITMDevice(device),
		Session:           deviceSession,
	}

	if !includeWorkerStats && !includeWorkers {
		return apiDevice
	}

	workers := device.Workers()
	apiDevice.WorkerCount = len(workers)

	if includeWorkers {
		apiDevice.Workers = make([]Worker, apiDevice.WorkerCount)
	}
	for idx, worker := range workers {
		if controller := worker.GetModeInfo().Controller; controller != nil && !controller.IsZero() {
			apiDevice.WorkerInUseCount++
			apiDevice.WorkerInUseWeight += controller.Weight()
			apiDevice.IsInUse = true
		}
		if includeWorkers {
			apiDevice.Workers[idx] = converter.NewWorkerFromWorker(worker, canBeUsed)
		}
	}

	if apiDevice.WorkerCount > 0 {
		apiDevice.WorkerInUsePercent = float64(apiDevice.WorkerInUseCount) * 100 / float64(apiDevice.WorkerCount)
		maxWeight := apiDevice.WorkerCount * connections.MaxControllerWeight
		apiDevice.WorkerMaxWeight = maxWeight
		apiDevice.WorkerInUseWeightPercent = float64(apiDevice.WorkerInUseWeight) * 100 / float64(maxWeight)
	}

	return apiDevice
}

// ControllerResponse represents a controller in API responses.
type ControllerResponse struct {
	CommonStats

	ID                string  `json:"id"`
	UUID              string  `json:"uuid"`
	UserAgent         string  `json:"user_agent"`
	Weight            int     `json:"weight"`
	ProtoMajorVersion int     `json:"proto_major_version"`
	ProtoMinorVersion int     `json:"proto_minor_version"`
	WorkerID          *string `json:"worker_id,omitempty"`
	AccountUsername   string  `json:"account_username"`
	AccountSource     string  `json:"account_source"`
	ConnectedAtMs     int64   `json:"connected_at_ms"`
}

// WorkerSession represents a worker's current session information.
type WorkerSession struct {
	CommonStats

	ConnectedAtMs int64               `json:"connected_at_ms"`
	Controller    *ControllerResponse `json:"controller,omitempty"`
}

// TimeWindowedStats contains request rate and latency statistics over time windows.
type TimeWindowedStats struct {
	RequestsRateOver30Seconds float64 `json:"requests_rate_over_30_seconds"`
	RequestsRateOver1Min      float64 `json:"requests_rate_over_1_min"`
	RequestsRateOver5Min      float64 `json:"requests_rate_over_5_min"`
	RequestsRateOver15Min     float64 `json:"requests_rate_over_15_min"`
	RequestMsAvgOver30Seconds float64 `json:"request_ms_avg_over_30_seconds"`
	RequestMsAvgOver1Min      float64 `json:"request_ms_avg_over_1_min"`
	RequestMsAvgOver5Min      float64 `json:"request_ms_avg_over_5_min"`
	RequestMsAvgOver15Min     float64 `json:"request_ms_avg_over_15_min"`
}

// Worker represents a worker in API responses.
type Worker struct {
	CommonStats

	ID                string             `json:"id"`
	DeviceID          string             `json:"device_id"`
	Origin            string             `json:"origin"`
	VersionCode       int32              `json:"version_code"`
	VersionName       string             `json:"version_name"`
	StatsDisabled     bool               `json:"stats_disabled,omitzero"`
	UserAgent         string             `json:"user_agent"`
	LastConnectedAtMs int64              `json:"last_connected_at_ms"`
	IsConnected       bool               `json:"is_connected"`
	IsInUse           bool               `json:"is_in_use"`
	Platform          string             `json:"platform"`
	Weight            *int               `json:"weight,omitempty"`
	CanBeUsed         bool               `json:"can_be_used"`
	Session           WorkerSession      `json:"session,omitzero"`
	TimeWindowedStats *TimeWindowedStats `json:"time_windowed_stats,omitempty"`
}

// NewControllerResponseFromController converts an internal controller to an ControllerResponse.
func (converter *Converter[D, W, C]) NewControllerResponseFromController(controller Controller) ControllerResponse {
	workerID := func() *string {
		if wID := controller.WorkerID(); wID != "" {
			return &wID
		}
		return nil
	}()

	sessionStats := controller.WebsocketStats()
	accountInfo := controller.AccountInfo()

	return ControllerResponse{
		CommonStats:       converter.CommonStatsFromWebsocketStats(sessionStats),
		ID:                controller.ID(),
		UUID:              controller.UUID(),
		UserAgent:         controller.UserAgent(),
		Weight:            controller.Weight(),
		ProtoMajorVersion: controller.ProtoMajorVersion(),
		ProtoMinorVersion: controller.ProtoMinorVersion(),
		WorkerID:          workerID,
		AccountUsername:   accountInfo.Username,
		AccountSource:     accountInfo.Source,
		ConnectedAtMs:     timeToMs(sessionStats.ConnectedAt),
	}
}

// CreateTimeWindowedStats converts windowed count/duration stats into the API
// representation. Windows must be in order: 30s, 1m, 5m, 15m (matching
// mitm.StatsWindows).
func (converter *Converter[D, W, C]) CreateTimeWindowedStats(s stats.CountDurationWindows[uint64]) *TimeWindowedStats {
	return &TimeWindowedStats{
		RequestsRateOver30Seconds: s.Counts[0].RatePerSecond(),
		RequestsRateOver1Min:      s.Counts[1].RatePerSecond(),
		RequestsRateOver5Min:      s.Counts[2].RatePerSecond(),
		RequestsRateOver15Min:     s.Counts[3].RatePerSecond(),
		RequestMsAvgOver30Seconds: s.Durations[0].Avg(),
		RequestMsAvgOver1Min:      s.Durations[1].Avg(),
		RequestMsAvgOver5Min:      s.Durations[2].Avg(),
		RequestMsAvgOver15Min:     s.Durations[3].Avg(),
	}
}

// CreateTimeWindowedStatsFromWorker creates time-windowed statistics from a worker.
// Windows are returned in order: 30s, 1m, 5m, 15m (matching mitm.StatsWindows).
func (converter *Converter[D, W, C]) CreateTimeWindowedStatsFromWorker(worker W) *TimeWindowedStats {
	return converter.CreateTimeWindowedStats(worker.GetRequestStats())
}

// NewWorkerFromWorker converts an internal worker to an Worker.
func (converter *Converter[D, W, C]) NewWorkerFromWorker(worker W, canBeUsed bool) Worker {
	var weight *int
	var apiController *ControllerResponse

	workerModeInfo := worker.GetModeInfo()
	if controller := workerModeInfo.Controller; controller != nil && !controller.IsZero() {
		weight = func() *int {
			weight := controller.Weight()
			return &weight
		}()
		apiController = func() *ControllerResponse {
			apiController := converter.NewControllerResponseFromController(controller)
			return &apiController
		}()
	}

	sessionStats, totalStats := worker.WebsocketStats()
	workerSession := WorkerSession{
		CommonStats:   converter.CommonStatsFromWebsocketStats(sessionStats),
		ConnectedAtMs: timeToMs(sessionStats.ConnectedAt),
		Controller:    apiController,
	}

	// isConnected will always be true, since we only keep track of
	// connected workers.
	isConnected := true
	isInUse := isConnected && workerSession.Controller != nil

	var timeWindowedStats *TimeWindowedStats
	if !workerModeInfo.DisableStats {
		timeWindowedStats = converter.CreateTimeWindowedStatsFromWorker(worker)
	}

	return Worker{
		CommonStats:       converter.CommonStatsFromWebsocketStats(totalStats),
		ID:                worker.ID(),
		DeviceID:          worker.DeviceID(),
		Origin:            worker.Origin(),
		VersionCode:       worker.VersionCode(),
		VersionName:       worker.VersionName(),
		StatsDisabled:     workerModeInfo.DisableStats,
		UserAgent:         worker.UserAgent(),
		Platform:          worker.Platform().String(),
		LastConnectedAtMs: timeToMs(totalStats.ConnectedAt),
		IsConnected:       isConnected,
		IsInUse:           isInUse,
		Weight:            weight,
		CanBeUsed:         canBeUsed,
		Session:           workerSession,
		TimeWindowedStats: timeWindowedStats,
	}
}

// StatusResponse represents the full status API response.
type StatusResponse struct {
	Devices     []Device             `json:"devices"`
	Controllers []ControllerResponse `json:"controllers"`
	// GlobalStats holds aggregate request stats across all workers, including
	// those that have since disconnected. It is the accurate source for overall
	// req/s and avg ms, rather than summing the per-worker stats.
	GlobalStats *TimeWindowedStats `json:"global_stats,omitempty"`
}

// StatusResponseFromDevicesAndControllers builds a StatusResponse from devices,
// controllers, and the global aggregate request stats.
func (converter *Converter[D, W, C]) StatusResponseFromDevicesAndControllers(devices []D, controllers []C, globalStats stats.CountDurationWindows[uint64]) StatusResponse {
	apiDevices := make([]Device, len(devices))
	for idx, device := range devices {
		apiDevices[idx] = converter.NewDeviceFromDevice(device, true, true)
	}
	apiControllers := make([]ControllerResponse, len(controllers))
	for idx, controller := range controllers {
		apiControllers[idx] = converter.NewControllerResponseFromController(controller)
	}
	return StatusResponse{
		Devices:     apiDevices,
		Controllers: apiControllers,
		GlobalStats: converter.CreateTimeWindowedStats(globalStats),
	}
}

// Job represents a job definition in API responses.
type Job struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Exec        string `json:"exec"`
}

// ConnectionJobToAPIJob converts an internal job to an API Job.
func (converter *Converter[D, W, C]) ConnectionJobToAPIJob(job jobs.Job) Job {
	return Job{
		ID:          job.ID,
		Description: job.Description,
		Exec:        job.Exec,
	}
}

// ConnectionJobsToAPIJobs converts a slice of internal jobs to API Jobs.
func (converter *Converter[D, W, C]) ConnectionJobsToAPIJobs(jobsList []jobs.Job) []Job {
	apiJobs := make([]Job, len(jobsList))
	for idx, job := range jobsList {
		apiJobs[idx] = converter.ConnectionJobToAPIJob(job)
	}
	slices.SortFunc(apiJobs, func(a, b Job) int {
		if a.ID < b.ID {
			return -1
		}
		return 1
	})
	return apiJobs
}

// JobInstance represents a job instance in API responses.
type JobInstance struct {
	ID           uint64 `json:"id,omitzero"`
	JobID        string `json:"job_id"`
	DeviceID     string `json:"device_id"`
	DeviceOrigin string `json:"device_origin"`
	StartedAtMs  int64  `json:"started_at_ms"`
	FinishedAtMs int64  `json:"finished_at_ms,omitzero"`
	Status       string `json:"status"`
	Result       string `json:"result,omitempty"`
}

// NewJobInstanceFromJobInstance converts an internal job instance to an JobInstance.
func (converter *Converter[D, W, C]) NewJobInstanceFromJobInstance(jobInstance jobs.JobInstance) JobInstance {
	return JobInstance{
		ID:           jobInstance.ID,
		JobID:        jobInstance.JobID,
		DeviceID:     jobInstance.DeviceID,
		DeviceOrigin: jobInstance.DeviceOrigin,
		StartedAtMs:  timeToMs(jobInstance.StartedAt),
		FinishedAtMs: timeToMs(jobInstance.FinishedAt),
		Status:       jobInstance.Status.String(),
		Result:       jobInstance.Result,
	}
}

// NewJobInstancesFromJobInstances converts a slice of internal job instances to JobInstances.
func (converter *Converter[D, W, C]) NewJobInstancesFromJobInstances(jobInstances []jobs.JobInstance) []JobInstance {
	apiJobInstances := make([]JobInstance, len(jobInstances))
	for idx, jobInstance := range jobInstances {
		apiJobInstances[idx] = converter.NewJobInstanceFromJobInstance(jobInstance)
	}
	return apiJobInstances
}
