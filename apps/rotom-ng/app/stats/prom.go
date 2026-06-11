// Package stats provides Prometheus-based metrics collection for RotomNG.
package stats

import (
	"net/http"
	"time"

	"github.com/Depado/ginprom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/UnownHash/RotomNG/libs/handlers"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/prom"
)

// Prometheus label name constants.
const (
	labelOrigin  = "origin"
	labelCommand = "command"
	labelMethod  = "method"
)

// Collector defines the interface for collecting application metrics.
type Collector interface {
	// HTTP service metrics
	GetMetricsHandler() http.Handler
	mitm.DeviceStatsCollector
	mitm.WorkerStatsCollector
	handlers.ControllerStatsCollector

	// Application lifecycle metrics
	IncrAppStartups(version string)
	IncrConfigReloads(version string)
}

var _ Collector = (*PromStatsCollector)(nil)

// PromStatsCollector implements StatsCollector using Prometheus metrics.
type PromStatsCollector struct {
	namespace string
	registry  *prometheus.Registry

	devicesConnected *prometheus.GaugeVec
	devicesTotal     *prometheus.GaugeVec

	deviceMemoryFree  *prometheus.GaugeVec
	deviceMemoryMitm  *prometheus.GaugeVec
	deviceMemoryStart *prometheus.GaugeVec

	deviceCommandsExecuted *prometheus.CounterVec
	deviceCommandsSuccess  *prometheus.CounterVec
	deviceCommandsError    *prometheus.CounterVec

	// Worker metrics
	workersConnected        *prometheus.GaugeVec
	workersInUse            *prometheus.GaugeVec
	workerRequests          *prometheus.CounterVec
	workerDroppedResponses  prometheus.Counter
	workerResponses         *prometheus.CounterVec
	workerResponseDuration  *prometheus.HistogramVec
	workerRegistrationFails prometheus.Counter
	workerRegistrations     *prometheus.CounterVec

	// Device registration metrics
	deviceRegistrationFails prometheus.Counter
	deviceRegistrations     *prometheus.CounterVec

	// Connection handler metrics
	deviceControlAccepts     prometheus.Counter
	deviceControlAcceptFails prometheus.Counter
	workerAccepts            prometheus.Counter
	workerAcceptFails        prometheus.Counter

	// Controller handler metrics
	controllerAccepts     prometheus.Counter
	controllerAcceptFails prometheus.Counter
	controllerConnections *prometheus.GaugeVec

	// RPC metrics
	rpcRequests        prometheus.Counter
	rpcRequestDuration prometheus.Histogram

	// Application lifecycle metrics
	appStartups   *prometheus.CounterVec
	configReloads *prometheus.CounterVec
}

