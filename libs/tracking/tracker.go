// Package tracking provides a generic, concurrency-safe request tracker
// for correlating outbound requests with their responses.
package tracking

import (
	"sync"
	"time"
)

// RequestIndex is a constraint for types that can serve as request map keys.
type RequestIndex interface {
	comparable
}

// Request holds metadata for a tracked in-flight request.
// The type parameter D carries caller-defined data alongside the request.
type Request[D any] struct {
	StartTime  time.Time
	MethodName string
	Data       D
}

// RequestTracker is a concurrency-safe map of in-flight requests keyed by index type I.
// The type parameter D allows callers to attach arbitrary data to each request.
type RequestTracker[I RequestIndex, D any] struct {
	mu       sync.Mutex
	requests map[I]Request[D]
}

func (tracker *RequestTracker[I, D]) extractRequests() map[I]Request[D] {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	requests := tracker.requests
	tracker.requests = nil
	return requests
}

// Add stores a request under the given index, replacing any existing entry.
func (tracker *RequestTracker[I, D]) Add(index I, request Request[D]) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if tracker.requests == nil {
		return
	}
	tracker.requests[index] = request
}

// Get retrieves and removes the request for the given index.
// Returns the request and true if found, or the zero value and false otherwise.
func (tracker *RequestTracker[I, D]) Get(index I) (Request[D], bool) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if tracker.requests == nil {
		return Request[D]{}, false
	}
	request, ok := tracker.requests[index]
	if ok {
		delete(tracker.requests, index)
	}
	return request, ok
}

// Peak retrieves the request for the given index without removing it.
// Returns the request and true if found, or the zero value and false otherwise.
func (tracker *RequestTracker[I, D]) Peak(index I) (Request[D], bool) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	if tracker.requests == nil {
		return Request[D]{}, false
	}
	request, ok := tracker.requests[index]
	return request, ok
}

// Done drains all tracked requests and calls fn for each one.
// If fn is nil, the requests are silently discarded. Any attempt to
// Add() after this will be a no-op and all Get() and Peak()s will return
// false.
func (tracker *RequestTracker[I, D]) Done(fn func(request Request[D])) {
	requests := tracker.extractRequests()
	if fn == nil || requests == nil {
		return
	}
	for _, request := range requests {
		fn(request)
	}
}

// NewRequestTracker creates an empty RequestTracker ready for use.
func NewRequestTracker[I RequestIndex, D any]() *RequestTracker[I, D] {
	return &RequestTracker[I, D]{
		requests: make(map[I]Request[D]),
	}
}
