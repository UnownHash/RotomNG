package handlers

import "testing"

func TestNoOpStatsCollector(_ *testing.T) {
	sc := NewNoOpStatsCollector()

	// Verify it implements WorkerStatsCollector interface
	var _ WorkerStatsCollector = sc

	// Exercise every method — test passes if no panic occurs
	sc.IncrWorkerAccepts()
	sc.IncrWorkerAcceptFails()
	sc.IncrWorkerRegistrationFails()
	sc.IncrWorkersConnected("test-origin")
	sc.DecrWorkersConnected("test-origin")
}
