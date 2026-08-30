package handlers

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestBaseHandlerRunInBackgroundIsWaitedOn covers the contract shutdown relies
// on: a goroutine started through the handler is tracked, so Wait cannot return
// while it is still running.
func TestBaseHandlerRunInBackgroundIsWaitedOn(t *testing.T) {
	var handler BaseHandler

	var finished atomic.Bool
	release := make(chan struct{})

	handler.RunInBackground(func() {
		<-release
		finished.Store(true)
	})

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		handler.Wait()
	}()

	select {
	case <-waited:
		t.Fatal("Wait returned while a background goroutine was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the background goroutine finished")
	}
	if !finished.Load() {
		t.Error("Wait returned before the goroutine's work was visible")
	}
}

// TestBaseHandlerPreventShutdown covers the other half: a handler can hold
// shutdown open across work it does not run in a goroutine of its own.
func TestBaseHandlerPreventShutdown(t *testing.T) {
	var handler BaseHandler

	done := handler.PreventShutdown()

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		handler.Wait()
	}()

	select {
	case <-waited:
		t.Fatal("Wait returned while shutdown was being prevented")
	case <-time.After(50 * time.Millisecond):
	}

	done()

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after shutdown was released")
	}
}

// TestBaseHandlerWaitWithNothingRunning guards the trivial case: an idle
// handler must not block a shutdown.
func TestBaseHandlerWaitWithNothingRunning(t *testing.T) {
	var handler BaseHandler

	waited := make(chan struct{})
	go func() {
		defer close(waited)
		handler.Wait()
	}()

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait blocked with no tracked goroutines")
	}
}
