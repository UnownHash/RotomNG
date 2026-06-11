package settings

import (
	"errors"
	"testing"
)

type validatableSettings struct {
	Value    int
	FailNext bool
}

func (s validatableSettings) Validate() error {
	if s.FailNext {
		return errors.New("validation failed")
	}
	return nil
}

func TestNewContainer_ValidationFailure(t *testing.T) {
	_, err := NewContainer(validatableSettings{FailNext: true})
	if err == nil {
		t.Fatal("expected error for invalid settings")
	}
}

func TestPutSettings_ValidationFailure(t *testing.T) {
	container, err := NewContainer(validatableSettings{Value: 1})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}

	err = container.PutSettings(validatableSettings{FailNext: true})
	if err == nil {
		t.Fatal("expected error for invalid settings")
	}

	// Original settings should be unchanged
	if container.GetSettings().Value != 1 {
		t.Error("settings should not have changed after validation failure")
	}
}

func TestNotify_ReceivesUpdates(t *testing.T) {
	container, err := NewContainer(validatableSettings{Value: 1})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}

	var received []int
	dereg := container.Notify(func(s validatableSettings) {
		received = append(received, s.Value)
	})

	container.PutSettings(validatableSettings{Value: 10})
	container.PutSettings(validatableSettings{Value: 20})

	if len(received) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(received))
	}
	if received[0] != 10 || received[1] != 20 {
		t.Errorf("received = %v, want [10, 20]", received)
	}

	// Deregister and verify no more notifications
	dereg()

	container.PutSettings(validatableSettings{Value: 30})
	if len(received) != 2 {
		t.Errorf("expected no more notifications after dereg, got %d", len(received))
	}
}

func TestNotify_MultipleWatchers(t *testing.T) {
	container, err := NewContainer(validatableSettings{Value: 1})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}

	calls1 := 0
	calls2 := 0

	dereg1 := container.Notify(func(_ validatableSettings) { calls1++ })
	dereg2 := container.Notify(func(_ validatableSettings) { calls2++ })

	container.PutSettings(validatableSettings{Value: 5})

	if calls1 != 1 || calls2 != 1 {
		t.Errorf("expected both watchers called once, got %d and %d", calls1, calls2)
	}

	dereg1()
	container.PutSettings(validatableSettings{Value: 6})

	if calls1 != 1 {
		t.Errorf("expected watcher1 not called after dereg, got %d", calls1)
	}
	if calls2 != 2 {
		t.Errorf("expected watcher2 called twice, got %d", calls2)
	}

	dereg2()
}