// NewPromStatsCollector creates a new PromStatsCollector instance with registered metrics.
func NewPromStatsCollector(namespace string) *PromStatsCollector {
	registry := prometheus.NewRegistry()

	workersConnected := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "workers_connected",
		Help:      "Number of workers connected",
	}, []string{labelOrigin})

	workersInUse := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "workers_in_use",
		Help:      "Number of workers in use",
	}, []string{labelOrigin})

	devicesConnected := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "devices_connected",
		Help:      "Number of devices with active connections",
	}, []string{labelOrigin})

	devicesTotal := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "devices_total",
		Help:      "Devices known regardless of state",
	}, []string{labelOrigin})

	deviceMemoryFree := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "device_memory_free",
		Help:      "Device free memory in bytes",
	}, []string{labelOrigin})

	deviceMemoryMitm := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "device_memory_mitm",
		Help:      "Device MITM memory usage in bytes",
	}, []string{labelOrigin})

	deviceMemoryStart := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "device_memory_start",
		Help:      "Device start memory in bytes",
	}, []string{labelOrigin})

	deviceCommandsExecuted := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_commands_executed_total",
		Help:      "Total number of device commands executed",
	}, []string{labelOrigin, labelCommand})

	deviceCommandsSuccess := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_commands_success_total",
		Help:      "Total number of successful device commands",
	}, []string{labelOrigin, labelCommand})

	deviceCommandsError := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_commands_error_total",
		Help:      "Total number of failed device commands",
	}, []string{labelOrigin, labelCommand})

	// Worker metrics
	workerRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_requests_total",
		Help:      "Total number of worker requests",
	}, []string{labelMethod})

	workerDroppedResponses := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_dropped_responses_total",
		Help:      "Total number of dropped worker responses",
	})

	workerResponses := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_responses_total",
		Help:      "Total number of worker responses",
	}, []string{labelMethod, "status", "error"})

	workerResponseDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "worker_response_duration_seconds",
		Help:      "Worker response duration in seconds",
	}, []string{labelMethod, "status"})

	// Device registration metrics
	deviceRegistrationFails := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_registration_fails_total",
		Help:      "Total number of device registration failures",
	})

	deviceRegistrations := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_registrations_total",
		Help:      "Total number of device registrations",
	}, []string{labelOrigin})

	// Worker registration metrics
	workerRegistrationFails := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_registration_fails_total",
		Help:      "Total number of worker registration failures",
	})

	workerRegistrations := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_registrations_total",
		Help:      "Total number of worker registrations",
	}, []string{labelOrigin})

	// Connection handler metrics
	deviceControlAccepts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_control_accepts_total",
		Help:      "Total number of device control accepts",
	})

	deviceControlAcceptFails := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "device_control_accept_fails_total",
		Help:      "Total number of device control accept failures",
	})

	workerAccepts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_accepts_total",
		Help:      "Total number of worker accepts",
	})

	workerAcceptFails := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "worker_accept_fails_total",
		Help:      "Total number of worker accept failures",
	})

	// Controller handler metrics
	controllerAccepts := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "controller_accepts_total",
		Help:      "Total number of controller accepts",
	})

	controllerAcceptFails := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "controller_accept_fails_total",
		Help:      "Total number of controller accept failures",
	})

	controllerConnections := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "controller_connections",
		Help:      "Number of controller connections by user agent",
	}, []string{"user_agent"})

	// RPC metrics
	rpcRequests := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rpc_requests_total",
		Help:      "Total number of RPC requests",
	})

	rpcRequestDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "rpc_request_duration_seconds",
		Help:      "RPC request duration in seconds",
	})

	// Application lifecycle metrics
	appStartups := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "app_startups_total",
		Help:      "Total number of application startups",
	}, []string{"version"})

	configReloads := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "config_reloads_total",
		Help:      "Total number of successful config reloads",
	}, []string{"version"})

	// Register Go runtime metrics collectors
	registry.MustRegister(prom.NewNamespaceCollector(namespace, collectors.NewGoCollector()))
	registry.MustRegister(collectors.NewProcessCollector(
		collectors.ProcessCollectorOpts{
			Namespace: namespace,
		},
	))

	// Register metrics with the registry
	registry.MustRegister(workersConnected)
	registry.MustRegister(workersInUse)
	registry.MustRegister(devicesConnected)
	registry.MustRegister(devicesTotal)
	registry.MustRegister(deviceMemoryFree)
	registry.MustRegister(deviceMemoryMitm)
	registry.MustRegister(deviceMemoryStart)
	registry.MustRegister(deviceCommandsExecuted)
	registry.MustRegister(deviceCommandsSuccess)
	registry.MustRegister(deviceCommandsError)
	registry.MustRegister(workerRequests)
	registry.MustRegister(workerDroppedResponses)
	registry.MustRegister(workerResponses)
	registry.MustRegister(workerResponseDuration)
	registry.MustRegister(deviceRegistrationFails)
	registry.MustRegister(deviceRegistrations)
	registry.MustRegister(workerRegistrationFails)
	registry.MustRegister(workerRegistrations)
	registry.MustRegister(deviceControlAccepts)
	registry.MustRegister(deviceControlAcceptFails)
	registry.MustRegister(workerAccepts)
	registry.MustRegister(workerAcceptFails)
	registry.MustRegister(controllerAccepts)
	registry.MustRegister(controllerAcceptFails)
	registry.MustRegister(controllerConnections)
	registry.MustRegister(rpcRequests)
	registry.MustRegister(rpcRequestDuration)
	registry.MustRegister(appStartups)
	registry.MustRegister(configReloads)

	return &PromStatsCollector{
		namespace:                namespace,
		registry:                 registry,
		workersConnected:         workersConnected,
		workersInUse:             workersInUse,
		devicesConnected:         devicesConnected,
		devicesTotal:             devicesTotal,
		deviceMemoryFree:         deviceMemoryFree,
		deviceMemoryMitm:         deviceMemoryMitm,
		deviceMemoryStart:        deviceMemoryStart,
		deviceCommandsExecuted:   deviceCommandsExecuted,
		deviceCommandsSuccess:    deviceCommandsSuccess,
		deviceCommandsError:      deviceCommandsError,
		workerRequests:           workerRequests,
		workerDroppedResponses:   workerDroppedResponses,
		workerResponses:          workerResponses,
		workerResponseDuration:   workerResponseDuration,
		deviceRegistrationFails:  deviceRegistrationFails,
		deviceRegistrations:      deviceRegistrations,
		workerRegistrationFails:  workerRegistrationFails,
		workerRegistrations:      workerRegistrations,
		deviceControlAccepts:     deviceControlAccepts,
		deviceControlAcceptFails: deviceControlAcceptFails,
		workerAccepts:            workerAccepts,
		workerAcceptFails:        workerAcceptFails,
		controllerAccepts:        controllerAccepts,
		controllerAcceptFails:    controllerAcceptFails,
		controllerConnections:    controllerConnections,
		rpcRequests:              rpcRequests,
		rpcRequestDuration:       rpcRequestDuration,
		appStartups:              appStartups,
		configReloads:            configReloads,
	}
}

