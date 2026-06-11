// Package bufferpool provides a sync.Pool-based buffer pool with size limits.
package bufferpool

import (
	"bytes"
	"sync"
)

var globalBufferPool = New(32 * 1024)

// BufferPool is a pool of reusable byte buffers with a maximum size cap.
type BufferPool struct {
	pool    *sync.Pool
	maxSize int
}

// Get retrieves a buffer from the pool.
func (bp *BufferPool) Get() *bytes.Buffer {
	buf, ok := bp.pool.Get().(*bytes.Buffer)
	if !ok {
		return bytes.NewBuffer(make([]byte, 0, bp.maxSize))
	}
	return buf
}

// Put returns a buffer to the pool if it does not exceed the size limit.
func (bp *BufferPool) Put(buf *bytes.Buffer) {
	if bp.maxSize > 0 && buf.Cap() > bp.maxSize {
		return
	}
	buf.Reset()
	bp.pool.Put(buf)
}

// New creates a new BufferPool with the given maximum buffer size.
func New(maxSize int) *BufferPool {
	return &BufferPool{
		pool: &sync.Pool{
			New: func() any {
				return &bytes.Buffer{}
			},
		},
		maxSize: maxSize,
	}
}

// Get retrieves a buffer from the global pool.
func Get() *bytes.Buffer {
	return globalBufferPool.Get()
}

// Put returns a buffer to the global pool.
func Put(buf *bytes.Buffer) {
	globalBufferPool.Put(buf)
}
