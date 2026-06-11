package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
)

// writerMessage contains both the message type and reader for background writing.
type writerMessage struct {
	reader   Reader
	ctx      context.Context
	respChan chan<- error
}

var _ io.WriteCloser = (*wsWriter)(nil)

type wsWriter struct {
	ctx      context.Context
	wsConn   *Conn
	wsReader *wsReader
	wait     bool
}

func (writer *wsWriter) Write(data []byte) (int, error) {
	return writer.wsReader.Write(data)
}

func (writer *wsWriter) ReadFrom(reader io.Reader) (int64, error) {
	return writer.wsReader.ReadFrom(reader)
}

func (writer *wsWriter) MessageType() MessageType {
	return writer.wsReader.msgType
}

func (writer *wsWriter) Close() error {
	return writer.wsConn.writeAsyncFromReader(writer.ctx, writer.wsReader, writer.wait)
}

// Conn is a wrapper around a *websocket.Conn that
// provides a background writer, and use of a buffer pool
// for temporary buffers.
type Conn struct {
	conn       *websocket.Conn
	bufferPool BufferPool
	wg         sync.WaitGroup
	closed     atomic.Bool
	cancelFn   context.CancelFunc

	statsMu sync.Mutex
	stats   ConnStats

	writerCh chan writerMessage
	ctx      context.Context
}

// ConnOption is a functional option for NewConn.
type ConnOption func(*Conn)

// WithBufferPoolOpt returns a ConnOption that sets the buffer pool.
func WithBufferPoolOpt(bufferPool BufferPool) ConnOption {
	return func(w *Conn) {
		w.bufferPool = bufferPool
	}
}

// WithConnectedAtOpt allows setting the time the connection started vs the
// default of 'now'. A zero time.Time also means 'now'.
func WithConnectedAtOpt(t time.Time) ConnOption {
	return func(w *Conn) {
		w.stats.ConnectedAt = t
	}
}

// NewConn creates a new Conn wrapper around a *websocket.Conn.
func NewConn(conn *websocket.Conn, opts ...ConnOption) *Conn {
	ctx, cancelFn := context.WithCancel(context.Background())
	w := &Conn{
		ctx:      ctx,
		cancelFn: cancelFn,
		conn:     conn,
	}
	for _, opt := range opts {
		opt(w)
	}
	if w.bufferPool == nil {
		w.bufferPool = bufferpool.New(8 * 1024)
	}
	if w.stats.ConnectedAt.IsZero() {
		w.stats.ConnectedAt = time.Now()
	}

	w.writerCh = make(chan writerMessage, 10)
	w.wg.Go(func() {
		defer w.closeNormal()
		w.backgroundWriter()
	})

	return w
}

// Write writes a message to the WebSocket connection via a background goroutine,
// waiting for the context to be cancelled, or for the write to complete or error.
func (w *Conn) Write(ctx context.Context, messageType MessageType, data []byte) error {
	return w.writeAsync(ctx, messageType, data, true)
}

// WriteJSON marshals v to JSON and writes it to the WebSocket connection via a background
// goroutine, waiting for the context to be cancelled, or the write to complete or error.
func (w *Conn) WriteJSON(ctx context.Context, v any) error {
	return w.writeJSONAsync(ctx, v, true)
}

// WriteAsync writes a message asynchronously via a background goroutine.
func (w *Conn) WriteAsync(ctx context.Context, messageType MessageType, data []byte) error {
	return w.writeAsync(ctx, messageType, data, false)
}

// WriteJSONAsync marshals obj to JSON using buffer_pool and writes it asynchronously via a background
// goroutine.
func (w *Conn) WriteJSONAsync(ctx context.Context, obj any) error {
	return w.writeJSONAsync(ctx, obj, false)
}

// WriteAsyncFromReader writes data from a Reader asynchronously via a background goroutine.
func (w *Conn) WriteAsyncFromReader(ctx context.Context, reader Reader) error {
	return w.writeAsyncFromReader(ctx, reader, false)
}