// RegisterGinEngine registers Prometheus instrumentation middleware on a Gin engine.
func (sc *PromStatsCollector) RegisterGinEngine(ginEngine *gin.Engine) {
	pgin := ginprom.New(
		ginprom.Registry(sc.registry),
		ginprom.Namespace(sc.namespace),
		ginprom.Subsystem("gin"),
	)
	ginEngine.Use(pgin.Instrument())
}

// IncrDevicesTotal increments the devices_total gauge for the given origin.
func (sc *PromStatsCollector) IncrDevicesTotal(origin string) {
	sc.devicesTotal.WithLabelValues(origin).Inc()
}

// DecrDevicesTotal decrements the devices_total gauge for the given origin.
func (sc *PromStatsCollector) DecrDevicesTotal(origin string, count int) {
	sc.devicesTotal.WithLabelValues(origin).Sub(float64(count))
}

// IncrDevicesConnected increments the devices_connected gauge for the given origin.
func (sc *PromStatsCollector) IncrDevicesConnected(origin string) {
	sc.devicesConnected.WithLabelValues(origin).Inc()
}

// DecrDevicesConnected decrements the devices_connected gauge for the given origin.
func (sc *PromStatsCollector) DecrDevicesConnected(origin string) {
	sc.devicesConnected.WithLabelValues(origin).Dec()
}

// SetDeviceMemoryFree sets the device_memory_free gauge for the given origin.
func (sc *PromStatsCollector) SetDeviceMemoryFree(origin string, value float64) {
	sc.deviceMemoryFree.WithLabelValues(origin).Set(value)
}

// SetDeviceMemoryMITM sets the device_memory_mitm gauge for the given origin.
func (sc *PromStatsCollector) SetDeviceMemoryMITM(origin string, value float64) {
	sc.deviceMemoryMitm.WithLabelValues(origin).Set(value)
}

// SetDeviceMemoryStart sets the device_memory_start gauge for the given origin.
func (sc *PromStatsCollector) SetDeviceMemoryStart(origin string, value float64) {
	sc.deviceMemoryStart.WithLabelValues(origin).Set(value)
}

// IncrDeviceCommandExecuted increments the device_commands_executed_total counter.
func (sc *PromStatsCollector) IncrDeviceCommandExecuted(origin string, command string) {
	sc.deviceCommandsExecuted.WithLabelValues(origin, command).Inc()
}

// IncrDeviceCommandSuccess increments the device_commands_success_total counter.
func (sc *PromStatsCollector) IncrDeviceCommandSuccess(origin string, command string) {
	sc.deviceCommandsSuccess.WithLabelValues(origin, command).Inc()
}

// IncrDeviceCommandError increments the device_commands_error_total counter.
func (sc *PromStatsCollector) IncrDeviceCommandError(origin string, command string) {
	sc.deviceCommandsError.WithLabelValues(origin, command).Inc()
}

// IncrWorkerRequests increments the worker requests counter for the given method.
func (sc *PromStatsCollector) IncrWorkerRequests(method string) {
	sc.workerRequests.WithLabelValues(method).Inc()
}

// IncrWorkerDroppedResponses increments the dropped worker responses counter.
func (sc *PromStatsCollector) IncrWorkerDroppedResponses() {
	sc.workerDroppedResponses.Inc()
}

