package ws

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

// Reader is the interface for reading WebSocket messages.
type Reader interface {
	io.Reader
	Bytes() []byte
	MessageType() MessageType
	Len() int
	Done()
}

var wsReaderPool = sync.Pool{
	New: func() any {
		return &wsReader{}
	},
}

type wsReader struct {
	*bytes.Buffer

	bufferPool BufferPool
	msgType    MessageType
	done       atomic.Bool
}

func (r *wsReader) MessageType() MessageType {
	return r.msgType
}

func (r *wsReader) Done() {
	if !r.done.CompareAndSwap(false, true) {
		return
	}
	r.bufferPool.Put(r.Buffer)
	r.Buffer = nil
	r.bufferPool = nil
	wsReaderPool.Put(r)
}

func getReader(bufferPool BufferPool, buf *bytes.Buffer, msgType MessageType) *wsReader {
	r, ok := wsReaderPool.Get().(*wsReader)
	if !ok {
		r = &wsReader{}
	}
	r.Buffer = buf
	r.bufferPool = bufferPool
	r.msgType = msgType
	r.done.Store(false)
	return r
}

func newReader(bufferPool BufferPool, msgType MessageType) *wsReader {
	return getReader(bufferPool, bufferPool.Get(), msgType)
}
