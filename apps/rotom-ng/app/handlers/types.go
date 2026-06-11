package handlers

import (
	"context"

	"github.com/UnownHash/RotomNG/libs/controller"
	"github.com/UnownHash/RotomNG/libs/mitm"
)

// MITMWorker is an alias for the MITM worker type.
type MITMWorker = mitm.Worker

// MITMWorkerStatsCollector is an alias for the MITM worker stats collector interface.
type MITMWorkerStatsCollector = mitm.WorkerStatsCollector

// Controller is an alias for the controller type.
type Controller = controller.Controller

// WorkerConnectionManager defines the interface for registering workers with the connection manager.
type WorkerConnectionManager interface {
	RegisterWorker(ctx context.Context, worker *MITMWorker) error
}

// WorkerStatsCollector defines the interface for collecting worker connection statistics.
type WorkerStatsCollector interface {
	IncrWorkerAccepts()
	IncrWorkerAcceptFails()
	IncrWorkerRegistrationFails()
	IncrWorkersConnected(origin string)
	DecrWorkersConnected(origin string)
}
