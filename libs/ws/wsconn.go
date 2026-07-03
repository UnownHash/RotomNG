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

// PingSettings groups the ping keep-alive settings applied to a Conn. They are
// stored together behind a single atomic pointer so they always change as a set.
//
//   - Interval: how often to send a ping control frame. Zero disables ping
//     sending entirely.
//   - Timeout: how long the connection may go without receiving either a pong or
//     a data message before it is considered dead. Zero disables the ping
//     timeout. When it fires, Reader returns errReadTimeout.
//
// Negative values are invalid.
type PingSettings struct {
	Interval time.Duration
	Timeout  time.Duration
}

// IsValid reports whether the settings are usable (no negative durations).
func (p PingSettings) IsValid() bool {
	return p.Interval >= 0 && p.Timeout >= 0
}

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

	// pingSettings holds the current ping keep-alive settings. It is never nil
	// after NewConn. The always-running timeout goroutine reads it each cycle so
	// SetPingSettings takes effect without any goroutine start/stop.
	pingSettings atomic.Pointer[PingSettings]

	// readDataTimeout (nanoseconds) is the maximum time the connection may go
	// without receiving a data message before it is considered dead. Zero
	// disables the data timeout. When it fires, Reader returns errReadDataTimeout.
	readDataTimeout atomic.Int64

	// readErr, when set, is the error Reader returns once the timeout goroutine
	// has declared the connection dead (see errReadTimeout / errReadDataTimeout).
	readErr atomic.Pointer[error]

	// wakeCh nudges the timeout goroutine to recompute its schedule after a
	// settings or read-deadline change. Buffered so signalling never blocks.
	wakeCh chan struct{}

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

// WithPingSettings returns a ConnOption that sets the initial ping keep-alive
// settings. Invalid (negative) settings are ignored, leaving pinging disabled;
// callers that need to detect invalid input should use SetPingSettings.
func WithPingSettings(s PingSettings) ConnOption {
	return func(w *Conn) {
		if s.IsValid() {
			w.pingSettings.Store(&s)
		}
	}
}

// WithReadDataTimeout returns a ConnOption that sets the initial data read
// timeout. A negative timeout is ignored, leaving the data timeout disabled.
func WithReadDataTimeout(d time.Duration) ConnOption {
	return func(w *Conn) {
		if d >= 0 {
			w.readDataTimeout.Store(int64(d))
		}
	}
}

// NewConn creates a new Conn wrapper around a *websocket.Conn.
func NewConn(conn *websocket.Conn, opts ...ConnOption) *Conn {
	ctx, cancelFn := context.WithCancel(context.Background())
	w := &Conn{
		ctx:      ctx,
		cancelFn: cancelFn,
		conn:     conn,
		wakeCh:   make(chan struct{}, 1),
	}
	// Default to no pinging and no timeouts until configured.
	w.pingSettings.Store(&PingSettings{})
	for _, opt := range opts {
		opt(w)
	}
	if w.bufferPool == nil {
		w.bufferPool = bufferpool.New(8 * 1024)
	}
	if w.stats.ConnectedAt.IsZero() {
		w.stats.ConnectedAt = time.Now()
	}

	// Install the pong handler once, before any reads. It runs on the read
	// goroutine inside NextReader; it only records the pong into stats (no
	// read-deadline mutation), so it is safe alongside the reader. The timeout
	// goroutine reads stats.LastPongAt to enforce the ping timeout.
	w.conn.SetPongHandler(func(string) error {
		now := time.Now()
		w.statsMu.Lock()
		w.stats.setPongReceived(now)
		w.statsMu.Unlock()
		return nil
	})

	w.writerCh = make(chan writerMessage, 10)
	w.wg.Go(func() {
		defer w.closeNormal()
		w.backgroundWriter()
	})
	// The timeout goroutine always runs; it sends pings and enforces the ping and
	// data timeouts based on the current (atomically updated) settings.
	w.wg.Go(w.timeoutLoop)

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
	if err := w.loadReadErr(); err != nil {
		return nil, err
	}
	buf := w.bufferPool.Get()
	msgType, reader, err := w.conn.NextReader()
	if err != nil {
		w.bufferPool.Put(buf)
		// If the timeout goroutine woke the reader by expiring the deadline, it
		// stored the specific timeout error first; prefer it over the raw
		// deadline-exceeded error.
		if readErr := w.loadReadErr(); readErr != nil {
			return nil, readErr
		}
		return nil, err
	}
	_, err = buf.ReadFrom(reader)
	if err != nil {
		w.bufferPool.Put(buf)
		return nil, err
	}
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	// The timeout goroutine reads stats.LastReceivedAt to enforce the ping and
	// data timeouts.
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
	// All background goroutines (the writer and the timeout loop) are started
	// synchronously in NewConn, before any caller can invoke Close, so there is
	// never a concurrent wg.Add racing this Wait. w.close cancelled the context,
	// which both goroutines observe and exit on.
	w.wg.Wait()
	return err
}