// IncrWorkerResponses increments the worker responses counter and records duration.
func (sc *PromStatsCollector) IncrWorkerResponses(duration time.Duration, method string, status string, errStr string) {
	sc.workerResponses.WithLabelValues(method, status, errStr).Inc()
	sc.workerResponseDuration.WithLabelValues(method, status).Observe(duration.Seconds())
}

// IncrDeviceRegistrationFails increments the device registration failures counter.
func (sc *PromStatsCollector) IncrDeviceRegistrationFails() {
	sc.deviceRegistrationFails.Inc()
}

// IncrDeviceRegistrations increments the device registrations counter for the given origin.
func (sc *PromStatsCollector) IncrDeviceRegistrations(origin string) {
	sc.deviceRegistrations.WithLabelValues(origin).Inc()
}

// IncrWorkerRegistrationFails increments the worker registration failures counter.
func (sc *PromStatsCollector) IncrWorkerRegistrationFails() {
	sc.workerRegistrationFails.Inc()
}

// IncrWorkerRegistrations increments the worker registrations counter for the given origin.
func (sc *PromStatsCollector) IncrWorkerRegistrations(origin string) {
	sc.workerRegistrations.WithLabelValues(origin).Inc()
}

// IncrWorkersConnected increments the workers_connected gauge for the given origin.
func (sc *PromStatsCollector) IncrWorkersConnected(origin string) {
	sc.workersConnected.WithLabelValues(origin).Inc()
}

// DecrWorkersConnected decrements the workers_connected gauge for the given origin.
func (sc *PromStatsCollector) DecrWorkersConnected(origin string) {
	sc.workersConnected.WithLabelValues(origin).Dec()
}

// IncrWorkersInUse increments the workers_in_use gauge for the given origin.
func (sc *PromStatsCollector) IncrWorkersInUse(origin string) {
	sc.workersInUse.WithLabelValues(origin).Inc()
}

// DecrWorkersInUse decrements the workers_in_use gauge for the given origin.
func (sc *PromStatsCollector) DecrWorkersInUse(origin string) {
	sc.workersInUse.WithLabelValues(origin).Dec()
}

// IncrDeviceControlAccepts increments the device control accepts counter.
func (sc *PromStatsCollector) IncrDeviceControlAccepts() {
	sc.deviceControlAccepts.Inc()
}

// IncrDeviceControlAcceptFails increments the device control accept failures counter.
func (sc *PromStatsCollector) IncrDeviceControlAcceptFails() {
	sc.deviceControlAcceptFails.Inc()
}

// IncrWorkerAccepts increments the worker accepts counter.
func (sc *PromStatsCollector) IncrWorkerAccepts() {
	sc.workerAccepts.Inc()
}

// IncrWorkerAcceptFails increments the worker accept failures counter.
func (sc *PromStatsCollector) IncrWorkerAcceptFails() {
	sc.workerAcceptFails.Inc()
}

// IncrControllerAccepts increments the controller accepts counter.
func (sc *PromStatsCollector) IncrControllerAccepts() {
	sc.controllerAccepts.Inc()
}

// IncrControllerAcceptFails increments the controller accept failures counter.
func (sc *PromStatsCollector) IncrControllerAcceptFails() {
	sc.controllerAcceptFails.Inc()
}

// IncrControllerConnections increments the controller connections gauge for the given user agent.
func (sc *PromStatsCollector) IncrControllerConnections(userAgent string) {
	sc.controllerConnections.WithLabelValues(userAgent).Inc()
}

// DecrControllerConnections decrements the controller connections gauge for the given user agent.
func (sc *PromStatsCollector) DecrControllerConnections(userAgent string) {
	sc.controllerConnections.WithLabelValues(userAgent).Dec()
}

// IncrRPCRequests increments the RPC requests counter and records duration.
func (sc *PromStatsCollector) IncrRPCRequests(duration time.Duration) {
	sc.rpcRequests.Inc()
	sc.rpcRequestDuration.Observe(duration.Seconds())
}

// IncrAppStartups increments the application startups counter for the given version.
func (sc *PromStatsCollector) IncrAppStartups(version string) {
	sc.appStartups.WithLabelValues(version).Inc()
}

// IncrConfigReloads increments the config reloads counter for the given version.
func (sc *PromStatsCollector) IncrConfigReloads(version string) {
	sc.configReloads.WithLabelValues(version).Inc()
}

// GetMetricsHandler returns an HTTP handler for the /api/metrics endpoint.
func (sc *PromStatsCollector) GetMetricsHandler() http.Handler {
	return promhttp.HandlerFor(sc.registry, promhttp.HandlerOpts{})
}
