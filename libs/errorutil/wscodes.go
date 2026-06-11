package errorutil

import "github.com/UnownHash/RotomNG/libs/ws"

// WebSocket close codes for application-level connection termination reasons.
const (
	CloseCodeMITMWorkerDisconnected = 3000
	CloseCodeNoMITMWorkersAvailable = 3001
	CloseCodeRestartSession         = 3002
	CloseCodeKillSession            = 3003
	CloseCodeSessionExpired         = 3004
	CloseCodeShuttingDown           = ws.StatusGoingAway
	CloseCodeProtocolError          = ws.StatusProtocolError
	CloseCodeInternalServerError    = ws.StatusInternalServerError
)