// WriteFromReader writes data from a Reader asynchronously via a background goroutine,
// waiting for the context to be cancelled, or the write to complete or error.
func (w *Conn) WriteFromReader(ctx context.Context, reader Reader) error {
	return w.writeAsyncFromReader(ctx, reader, true)
}

// Reader returns the message type, an io.ReadCloser that can be used to read the message.
// The returned reader should be closed after use to return the buffer to the pool. You
// can only have 1 concurrent reader at any given time, including combined with ReadJSON().
func (w *Conn) Reader(ctx context.Context) (Reader, error) {
	if w.closed.Load() {
		return nil, errWebsocketClosed
	}
	buf := w.bufferPool.Get()
	w.setDeadline(ctx, w.conn.SetReadDeadline)
	msgType, reader, err := w.conn.NextReader()
	if err != nil {
		w.bufferPool.Put(buf)
		return nil, err
	}
	_, err = buf.ReadFrom(reader)
	if err != nil {
		w.bufferPool.Put(buf)
		return nil, err
	}
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.stats.setMessageReceived(time.Now(), int64(buf.Len()))
	return getReader(w.bufferPool, buf, msgType), nil
}

// ReadJSON reads a JSON message from the WebSocket connection and unmarshals it into v
// You can only have 2 concurrent reader at any given time, including combined with Reader().
func (w *Conn) ReadJSON(ctx context.Context, v any) error {
	reader, err := w.Reader(ctx)
	if err != nil {
		return fmt.Errorf("failed to get reader for json message: %w", err)
	}
	defer reader.Done()

	// Use the reader's bytes directly since it already contains the data
	data := reader.Bytes()
	if err := json.Unmarshal(data, v); err != nil {
		w.Close(StatusInvalidFramePayloadData, "frame does not contain json")
		return fmt.Errorf("failed to decode json: %w", err)
	}

	return nil
}

// Flush waits for all pending write messages (at the time Flush
// is called) to complete.
// The websocket may continue to be used unless it has been closed.
func (w *Conn) Flush(ctx context.Context) error {
	// Send a nil reader to the background writer and wait for
	// the result.
	return w.writeAsyncFromReader(ctx, nil, true)
}

// Close closes the WebSocket connection with the given status code and reason.
func (w *Conn) Close(code StatusCode, reason string) error {
	err := w.close(code, reason)
	w.wg.Wait()
	return err
}

// SetReadDeadline sets the read deadline on the underlying connection.
// Setting a deadline in the past unblocks a pending Reader() call.
func (w *Conn) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

// Ping sends a ping frame and waits for a pong frame.
func (w *Conn) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.setDeadline(ctx, w.conn.SetWriteDeadline)
	return w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second))
}

// Subprotocol returns the negotiated subprotocol.
func (w *Conn) Subprotocol() string {
	return w.conn.Subprotocol()
}

// SetReadLimit sets the maximum size of incoming messages.
func (w *Conn) SetReadLimit(n int64) {
	w.conn.SetReadLimit(n)
}

// Writer returns a writer for the given message type. The closing of the writer will send
// the written data via a background goroutine, waiting for the context to be cancelled, or
// for the write to complete or error.
func (w *Conn) Writer(ctx context.Context, messageType MessageType) (io.WriteCloser, error) {
	if w.closed.Load() {
		return nil, errWebsocketClosed
	}
	return w.newWriter(ctx, messageType, true)
}

// AsyncWriter returns a writer for the given message type. The closing of the writer will send
// the written data via a background goroutine, not waiting for the write to complete before
// returning from Close().
func (w *Conn) AsyncWriter(ctx context.Context, messageType MessageType) (io.WriteCloser, error) {
	if w.closed.Load() {
		return nil, errWebsocketClosed
	}
	return w.newWriter(ctx, messageType, false)
}

