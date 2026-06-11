package stats

import (
	"sync"
	"time"
)

type bucket[S TimedWindowedStat] struct {
	sum   S
	count uint64
}

// CountDurationCollector tracks event counts and their durations in a single
// structure with one mutex. This guarantees that Add and GetWindows operate on
// both metrics atomically with a consistent timestamp.
type CountDurationCollector[S TimedWindowedStat] struct {
	mu sync.Mutex

	bucketDuration time.Duration
	maxWindow      time.Duration

	countBuckets    []bucket[S]
	durationBuckets []bucket[S]
	head            int
	headTime        time.Time
	createdAt       time.Time
}

// NewCountDurationCollector creates a collector that tracks both event counts and
// durations. Both metrics share the same ring buffer geometry and lock.
func NewCountDurationCollector[S TimedWindowedStat](maxWindow, bucketDuration time.Duration) *CountDurationCollector[S] {
	if bucketDuration == 0 {
		return nil
	}
	if maxWindow%bucketDuration != 0 {
		return nil
	}

	numBuckets := 1 + int(maxWindow/bucketDuration)
	now := time.Now()

	return &CountDurationCollector[S]{
		bucketDuration:  bucketDuration,
		maxWindow:       maxWindow,
		countBuckets:    make([]bucket[S], numBuckets),
		durationBuckets: make([]bucket[S], numBuckets),
		head:            0,
		headTime:        now,
		createdAt:       now,
	}
}

// advance rotates both ring buffers forward. Caller must hold mu.
func (c *CountDurationCollector[S]) advance(now time.Time) {
	elapsed := now.Sub(c.headTime)
	bucketsToAdvance := int(elapsed / c.bucketDuration)

	if bucketsToAdvance == 0 {
		return
	}

	numBuckets := len(c.countBuckets)
	if bucketsToAdvance >= numBuckets {
		for i := range c.countBuckets {
			c.countBuckets[i] = bucket[S]{}
			c.durationBuckets[i] = bucket[S]{}
		}
		c.headTime = c.headTime.Add(time.Duration(bucketsToAdvance) * c.bucketDuration)
		c.head = 0
		return
	}

	for range bucketsToAdvance {
		c.head = (c.head + 1) % numBuckets
		c.countBuckets[c.head] = bucket[S]{}
		c.durationBuckets[c.head] = bucket[S]{}
		c.headTime = c.headTime.Add(c.bucketDuration)
	}
}

// Add records a count and duration into the current time bucket under one lock.
func (c *CountDurationCollector[S]) Add(count, duration S) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.advance(time.Now())
	c.countBuckets[c.head].sum += count
	c.countBuckets[c.head].count++
	c.durationBuckets[c.head].sum += duration
	c.durationBuckets[c.head].count++
}

// CountDurationWindows holds count and duration stats across time windows
// from a single atomic snapshot.
type CountDurationWindows[S TimedWindowedStat] struct {
	Counts    []TimeWindowedStatsPeriod[S]
	Durations []TimeWindowedStatsPeriod[S]
}

// GetWindows returns aggregated count and duration stats across all requested
// window durations in a single lock acquisition with one consistent timestamp.
func (c *CountDurationCollector[S]) GetWindows(windows ...time.Duration) CountDurationWindows[S] {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.advance(now)

	numBuckets := len(c.countBuckets)
	counts := make([]TimeWindowedStatsPeriod[S], len(windows))
	durations := make([]TimeWindowedStatsPeriod[S], len(windows))

	for i, window := range windows {
		bucketsToScan := min(int(window/c.bucketDuration), numBuckets)

		var countSum, durationSum S
		var countN, durationN uint64

		for j := range bucketsToScan {
			idx := (c.head - j + numBuckets) % numBuckets
			countSum += c.countBuckets[idx].sum
			countN += c.countBuckets[idx].count
			durationSum += c.durationBuckets[idx].sum
			durationN += c.durationBuckets[idx].count
		}

		startTime := now.Add(-window)
		if startTime.Before(c.createdAt) {
			startTime = c.createdAt
		}
		counts[i] = TimeWindowedStatsPeriod[S]{
			TimeWindowedStats: TimeWindowedStats[S]{sum: countSum, count: countN},
			startTime:         startTime,
			endTime:           now,
		}
		durations[i] = TimeWindowedStatsPeriod[S]{
			TimeWindowedStats: TimeWindowedStats[S]{sum: durationSum, count: durationN},
			startTime:         startTime,
			endTime:           now,
		}
	}

	return CountDurationWindows[S]{Counts: counts, Durations: durations}
}
