package handlers

import "sync"

// BaseHandler provides goroutine lifecycle tracking for WebSocket handlers.
// Handlers embed this to track active handler goroutines. During shutdown,
// Wait blocks until all tracked goroutines have exited.
type BaseHandler struct {
	wg sync.WaitGroup
}

// RunInBackground runs fn in a new goroutine and tracks it.
func (h *BaseHandler) RunInBackground(fn func()) {
	h.wg.Go(fn)
}

// PreventShutdown prevents Wait from returning until the returned function
// is called. Callers should defer the returned function to ensure shutdown
// is unblocked when the handler completes.
func (h *BaseHandler) PreventShutdown() func() {
	h.wg.Add(1)
	return h.wg.Done
}

// Wait blocks until all tracked goroutines have exited.
func (h *BaseHandler) Wait() {
	h.wg.Wait()
}