// GetStats returns a snapshot of the connection statistics.
func (w *Conn) GetStats() ConnStats {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()

	return w.stats
}

func (w *Conn) setDeadline(ctx context.Context, setFn func(time.Time) error) {
	var zeroTime time.Time
	if deadline, ok := ctx.Deadline(); ok {
		setFn(deadline)
	} else {
		setFn(zeroTime)
	}
}

func (w *Conn) close(code StatusCode, reason string) error {
	err := errWebsocketAlreadyClosed
	if w.closed.CompareAndSwap(false, true) {
		w.cancelFn()
		closeMsg := websocket.FormatCloseMessage(code, reason)
		w.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
		err = w.conn.Close()
	}
	return err
}

func (w *Conn) closeNormal() error {
	return w.close(StatusNormalClosure, "Normal close")
}

// backgroundWriter runs in a goroutine and handles writing data from Readers.
func (w *Conn) backgroundWriter() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case msg, ok := <-w.writerCh:
			if !ok {
				// We don't ever close the writer channel, so
				// this shouldn't be reached.
				return
			}
			var err error
			// reader may be nil for Flush()
			if reader := msg.reader; reader != nil {
				w.setDeadline(msg.ctx, w.conn.SetWriteDeadline)
				err = w.conn.WriteMessage(reader.MessageType(), reader.Bytes())
				if err == nil {
					func() {
						w.statsMu.Lock()
						defer w.statsMu.Unlock()
						w.stats.setMessageSent(time.Now(), int64(reader.Len()))
					}()
				}
				reader.Done()
			}
			if respChan := msg.respChan; respChan != nil {
				select {
				case <-msg.ctx.Done():
				case respChan <- err:
				}
				close(respChan)
			}
			if err != nil {
				return
			}
		}
	}
}

// all writes funnel through here, sending them to the background goroutine to run.
// if wait is true, we'll wait for the background write to complete (or error). A nil
// reader combined with wait=true means to just receive feedback that all current writes
// have been flushed.
func (w *Conn) writeAsyncFromReader(ctx context.Context, reader Reader, wait bool) error {
	if w.closed.Load() {
		return errWebsocketClosed
	}

	var respChan chan error
	if wait {
		respChan = make(chan error, 1)
	}

	// Send the message type, reader, and context to the background goroutine
	msg := writerMessage{
		reader:   reader,
		ctx:      ctx,
		respChan: respChan,
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.ctx.Done():
		return errWebsocketClosed
	case w.writerCh <- msg:
	}

	if respChan == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.ctx.Done():
		return errWebsocketClosed
	case err := <-respChan:
		return err
	}
}

func (w *Conn) newWriter(ctx context.Context, messageType MessageType, wait bool) (io.WriteCloser, error) {
	if w.closed.Load() {
		return nil, errWebsocketClosed
	}

	writer := &wsWriter{
		ctx:      ctx,
		wsConn:   w,
		wsReader: newReader(w.bufferPool, messageType),
		wait:     wait,
	}

	return writer, nil
}

func (w *Conn) writeAsync(ctx context.Context, messageType MessageType, data []byte, wait bool) error {
	reader := newReader(w.bufferPool, messageType)
	reader.Write(data)
	return w.writeAsyncFromReader(ctx, reader, wait)
}

func (w *Conn) writeJSONAsync(ctx context.Context, obj any, wait bool) error {
	reader := newReader(w.bufferPool, MessageText)
	encoder := json.NewEncoder(reader)
	if err := encoder.Encode(obj); err != nil {
		reader.Done()
		return fmt.Errorf("failed to encode json: %w", err)
	}

	err := w.writeAsyncFromReader(ctx, reader, wait)
	if err != nil {
		// we can't be sure the buffer didn't reach the goroutine
		// and already put it back. reader.Done() would prevent
		// multiple put-backs, so it's probably safe to do it when
		// wait is true... but it's not a big deal to lose it, either.
		return err
	}
	return nil
}