// SetReadDeadline sets the read deadline on the underlying connection.
// Setting a deadline in the past unblocks a pending Reader() call. It also wakes
// the timeout goroutine so it recomputes its schedule.
func (w *Conn) SetReadDeadline(t time.Time) error {
	err := w.conn.SetReadDeadline(t)
	w.signalTimeout()
	return err
}

// SetPingSettings replaces the ping keep-alive settings and wakes the timeout
// goroutine to apply them. Negative durations are rejected.
func (w *Conn) SetPingSettings(s PingSettings) error {
	if !s.IsValid() {
		return errInvalidPingSettings
	}
	w.pingSettings.Store(&s)
	w.signalTimeout()
	return nil
}

// SetReadDataTimeout replaces the data read timeout and wakes the timeout
// goroutine to apply it. Zero disables the data timeout; a negative value is
// rejected.
func (w *Conn) SetReadDataTimeout(d time.Duration) error {
	if d < 0 {
		return errInvalidReadDataTimeout
	}
	w.readDataTimeout.Store(int64(d))
	w.signalTimeout()
	return nil
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

// loadReadErr returns the stored terminal read error, if any.
func (w *Conn) loadReadErr() error {
	if errp := w.readErr.Load(); errp != nil {
		return *errp
	}
	return nil
}

// signalTimeout nudges the timeout goroutine without blocking.
func (w *Conn) signalTimeout() {
	select {
	case w.wakeCh <- struct{}{}:
	default:
	}
}

// timeoutLoop is the always-running goroutine started by NewConn. It sends
// pings on the configured interval and enforces the ping and data timeouts.
// When a timeout elapses it stores the corresponding error and expires the read
// deadline so a blocked Reader wakes up and returns that error.
func (w *Conn) timeoutLoop() {
	// idleWake bounds how long we sleep when nothing is scheduled; a wakeCh
	// signal or ctx cancellation still wakes us sooner.
	const idleWake = time.Hour

	timer := time.NewTimer(idleWake)
	defer timer.Stop()

	// lastPing tracks when we last sent a ping (loop-local; pings are only sent
	// from here).
	lastPing := time.Now()

	for {
		ps := *w.pingSettings.Load()
		dataTimeout := time.Duration(w.readDataTimeout.Load())
		now := time.Now()

		// Send a ping if one is due.
		nextPing := time.Duration(-1)
		if ps.Interval > 0 {
			if elapsed := now.Sub(lastPing); elapsed >= ps.Interval {
				if err := w.conn.WriteControl(websocket.PingMessage, nil, now.Add(time.Second)); err != nil {
					// The write failed, so the connection is broken. Wake any
					// blocked reader; it will surface the underlying error.
					_ = w.conn.SetReadDeadline(now)
					return
				}
				lastPing = now
				nextPing = ps.Interval
			} else {
				nextPing = ps.Interval - elapsed
			}
		}

		// Snapshot the activity timestamps recorded by Reader and the pong
		// handler, after any ping write so activity that arrives meanwhile is
		// picked up. ConnectedAt is the floor so a connection that has not yet
		// received anything is measured from its start rather than the zero time.
		w.statsMu.Lock()
		lastData := laterTime(w.stats.ConnectedAt, w.stats.LastReceivedAt)
		lastPongOrData := laterTime(lastData, w.stats.LastPongAt)
		w.statsMu.Unlock()

		// Enforce the ping timeout (reset by a pong OR a data message).
		wakePing, dead := w.checkTimeout(ps.Timeout, lastPongOrData, now, errReadTimeout)
		if dead {
			return
		}

		// Enforce the data timeout (reset only by a data message).
		wakeData, dead := w.checkTimeout(dataTimeout, lastData, now, errReadDataTimeout)
		if dead {
			return
		}

		wake := minPositiveDuration(minPositiveDuration(nextPing, wakePing), wakeData)
		if wake <= 0 {
			wake = idleWake
		}
		// Go 1.23+: Reset on a running or expired timer is safe and cannot yield a
		// stale value, so no Stop/drain is needed here.
		timer.Reset(wake)

		select {
		case <-w.ctx.Done():
			return
		case <-w.wakeCh:
		case <-timer.C:
		}
	}
}

// checkTimeout evaluates a single timeout. If timeout <= 0 it is disabled and
// returns (-1, false). If the deadline (lastActivity+timeout) has passed it
// stores deadErr, expires the read deadline, and returns (_, true). Otherwise it
// returns the remaining duration until the deadline and false.
func (w *Conn) checkTimeout(timeout time.Duration, lastActivity, now time.Time, deadErr error) (time.Duration, bool) {
	if timeout <= 0 {
		return -1, false
	}
	deadline := lastActivity.Add(timeout)
	if !now.Before(deadline) {
		err := deadErr
		w.readErr.CompareAndSwap(nil, &err)
		_ = w.conn.SetReadDeadline(now)
		return -1, true
	}
	return deadline.Sub(now), false
}

// laterTime returns the later of two times.
func laterTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// minPositiveDuration returns the smaller of two durations, ignoring any that
// are <= 0 (treated as "not scheduled"). Returns -1 if neither is positive.
func minPositiveDuration(a, b time.Duration) time.Duration {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
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
