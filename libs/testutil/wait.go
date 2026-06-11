package testutil

import (
	"fmt"
	"time"
)

// WaitForCondition polls fn at ~25ms intervals until it returns true or
// the timeout expires. Returns nil on success, or a non-nil error if the
// condition was not met within the given timeout.
func WaitForCondition(fn func() bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met within %s", timeout)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
