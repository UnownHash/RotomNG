package handlers

var _ WorkerStatsCollector = noOpStatsCollector{}

type noOpStatsCollector struct{}

//nolint:revive // unexported-return: noOpStatsCollector implements WorkerStatsCollector interface
func NewNoOpStatsCollector() noOpStatsCollector {
	return noOpStatsCollector{}
}

func (n noOpStatsCollector) IncrWorkerAccepts()           {}
func (n noOpStatsCollector) IncrWorkerAcceptFails()       {}
func (n noOpStatsCollector) IncrWorkerRegistrationFails() {}
func (n noOpStatsCollector) IncrWorkersConnected(string)  {}
func (n noOpStatsCollector) DecrWorkersConnected(string)  {}
