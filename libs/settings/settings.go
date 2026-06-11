// Package settings provides a generic, thread-safe container for validated configuration values.
package settings

import (
	"sync"
	"sync/atomic"
)

type settingsContainer interface {
	Validate() error
}

// Container holds a validated settings value with atomic access and change notifications.
type Container[T settingsContainer] struct {
	ptr      atomic.Pointer[T]
	mu       sync.Mutex
	watchers map[*func(T)]struct{}
}

// GetSettings returns the current settings value.
func (s *Container[T]) GetSettings() T {
	settings := s.ptr.Load()
	return *settings
}

// PutSettings validates and stores new settings, notifying all registered watchers.
func (s *Container[T]) PutSettings(settings T) error {
	if err := settings.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ptr.Store(&settings)
	for fn := range s.watchers {
		(*fn)(settings)
	}

	return nil
}

// Notify registers a callback invoked on settings changes and returns a deregistration function.
func (s *Container[T]) Notify(fn func(T)) (dereg func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.watchers[&fn] = struct{}{}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		delete(s.watchers, &fn)
	}
}

// NewContainer creates a new Container with the given initial settings.
func NewContainer[T settingsContainer](settings T) (*Container[T], error) {
	s := &Container[T]{
		watchers: make(map[*func(T)]struct{}),
	}
	if err := s.PutSettings(settings); err != nil {
		return nil, err
	}
	return s, nil
}
