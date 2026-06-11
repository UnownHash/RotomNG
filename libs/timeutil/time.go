// Package timeutil provides time-related utility functions.
package timeutil

import (
	"context"
	"time"
)

// SleepContext behaves like time.Sleep, but can be interrupted
// by cancelling the supplied Context. Returns true if context
// was cancelled.
func SleepContext(ctx context.Context, dur time.Duration) (cancelled bool) {
	timer := time.NewTimer(dur)
	defer func() {
		if cancelled && !timer.Stop() {
			<-timer.C
		}
	}()
	select {
	case <-ctx.Done():
		cancelled = true
	case <-timer.C:
	}
	return
}
