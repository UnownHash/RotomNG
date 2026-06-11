// Package factories provides factory functions for creating connection types.
package factories

import (
	"github.com/UnownHash/RotomNG/libs/connections"
	"github.com/UnownHash/RotomNG/libs/controller"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/protos"
)

// Controller is an alias for the controller.Controller type.
type Controller = controller.Controller

// MITMWorker is an alias for the mitm.Worker type.
type MITMWorker = mitm.Worker

// NewControllerFunc is the function signature for creating controllers.
type NewControllerFunc = connections.NewControllerFunc[*Controller]

// NewControllerFactory returns a factory function that creates new Controller instances.
func NewControllerFactory() NewControllerFunc {
	return func(
		wsConn connections.ControllerWSConn,
		id string,
		mitmLoginRequest *protos.MitmRequest,
		mitmWorker connections.MITMWorker,
		weight int,
		userAgent string,
		disableWorkerStats bool,
		protoMajorVersion int,
		protoMinorVersion int,
	) *Controller {
		return controller.NewController(
			wsConn,
			id,
			mitmLoginRequest,
			mitmWorker,
			weight,
			userAgent,
			disableWorkerStats,
			protoMajorVersion,
			protoMinorVersion,
		)
	}
}
