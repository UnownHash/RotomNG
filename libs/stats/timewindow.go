// Package stats provides time-windowed statistics collection and aggregation.
package stats

import (
	"time"

	"golang.org/x/exp/constraints"
)

// TimedWindowedStat is a type constraint for numeric values that can be tracked in time windows.
type TimedWindowedStat interface {
	constraints.Integer | constraints.Float
}

// TimeWindowedStats holds the sum and count for a single stats bucket.
type TimeWindowedStats[S TimedWindowedStat] struct {
	sum   S
	count uint64
}

// TimeWindowedStatsPeriod represents aggregated stats over a specific time period.
type TimeWindowedStatsPeriod[S TimedWindowedStat] struct {
	TimeWindowedStats[S]

	startTime time.Time
	endTime   time.Time
}

// Sum returns the total sum of all values in the period.
func (st *TimeWindowedStatsPeriod[S]) Sum() S {
	return st.sum
}

// Count returns the number of values recorded in the period.
func (st *TimeWindowedStatsPeriod[S]) Count() uint64 {
	return st.count
}

// Avg returns the average value over the period, or 0 if no values were recorded.
func (st *TimeWindowedStatsPeriod[S]) Avg() float64 {
	if st.count == 0 {
		return 0
	}
	return float64(st.sum) / float64(st.count)
}

// RatePerSecond returns the sum divided by elapsed seconds in the period.
func (st *TimeWindowedStatsPeriod[S]) RatePerSecond() float64 {
	if st.count == 0 {
		return 0
	}
	seconds := st.endTime.Sub(st.startTime).Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(st.sum) / seconds
}

// StartTime returns the start of the period.
func (st *TimeWindowedStatsPeriod[S]) StartTime() time.Time {
	return st.startTime
}

// EndTime returns the end of the period.
func (st *TimeWindowedStatsPeriod[S]) EndTime() time.Time {
	return st.endTime
}
