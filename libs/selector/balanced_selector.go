// Package selector implements worker selection and load balancing strategies.
package selector

import (
	"errors"
	"time"
)

// Scoring constants control the penalty applied to recently-selected devices.
const (
	IncreasedScoreTotalPeriod  = 10 * time.Minute
	IncreasedScoreBucketPeriod = 30 * time.Second
	IncreasedScoreBuckets      = IncreasedScoreTotalPeriod / IncreasedScoreBucketPeriod
)

// BalancedSelector implements a weight-aware worker selection strategy.
type BalancedSelector[T MITMWorker] struct {
	*baseSelector[T]
}

// GetAvailableWorker selects a worker using a capacity-proportional load balancing strategy.
func (bs *BalancedSelector[T]) GetAvailableWorker(weight int) (T, error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if weight < MinimumWeight {
		weight = DefaultWeight
	} else if weight > MaximumWeight {
		weight = MaximumWeight
	}

	now := bs.getCurrentTime()

	// Calculate total capacity across all eligible devices
	totalCapacity := 0
	totalCurrentWeight := 0
	eligibleDevices := make([]*selectableDevice[T], 0, len(bs.devices))

	for _, selDevice := range bs.devices {
		if len(selDevice.availableWorkers) == 0 {
			continue
		}

		if !selDevice.selectionEnabled {
			continue
		}

		if selDevice.IsRateLimited(now) {
			continue
		}

		// device capacity is based on the number of workers
		// we've seen, not the number currently connected.
		totalCapacity += len(selDevice.workerIDsSeen) * MaximumWeight
		totalCurrentWeight += selDevice.currentWeight
		eligibleDevices = append(eligibleDevices, selDevice)
	}

	if len(eligibleDevices) == 0 {
		var zeroWorker T
		return zeroWorker, errors.New("no workers available")
	}
	if len(eligibleDevices) == 1 {
		selWorker := bs.claimWorkerFromDevice(eligibleDevices[0], weight, now)
		return selWorker.worker, nil
	}

	var bestDevice *selectableDevice[T]
	bestScore := float64(^uint(0) >> 1) // Max float64
	projectedTotalWeight := totalCurrentWeight + weight

	// Find the device that would have the best load distribution after assignment
	for _, device := range eligibleDevices {
		deviceCapacity := len(device.workerIDsSeen) * MaximumWeight

		// projected ratio of deviceWeight:deviceCapacity, if the worker would be assigned to this device.
		projectedDeviceWeightRatio := float64(device.currentWeight+weight) / float64(deviceCapacity)
		// ideal ratio of deviceWeight:deviceCapacity
		idealDeviceWeightRatio := ((float64(deviceCapacity) / float64(totalCapacity)) * float64(projectedTotalWeight)) / float64(deviceCapacity)

		// keep score ~0-100
		score := (100 * projectedDeviceWeightRatio) / idealDeviceWeightRatio

		// avoid selecting the same device repeatedly if the device's workers are cycling (bad device).
		timeSinceLastSelection := now.Sub(device.lastSelectionTime)
		if timeSinceLastSelection < IncreasedScoreTotalPeriod {
			// e.g. timeSinceLastSection = 1.5min, which is < 10min
			// IncreasedScoreBuckets = 20
			// multiplier = 20 - (90s / 30s) == 20 - 3 = 17
			// (multiplier will be 1 to IncreasedScoreBuckets, higher for more recent selection time)
			multiplier := IncreasedScoreBuckets - (timeSinceLastSelection / IncreasedScoreBucketPeriod)
			score = score * 10 * float64(multiplier)
		}
		if bestDevice == nil || score < bestScore {
			bestDevice = device
			bestScore = score
		}
	}

	if bestDevice == nil {
		var zeroWorker T
		return zeroWorker, errors.New("no workers available")
	}

	// Claim the selected worker and record selection
	selWorker := bs.claimWorkerFromDevice(bestDevice, weight, now)

	return selWorker.worker, nil
}

// NewBalancedSelector creates a new BalancedSelector instance.
func NewBalancedSelector[T MITMWorker](cfg Config) *BalancedSelector[T] {
	baseSelector := newBaseSelector[T](cfg)
	return &BalancedSelector[T]{
		baseSelector: baseSelector,
	}
}
