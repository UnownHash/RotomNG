package testutil

import (
	"testing"
	"time"
)

func TestWaitForConditionImmediateSuccess(t *testing.T) {
	err := WaitForCondition(func() bool { return true }, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error for immediate true, got: %v", err)
	}
}

func TestWaitForConditionEventualSuccess(t *testing.T) {
	count := 0
	fn := func() bool {
		count++
		return count >= 3
	}
	err := WaitForCondition(fn, 1*time.Second)
	if err != nil {
		t.Fatalf("expected nil error after eventual true, got: %v", err)
	}
	if count < 3 {
		t.Fatalf("expected fn to be called at least 3 times, got %d", count)
	}
}

func TestWaitForConditionTimeout(t *testing.T) {
	err := WaitForCondition(func() bool { return false }, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected non-nil error on timeout, got nil")
	}
}

func TestWaitForConditionTimeoutMessage(t *testing.T) {
	timeout := 50 * time.Millisecond
	err := WaitForCondition(func() bool { return false }, timeout)
	if err == nil {
		t.Fatal("expected non-nil error on timeout, got nil")
	}
	msg := err.Error()
	if got := timeout.String(); !containsString(msg, got) {
		t.Fatalf("expected error message to contain %q, got: %s", got, msg)
	}
}

// containsString is a helper to avoid importing strings in test.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
