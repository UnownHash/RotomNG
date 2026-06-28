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

	// readDeadlineExt is the duration (in nanoseconds) by which the read
	// deadline is extended each time a pong is received. It is updated by
	// StartPingLoop so settings reloads take effect without re-installing the
	// pong handler (which would race with the reader).
	readDeadlineExt atomic.Int64

	// bgMu serializes starting background goroutines (via wg.Go) against the
	// wg.Wait performed in Close, so the WaitGroup is never given a concurrent
	// Add and Wait (which the race detector flags and the docs forbid).
	bgMu sync.Mutex

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
func (w *Conn) Reader(_ context.Context) (Reader, error) {
	if w.closed.Load() {
		return nil, errWebsocketClosed
	}
	buf := w.bufferPool.Get()
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
	// Serialize against StartPingLoop: w.close has set closed=true, so any
	// in-flight StartPingLoop either already finished its wg.Go (ordered before
	// this Wait) or, once it acquires bgMu, observes closed and skips it. Holding
	// bgMu across Wait prevents a concurrent wg.Add/Wait on the WaitGroup. The
	// background goroutines do not take bgMu, so this cannot deadlock.
	w.bgMu.Lock()
	defer w.bgMu.Unlock()
	w.wg.Wait()
	return err
}

// SetReadDeadline sets the read deadline on the underlying connection.
// Setting a deadline in the past unblocks a pending Reader() call.
func (w *Conn) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

// Ping sends a ping control frame. It is safe to call concurrently with writes.
func (w *Conn) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	return w.conn.WriteControl(websocket.PingMessage, nil, deadline)
}

// EnableReadTimeout sets the initial read deadline and installs a pong handler
// that extends the read deadline by interval+pongWait each time a pong is
// received; if no pong arrives the deadline expires naturally and the next
// Reader call will return an error. It mutates read-side connection state and
// therefore must be called on the goroutine that reads from the connection,
// before any reads begin, and never concurrently with a read. Subsequent
// changes to the extension (e.g. on a settings reload) are picked up
// automatically by StartPingLoop without re-installing the handler.
func (w *Conn) EnableReadTimeout(interval, pongWait time.Duration) {
	w.readDeadlineExt.Store(int64(interval + pongWait))
	_ = w.conn.SetReadDeadline(time.Now().Add(interval + pongWait))
	w.conn.SetPongHandler(func(string) error {
		now := time.Now()
		ext := time.Duration(w.readDeadlineExt.Load())
		// The pong handler runs on the read goroutine inside NextReader, which
		// does not hold statsMu, so locking here is safe and keeps GetStats
		// consistent. Record the pong so it counts toward LastSeenAt without
		// inflating the data-message counters.
		w.statsMu.Lock()
		w.stats.setPongReceived(now)
		w.statsMu.Unlock()
		return w.conn.SetReadDeadline(now.Add(ext))
	})
}

// StartPingLoop begins a background goroutine that sends a WebSocket ping every
// interval. The read deadline is extended by interval+pongWait on each pong
// received via the handler installed by EnableReadTimeout, which must be called
// first. On ping write failure the connection is closed. The loop stops when ctx
// is done or the connection is closed. StartPingLoop only sends pings (a
// concurrency-safe control write) and updates the pong extension, so it is safe
// to call concurrently with a reader and may be called repeatedly to apply new
// interval/pongWait values.
func (w *Conn) StartPingLoop(ctx context.Context, interval, pongWait time.Duration) {
	w.readDeadlineExt.Store(int64(interval + pongWait))

	// Guard against racing the wg.Wait in Close: if the connection is already
	// closing, don't start the goroutine (it would have nothing live to ping).
	w.bgMu.Lock()
	defer w.bgMu.Unlock()
	if w.closed.Load() {
		return
	}

	w.wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				if err := w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(pongWait)); err != nil {
					w.close(StatusAbnormalClosure, "ping failed")
					return
				}
			}
		}
	})
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
