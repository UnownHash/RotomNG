package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"

	"github.com/UnownHash/RotomNG/libs/bufferpool"
	"github.com/UnownHash/RotomNG/libs/logging"
)

var testBufferPool = bufferpool.New(32 * 1024)

// setupPair creates a connected Conn client/server pair using an in-process HTTP server.
// The serverFn runs in a goroutine with the server-side Conn; the returned Conn is the client side.
func setupPair(t *testing.T, serverFn func(t *testing.T, server *Conn)) *Conn {
	t.Helper()

	serverReady := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sConn, err := Accept(w, r, WithAcceptBufferPoolOpt(testBufferPool))
		if err != nil {
			t.Errorf("server accept: %v", err)
			return
		}
		defer sConn.Close(StatusNormalClosure, "")
		close(serverReady)
		serverFn(t, sConn)
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, resp, err := Dial(context.Background(), url, WithDialBufferPoolOpt(testBufferPool))
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	t.Cleanup(func() { client.Close(StatusNormalClosure, "") })
	<-serverReady
	return client
}

// setupEchoPair creates a pair where the server echoes back every message it receives.
func setupEchoPair(t *testing.T) *Conn {
	t.Helper()
	return setupPair(t, func(_ *testing.T, server *Conn) {
		defer server.Close(StatusNormalClosure, "")
		for {
			reader, err := server.Reader(context.Background())
			if err != nil {
				return
			}
			if err := func() error {
				defer reader.Done()
				return server.Write(context.Background(), reader.MessageType(), reader.Bytes())
			}(); err != nil {
				return
			}
		}
	})
}

func TestNewConn(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	if client == nil {
		t.Fatal("NewConn returned nil")
	}
	if client.conn == nil {
		t.Error("WSConn.conn not set correctly")
	}
	if client.bufferPool == nil {
		t.Error("WSConn.bufferPool not set correctly")
	}
	if client.ctx == nil {
		t.Error("WSConn.ctx not set")
	}
	if client.cancelFn == nil {
		t.Error("WSConn.cancelFn not set")
	}
}

func TestWSConn_ReadWriteText(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	testData := []byte("test message")
	err := client.Write(ctx, MessageText, testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read back the echo
	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if reader.MessageType() != MessageText {
		t.Errorf("Expected MessageText, got %v", reader.MessageType())
	}
	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected data %s, got %s", testData, reader.Bytes())
	}
}

func TestWSConn_ReadWriteBinary(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	testData := []byte{0x00, 0x01, 0x02, 0xFF}
	err := client.Write(ctx, MessageBinary, testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if reader.MessageType() != MessageBinary {
		t.Errorf("Expected MessageBinary, got %v", reader.MessageType())
	}
	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected data %v, got %v", testData, reader.Bytes())
	}
}

func TestWSConn_Writer(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	writer, err := client.Writer(ctx, MessageText)
	if err != nil {
		t.Fatalf("Writer failed: %v", err)
	}

	testData := []byte("test message")
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatalf("Write to writer failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close writer failed: %v", err)
	}

	// Read back the echo
	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected data %s, got %s", testData, reader.Bytes())
	}
}

func TestWSConn_Writer_Closed(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	_, err := client.Writer(ctx, MessageText)
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}
}

func TestWSConn_WriteAsyncFromReader(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	buf := client.bufferPool.Get()
	testData := []byte("async test message")
	buf.Write(testData)
	reader := &wsReader{
		Buffer:     buf,
		bufferPool: client.bufferPool,
		msgType:    MessageText,
	}

	err := client.WriteAsyncFromReader(ctx, reader)
	if err != nil {
		t.Fatalf("WriteAsyncFromReader failed: %v", err)
	}

	// Read back the echo
	echoReader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer echoReader.Done()

	if !bytes.Equal(echoReader.Bytes(), testData) {
		t.Errorf("Expected data %s, got %s", testData, echoReader.Bytes())
	}
}

func TestWSConn_WriteAsyncFromReader_ContextCanceled(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Start background writer and block it
	setBlockingWriterChan(client)
	cancel()

	err := client.WriteAsyncFromReader(ctx, &wsReader{msgType: MessageText, Buffer: &bytes.Buffer{}})
	if !errors.Is(err, context.Canceled) && !IsErrWebsocketClosed(err) {
		t.Errorf("Expected context.Canceled or websocket closed, got %v", err)
	}
}

func TestWSConn_WriteAsyncFromReader_InternalContextCanceled(t *testing.T) {
	client := setupEchoPair(t)

	setBlockingWriterChan(client)
	client.cancelFn()

	ctx := context.Background()
	err := client.WriteAsyncFromReader(ctx, &wsReader{msgType: MessageText, Buffer: &bytes.Buffer{}})
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}

	client.Close(StatusNormalClosure, "test complete")
}

func TestWSConn_Ping(t *testing.T) {
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		defer server.Close(StatusNormalClosure, "")
		// Server needs to read to process control frames
		server.Reader(context.Background())
	})
	defer client.Close(StatusNormalClosure, "")

	err := client.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

// TestWSConn_PongUpdatesLastSeen verifies that the pong handler installed by
// EnableReadTimeout records pong activity so it counts toward LastSeenAt, even
// when no data messages flow. The server pings; the client auto-responds with a
// pong; the server's pong handler must advance LastSeenAt without inflating the
// received-message counters.
func TestWSConn_PongUpdatesLastSeen(t *testing.T) {
	serverCh := make(chan *Conn, 1)
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		// Installs the pong handler on the reading goroutine, before any reads.
		server.EnableReadTimeout(time.Minute, time.Minute)
		serverCh <- server
		for {
			if _, err := server.Reader(context.Background()); err != nil {
				return
			}
		}
	})

	// The client must be reading so it processes the server's ping control frame
	// and auto-responds with a pong (the default ping handler).
	go func() {
		for {
			if _, err := client.Reader(context.Background()); err != nil {
				return
			}
		}
	}()

	server := <-serverCh
	before := server.GetStats()
	if !before.LastPongAt.IsZero() {
		t.Fatal("LastPongAt should be zero before any pong")
	}

	time.Sleep(5 * time.Millisecond)
	if err := server.Ping(context.Background()); err != nil {
		t.Fatalf("server ping: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		got := server.GetStats()
		if !got.LastPongAt.IsZero() {
			if !got.LastSeenAt().After(before.LastSeenAt()) {
				t.Error("LastSeenAt should advance after a pong is received")
			}
			if got.MessagesReceived != before.MessagesReceived {
				t.Errorf("pong must not change MessagesReceived: before=%d after=%d", before.MessagesReceived, got.MessagesReceived)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server LastPongAt was not updated after receiving a pong")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWSConn_Close(t *testing.T) {
	serverErr := make(chan error)
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		defer close(serverErr)
		for {
			_, err := server.Reader(context.Background())
			if err != nil {
				serverErr <- err
				return
			}
		}
	})

	err := client.Close(StatusNormalClosure, "test close")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify server saw correct close code and text
	sErr := <-serverErr
	ce := GetWebsocketCloseError(sErr)
	if ce == nil {
		t.Fatalf("expected CloseError from server Reader, got %v", sErr)
	}
	if ce.Code != StatusNormalClosure {
		t.Errorf("expected close code %d, got %d", StatusNormalClosure, ce.Code)
	}
	if ce.Text != "test close" {
		t.Errorf("expected close text %q, got %q", "test close", ce.Text)
	}

	err = client.Close(StatusNormalClosure, "")
	if !errors.Is(err, errWebsocketAlreadyClosed) {
		t.Errorf("Expected errWebsocketAlreadyClosed, got %v", err)
	}
}

func TestWSConn_CloseInternalServerError(t *testing.T) {
	serverErr := make(chan error)
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		defer close(serverErr)
		for {
			_, err := server.Reader(context.Background())
			if err != nil {
				serverErr <- err
				return
			}
		}
	})

	err := client.Close(StatusInternalServerError, "something broke")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	sErr := <-serverErr
	ce := GetWebsocketCloseError(sErr)
	if ce == nil {
		t.Fatalf("expected CloseError from server Reader, got %v", sErr)
	}
	if ce.Code != StatusInternalServerError {
		t.Errorf("expected close code %d, got %d", StatusInternalServerError, ce.Code)
	}
	if ce.Text != "something broke" {
		t.Errorf("expected close text %q, got %q", "something broke", ce.Text)
	}
}

func TestWSConn_Subprotocol(t *testing.T) {
	subproto := "test-protocol"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin:  func(*http.Request) bool { return true },
			Subprotocols: []string{subproto},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		sConn := NewConn(conn, WithBufferPoolOpt(testBufferPool))
		defer sConn.Close(StatusNormalClosure, "")
		// Hold connection open until client closes
		sConn.Reader(context.Background())
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := websocket.Dialer{Subprotocols: []string{subproto}}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}

	client := NewConn(conn, WithBufferPoolOpt(testBufferPool))
	defer client.Close(StatusNormalClosure, "")

	if client.Subprotocol() != subproto {
		t.Errorf("Expected subprotocol %s, got %s", subproto, client.Subprotocol())
	}
}

func TestWSConn_SetReadLimit(t *testing.T) {
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		defer server.Close(StatusNormalClosure, "")
		// Send a message larger than the limit
		bigData := make([]byte, 2048)
		server.Write(context.Background(), MessageBinary, bigData)
	})
	defer client.Close(StatusNormalClosure, "")

	client.SetReadLimit(100)

	_, err := client.Reader(context.Background())
	if err == nil {
		t.Error("Expected error due to read limit exceeded")
	}
}

func TestWSConn_ReadJSON(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	testObj := map[string]any{
		"message": "hello",
		"number":  float64(42),
	}
	testData, _ := json.Marshal(testObj) //nolint:errchkjson
	if err := client.Write(ctx, MessageText, testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var result map[string]any
	err := client.ReadJSON(ctx, &result)
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	if result["message"] != "hello" {
		t.Errorf("Expected message 'hello', got %v", result["message"])
	}
	if result["number"] != float64(42) {
		t.Errorf("Expected number 42, got %v", result["number"])
	}
}

func TestWSConn_ReadJSON_InvalidJSON(t *testing.T) {
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		// Send invalid JSON
		server.Write(context.Background(), MessageText, []byte("invalid json"))
	})
	defer client.Close(StatusNormalClosure, "")

	var result map[string]any
	err := client.ReadJSON(context.Background(), &result)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestWSConn_WriteJSON(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	testObj := map[string]any{
		"message": "hello",
		"number":  float64(42),
	}

	err := client.WriteJSON(ctx, testObj)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// Read back the echo
	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if reader.MessageType() != MessageText {
		t.Errorf("Expected MessageText, got %v", reader.MessageType())
	}

	var result map[string]any
	err = json.Unmarshal(reader.Bytes(), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal written JSON: %v", err)
	}
	if result["message"] != "hello" {
		t.Errorf("Expected message 'hello', got %v", result["message"])
	}
	if result["number"] != float64(42) {
		t.Errorf("Expected number 42, got %v", result["number"])
	}
}

func TestWSConn_WriteJSONAsync(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	testObj := map[string]any{
		"message": "hello async",
		"number":  float64(123),
	}

	err := client.WriteJSONAsync(ctx, testObj)
	if err != nil {
		t.Fatalf("WriteJSONAsync failed: %v", err)
	}

	// Read back the echo
	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	var result map[string]any
	err = json.Unmarshal(reader.Bytes(), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal written JSON: %v", err)
	}
	if result["message"] != "hello async" {
		t.Errorf("Expected message 'hello async', got %v", result["message"])
	}
	if result["number"] != float64(123) {
		t.Errorf("Expected number 123, got %v", result["number"])
	}
}

func TestWSConn_WriteJSON_EncodeError(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	invalidObj := make(chan int)
	err := client.WriteJSON(context.Background(), invalidObj)
	if err == nil {
		t.Error("Expected error for invalid JSON object")
	}
	if !strings.Contains(err.Error(), "failed to encode json") {
		t.Errorf("Expected encode error message, got %v", err)
	}
}

func TestWSConn_WriteJSONAsync_EncodeError(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	invalidObj := make(chan int)
	err := client.WriteJSONAsync(context.Background(), invalidObj)
	if err == nil {
		t.Error("Expected error for invalid JSON object")
	}
	if !strings.Contains(err.Error(), "failed to encode json") {
		t.Errorf("Expected encode error message, got %v", err)
	}
}

func TestReader_Done(_ *testing.T) {
	buf := testBufferPool.Get()
	buf.WriteString("test data")

	reader := &wsReader{
		Buffer:     buf,
		bufferPool: testBufferPool,
	}

	// Test that Done() can be called multiple times safely
	reader.Done()
	reader.Done() // Should not panic or cause issues
}

func TestWSConn_MultipleAsyncWrites(t *testing.T) {
	numWrites := 5
	received := make(chan []byte, numWrites)

	client := setupPair(t, func(_ *testing.T, server *Conn) {
		defer server.Close(StatusNormalClosure, "")
		for range numWrites {
			reader, err := server.Reader(context.Background())
			if err != nil {
				return
			}
			data := make([]byte, reader.Len())
			copy(data, reader.Bytes())
			reader.Done()
			received <- data
		}
	})
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	for i := range numWrites {
		buf := client.bufferPool.Get()
		testData := []byte("message " + string(rune('0'+i)))
		buf.Write(testData)
		reader := &wsReader{
			Buffer:     buf,
			bufferPool: client.bufferPool,
			msgType:    MessageText,
		}
		err := client.WriteAsyncFromReader(ctx, reader)
		if err != nil {
			t.Fatalf("WriteAsyncFromReader %d failed: %v", i, err)
		}
	}

	// Wait for all messages to be received
	for i := range numWrites {
		select {
		case data := <-received:
			expectedData := "message " + string(rune('0'+i))
			if !bytes.Equal(data, []byte(expectedData)) {
				t.Errorf("Write %d: expected data %s, got %s", i, expectedData, data)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for message %d", i)
		}
	}
}

func TestWSConn_Flush_NoPendingWrites(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	err := client.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush failed: %v", err)
	}
}

func TestWSConn_Flush_WithPendingWrites(t *testing.T) {
	numWrites := 3
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		defer server.Close(StatusNormalClosure, "")
		for range numWrites {
			reader, err := server.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	})
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	for i := range numWrites {
		buf := client.bufferPool.Get()
		testData := []byte("message " + string(rune('0'+i)))
		buf.Write(testData)
		reader := &wsReader{
			Buffer:     buf,
			bufferPool: client.bufferPool,
			msgType:    MessageText,
		}
		err := client.WriteAsyncFromReader(ctx, reader)
		if err != nil {
			t.Fatalf("WriteAsyncFromReader %d failed: %v", i, err)
		}
	}

	// Flush should wait for all writes to complete
	err := client.Flush(ctx)
	if err != nil {
		t.Errorf("Flush failed: %v", err)
	}
}

func TestWSConn_Flush_ContextCanceled(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(context.Background())

	setBlockingWriterChan(client)
	cancel()

	err := client.Flush(ctx)
	if !errors.Is(err, context.Canceled) && !IsErrWebsocketClosed(err) {
		t.Errorf("Expected context.Canceled or websocket closed, got %v", err)
	}
}

func TestWSConn_Flush_WriterContextCanceled(t *testing.T) {
	client := setupEchoPair(t)

	// Start background writer
	client.Flush(context.Background())

	// Cancel the internal context
	client.cancelFn()

	err := client.Flush(context.Background())
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected ErrWebsocketClosed, got %v", err)
	}

	client.Close(StatusNormalClosure, "test complete")
}

func TestWSConn_Flush_SynchronousWrites(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	writesStarted := make(chan struct{})
	wg.Add(1)
	go func() {
		ch := writesStarted
		defer wg.Done()
		defer cancelFn()
		for {
			if ctx.Err() != nil {
				return
			}
			err := client.Write(ctx, MessageText, []byte("sync message"))
			if ch != nil {
				close(ch)
				ch = nil
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Write failed: %v", err)
			}
		}
	}()

	flushesStarted := make(chan struct{})
	wg.Add(1)
	go func() {
		ch := flushesStarted
		defer wg.Done()
		defer cancelFn()
		for {
			if ctx.Err() != nil {
				return
			}
			err := client.Flush(ctx)
			if ch != nil {
				close(ch)
				ch = nil
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Flush failed: %v", err)
				return
			}
		}
	}()

	<-writesStarted
	<-flushesStarted

	select {
	case <-ctx.Done():
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAccept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, WithAcceptBufferPoolOpt(testBufferPool))
		if err != nil {
			t.Errorf("Accept failed: %v", err)
			return
		}
		defer conn.Close(StatusNormalClosure, "")

		// Echo one message
		reader, err := conn.Reader(context.Background())
		if err != nil {
			return
		}
		data := make([]byte, reader.Len())
		copy(data, reader.Bytes())
		msgType := reader.MessageType()
		reader.Done()
		conn.Write(context.Background(), msgType, data)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	testData := []byte("accept test")
	conn.WriteMessage(MessageText, testData)

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(msg, testData) {
		t.Errorf("Expected %s, got %s", testData, msg)
	}
}

func TestAccept_Error(t *testing.T) {
	w := &http.Response{}
	r := &http.Request{
		Method: http.MethodGet,
		Header: make(http.Header),
	}

	// Use a minimal ResponseWriter that can't upgrade
	rw := &failResponseWriter{}
	conn, err := Accept(rw, r, WithAcceptBufferPoolOpt(testBufferPool))
	if err == nil {
		t.Error("Expected Accept to fail with invalid request")
	}
	if conn != nil {
		t.Error("expected conn to be nil")
	}
	_ = w
}

type failResponseWriter struct {
	header http.Header
}

func (f *failResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}
func (f *failResponseWriter) Write([]byte) (int, error) { return 0, errors.New("fail") }
func (f *failResponseWriter) WriteHeader(_ int)         {}

func TestDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Echo one message
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.WriteMessage(mt, msg)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx := context.Background()
	client, resp, err := Dial(ctx, url, WithDialBufferPoolOpt(testBufferPool))
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	defer client.Close(StatusNormalClosure, "")

	testData := []byte("dial test")
	if err := client.Write(ctx, MessageText, testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected %s, got %s", testData, reader.Bytes())
	}
}

func TestDial_Error(t *testing.T) {
	ctx := context.Background()
	conn, resp, err := Dial(ctx, "invalid-url", WithDialBufferPoolOpt(testBufferPool))
	if err == nil {
		t.Error("Expected Dial to fail with invalid URL")
	}
	if resp != nil {
		resp.Body.Close()
	}
	if conn != nil {
		t.Error("expected conn to be nil")
	}
}

func setBlockingWriterChan(wsConn *Conn) {
	// Start background goroutine once and then exit it. It
	// will then not be created again.
	wsConn.Flush(context.Background())
	close(wsConn.writerCh)
	wsConn.wg.Wait()
	wsConn.writerCh = make(chan writerMessage)
}

// --- errors.go coverage ---

func TestIsErrWebsocketAlreadyClosed(t *testing.T) {
	if !IsErrWebsocketAlreadyClosed(errWebsocketAlreadyClosed) {
		t.Error("expected true for errWebsocketAlreadyClosed")
	}
	if IsErrWebsocketAlreadyClosed(errors.New("other error")) {
		t.Error("expected false for other error")
	}
	if IsErrWebsocketAlreadyClosed(nil) {
		t.Error("expected false for nil")
	}
}

func TestGetWebsocketCloseError(t *testing.T) {
	// Non-close error returns nil
	if ce := GetWebsocketCloseError(errors.New("not a close error")); ce != nil {
		t.Errorf("expected nil, got %v", ce)
	}
	// Actual CloseError
	closeErr := &CloseError{Code: StatusGoingAway, Text: "going away"}
	ce := GetWebsocketCloseError(closeErr)
	if ce == nil {
		t.Fatal("expected CloseError, got nil")
	}
	if ce.Code != StatusGoingAway {
		t.Errorf("expected code %d, got %d", StatusGoingAway, ce.Code)
	}
}

func TestLogWebsocketReadError(_ *testing.T) {
	// nil logger context — should not panic
	LogWebsocketReadError(context.Background(), errors.New("some error"))

	// With logger: non-close, non-canceled error
	logger := logging.NewDiscardLogger()
	ctx := logging.ContextWithLogger(context.Background(), logger)
	LogWebsocketReadError(ctx, errors.New("read fail"))

	// context.Canceled — should be silently ignored
	LogWebsocketReadError(ctx, context.Canceled)

	// CloseError with StatusGoingAway
	LogWebsocketReadError(ctx, &CloseError{Code: StatusGoingAway, Text: "going away"})

	// CloseError with other code
	LogWebsocketReadError(ctx, &CloseError{Code: StatusAbnormalClosure, Text: "abnormal"})
}

func TestLogWebsocketWriteError(_ *testing.T) {
	// nil logger context
	LogWebsocketWriteError(context.Background(), "target", errors.New("some error"))

	logger := logging.NewDiscardLogger()
	ctx := logging.ContextWithLogger(context.Background(), logger)

	// Non-close, non-canceled
	LogWebsocketWriteError(ctx, "target", errors.New("write fail"))

	// context.Canceled
	LogWebsocketWriteError(ctx, "target", context.Canceled)

	// CloseError StatusGoingAway
	LogWebsocketWriteError(ctx, "target", &CloseError{Code: StatusGoingAway, Text: "going away"})

	// CloseError other
	LogWebsocketWriteError(ctx, "target", &CloseError{Code: StatusAbnormalClosure, Text: "abnormal"})
}

// --- wsconn.go additional coverage ---

func TestWSConn_AsyncWriter(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	writer, err := client.AsyncWriter(ctx, MessageText)
	if err != nil {
		t.Fatalf("AsyncWriter failed: %v", err)
	}

	testData := []byte("async writer test")
	_, err = writer.Write(testData)
	if err != nil {
		t.Fatalf("Write to async writer failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close async writer failed: %v", err)
	}

	// Read back the echo
	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected data %s, got %s", testData, reader.Bytes())
	}
}

func TestWSConn_AsyncWriter_Closed(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	_, err := client.AsyncWriter(context.Background(), MessageText)
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}
}

func TestWSConn_WriteAsync(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	testData := []byte("write async test")
	err := client.WriteAsync(ctx, MessageText, testData)
	if err != nil {
		t.Fatalf("WriteAsync failed: %v", err)
	}

	// Read back the echo
	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected data %s, got %s", testData, reader.Bytes())
	}
}

func TestWSConn_WriteFromReader(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	buf := client.bufferPool.Get()
	testData := []byte("write from reader test")
	buf.Write(testData)
	reader := &wsReader{
		Buffer:     buf,
		bufferPool: client.bufferPool,
		msgType:    MessageText,
	}

	err := client.WriteFromReader(ctx, reader)
	if err != nil {
		t.Fatalf("WriteFromReader failed: %v", err)
	}

	// Read back the echo
	echoReader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer echoReader.Done()

	if !bytes.Equal(echoReader.Bytes(), testData) {
		t.Errorf("Expected data %s, got %s", testData, echoReader.Bytes())
	}
}

func TestWSConn_Reader_Closed(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	_, err := client.Reader(context.Background())
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}
}

func TestWSConn_Ping_ContextCanceled(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.Ping(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestWSConn_Write_Closed(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	err := client.Write(context.Background(), MessageText, []byte("test"))
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}
}

func TestWSWriter_ReadFrom(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()
	writer, err := client.Writer(ctx, MessageText)
	if err != nil {
		t.Fatalf("Writer failed: %v", err)
	}

	testData := "readfrom test data"
	n, err := writer.(*wsWriter).ReadFrom(strings.NewReader(testData))
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if n != int64(len(testData)) {
		t.Errorf("Expected %d bytes, got %d", len(testData), n)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()

	if !bytes.Equal(reader.Bytes(), []byte(testData)) {
		t.Errorf("Expected %s, got %s", testData, reader.Bytes())
	}
}

func TestWSWriter_MessageType(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	writer, err := client.Writer(context.Background(), MessageBinary)
	if err != nil {
		t.Fatalf("Writer failed: %v", err)
	}

	ww := writer.(*wsWriter)
	if ww.MessageType() != MessageBinary {
		t.Errorf("Expected MessageBinary, got %v", ww.MessageType())
	}
	// Close writer to clean up
	ww.Write([]byte("x"))
	writer.Close()
}

func TestWSConn_SetDeadline_WithDeadline(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	testData := []byte("deadline test")
	err := client.Write(ctx, MessageText, testData)
	if err != nil {
		t.Fatalf("Write with deadline failed: %v", err)
	}

	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader with deadline failed: %v", err)
	}
	defer reader.Done()
	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected %s, got %s", testData, reader.Bytes())
	}
}

func TestAccept_WithOptions(t *testing.T) {
	subproto := "test-proto"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, WithAcceptSubprotocols(subproto), WithAcceptBufferPoolOpt(testBufferPool))
		if err != nil {
			t.Errorf("Accept with opts failed: %v", err)
			return
		}
		defer conn.Close(StatusNormalClosure, "")
		if conn.Subprotocol() != subproto {
			t.Errorf("Expected subprotocol %s, got %s", subproto, conn.Subprotocol())
		}
		// Echo one message
		reader, err := conn.Reader(context.Background())
		if err != nil {
			return
		}
		data := make([]byte, reader.Len())
		copy(data, reader.Bytes())
		msgType := reader.MessageType()
		reader.Done()
		conn.Write(context.Background(), msgType, data)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := websocket.Dialer{Subprotocols: []string{subproto}}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.WriteMessage(MessageText, []byte("hello"))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(msg, []byte("hello")) {
		t.Errorf("Expected hello, got %s", msg)
	}
}

func TestDial_WithSubprotocols(t *testing.T) {
	subproto := "my-protocol"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin:  func(*http.Request) bool { return true },
			Subprotocols: []string{subproto},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		defer conn.Close()
		// Hold connection open
		conn.ReadMessage()
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, resp, err := Dial(context.Background(), url, WithDialSubprotocols(subproto), WithDialBufferPoolOpt(testBufferPool))
	if err != nil {
		t.Fatalf("Dial with subprotocols failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	defer client.Close(StatusNormalClosure, "")

	if client.Subprotocol() != subproto {
		t.Errorf("Expected subprotocol %s, got %s", subproto, client.Subprotocol())
	}
}

func TestWSConn_newWriter_Closed(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	_, err := client.newWriter(context.Background(), MessageText, true)
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error from newWriter, got %v", err)
	}
}

func TestWSConn_ReadJSON_Closed(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	var v map[string]any
	err := client.ReadJSON(context.Background(), &v)
	if err == nil {
		t.Fatal("Expected error from ReadJSON on closed connection, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get reader for json message") {
		t.Errorf("Expected 'failed to get reader' error, got: %v", err)
	}
}

func TestDial_WithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.WriteMessage(mt, msg)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, resp, err := Dial(context.Background(), url, WithDialHTTPHeader(http.Header{"X-Test": []string{"value"}}), WithDialBufferPoolOpt(testBufferPool))
	if err != nil {
		t.Fatalf("Dial with options failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	defer client.Close(StatusNormalClosure, "")

	testData := []byte("options test")
	if err := client.Write(context.Background(), MessageText, testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	reader, err := client.Reader(context.Background())
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()
	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected %s, got %s", testData, reader.Bytes())
	}
}

func TestWSConn_Stats_ReadWrite(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()

	// Stats should start with zero messages
	stats := client.GetStats()
	if stats.MessagesReceived != 0 || stats.MessagesSent != 0 {
		t.Fatal("expected zero messages initially")
	}
	if stats.ConnectedAt.IsZero() {
		t.Fatal("expected non-zero ConnectedAt")
	}

	// Write a message and read it back (echo)
	testData := []byte("stats test")
	if err := client.Write(ctx, MessageText, testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader, err := client.Reader(ctx)
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	reader.Done()

	stats = client.GetStats()
	if stats.MessagesSent != 1 {
		t.Errorf("expected 1 message sent, got %d", stats.MessagesSent)
	}
	if stats.BytesSent != int64(len(testData)) {
		t.Errorf("expected %d bytes sent, got %d", len(testData), stats.BytesSent)
	}
	if stats.MessagesReceived != 1 {
		t.Errorf("expected 1 message received, got %d", stats.MessagesReceived)
	}
	if stats.BytesReceived != int64(len(testData)) {
		t.Errorf("expected %d bytes received, got %d", len(testData), stats.BytesReceived)
	}
	if stats.LastSentAt.IsZero() {
		t.Error("expected non-zero LastSentAt")
	}
	if stats.LastReceivedAt.IsZero() {
		t.Error("expected non-zero LastReceivedAt")
	}
}

func TestWSConn_Stats_MultipleMessages(t *testing.T) {
	client := setupEchoPair(t)
	defer client.Close(StatusNormalClosure, "")

	ctx := context.Background()

	for i := range 3 {
		data := fmt.Appendf(nil, "msg %d", i)
		if err := client.Write(ctx, MessageText, data); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
		reader, err := client.Reader(ctx)
		if err != nil {
			t.Fatalf("Reader %d failed: %v", i, err)
		}
		reader.Done()
	}

	stats := client.GetStats()
	if stats.MessagesSent != 3 {
		t.Errorf("expected 3 messages sent, got %d", stats.MessagesSent)
	}
	if stats.MessagesReceived != 3 {
		t.Errorf("expected 3 messages received, got %d", stats.MessagesReceived)
	}
}

func TestWSConn_WithConnectedAtOpt(t *testing.T) {
	connTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		srvConn := NewConn(conn)
		defer srvConn.Close(StatusNormalClosure, "")
		// echo
		reader, err := srvConn.Reader(context.Background())
		if err != nil {
			return
		}
		srvConn.Write(context.Background(), reader.MessageType(), reader.Bytes())
		reader.Done()
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	client := NewConn(conn, WithConnectedAtOpt(connTime))
	defer client.Close(StatusNormalClosure, "")

	stats := client.GetStats()
	if !stats.ConnectedAt.Equal(connTime) {
		t.Errorf("expected ConnectedAt %v, got %v", connTime, stats.ConnectedAt)
	}
}

func TestDial_WithCompression(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r,
			WithAcceptCompressionThreshold(true),
			WithAcceptBufferPoolOpt(testBufferPool),
		)
		if err != nil {
			t.Errorf("Accept failed: %v", err)
			return
		}
		defer conn.Close(StatusNormalClosure, "")
		reader, err := conn.Reader(context.Background())
		if err != nil {
			return
		}
		data := make([]byte, reader.Len())
		copy(data, reader.Bytes())
		msgType := reader.MessageType()
		reader.Done()
		conn.Write(context.Background(), msgType, data)
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, resp, err := Dial(context.Background(), url,
		WithDialCompression(true),
		WithDialBufferPoolOpt(testBufferPool),
	)
	if err != nil {
		t.Fatalf("Dial with compression failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	defer client.Close(StatusNormalClosure, "")

	testData := []byte("compression test")
	if err := client.Write(context.Background(), MessageText, testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	reader, err := client.Reader(context.Background())
	if err != nil {
		t.Fatalf("Reader failed: %v", err)
	}
	defer reader.Done()
	if !bytes.Equal(reader.Bytes(), testData) {
		t.Errorf("Expected %s, got %s", testData, reader.Bytes())
	}
}

func TestAccept_OptionError(t *testing.T) {
	badOpt := func(_ *acceptConfig) error {
		return errors.New("bad accept option")
	}

	rw := &failResponseWriter{}
	r := &http.Request{Method: http.MethodGet, Header: make(http.Header)}
	conn, err := Accept(rw, r, badOpt)
	if err == nil {
		t.Fatal("Expected error from bad option")
	}
	if conn != nil {
		t.Error("Expected nil conn from bad option")
	}
	if !strings.Contains(err.Error(), "bad accept option") {
		t.Errorf("Expected 'bad accept option' error, got: %v", err)
	}
}

func TestDial_OptionError(t *testing.T) {
	badOpt := func(_ *dialConfig) error {
		return errors.New("bad dial option")
	}

	conn, resp, err := Dial(context.Background(), "ws://localhost", badOpt)
	if err == nil {
		t.Fatal("Expected error from bad option")
	}
	if conn != nil {
		t.Error("Expected nil conn from bad option")
	}
	if resp != nil {
		resp.Body.Close()
		t.Error("Expected nil resp from bad option")
	}
	if !strings.Contains(err.Error(), "bad dial option") {
		t.Errorf("Expected 'bad dial option' error, got: %v", err)
	}
}

func TestWSConn_WriteJSON_ClosedConnection(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	err := client.WriteJSON(context.Background(), map[string]string{"key": "value"})
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}
}

func TestWSConn_WriteJSONAsync_ClosedConnection(t *testing.T) {
	client := setupEchoPair(t)
	client.Close(StatusNormalClosure, "")

	err := client.WriteJSONAsync(context.Background(), map[string]string{"key": "value"})
	if !IsErrWebsocketClosed(err) {
		t.Errorf("Expected websocket closed error, got %v", err)
	}
}

func TestWSConn_StartPingLoop_SendsPings(t *testing.T) {
	pingReceived := make(chan struct{}, 8)

	client := setupPair(t, func(_ *testing.T, server *Conn) {
		server.StartPingLoop(context.Background(), 20*time.Millisecond, 100*time.Millisecond)
		// Read so the server processes pong control frames and stays alive.
		for {
			reader, err := server.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	})
	defer client.Close(StatusNormalClosure, "")

	client.conn.SetPingHandler(func(string) error {
		select {
		case pingReceived <- struct{}{}:
		default:
		}
		return nil
	})

	// The client must be reading for the underlying conn to process ping control frames.
	go func() {
		for {
			reader, err := client.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	}()

	select {
	case <-pingReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("no ping received from StartPingLoop within 2s")
	}
}

func TestWSConn_StartPingLoop_PongExtendsDeadline(t *testing.T) {
	// interval+pongWait = 80ms read deadline. The client auto-responds to pings with
	// pongs (default handler), which must extend the server's read deadline well past
	// 80ms so the data message sent at 200ms is still received.
	got := make(chan []byte, 1)
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		server.EnableReadTimeout(30*time.Millisecond, 50*time.Millisecond)
		server.StartPingLoop(context.Background(), 30*time.Millisecond, 50*time.Millisecond)
		reader, err := server.Reader(context.Background())
		if err != nil {
			return
		}
		data := append([]byte(nil), reader.Bytes()...)
		reader.Done()
		got <- data
	})
	defer client.Close(StatusNormalClosure, "")

	// Client read loop processes incoming pings and auto-sends pongs.
	go func() {
		for {
			reader, err := client.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	}()

	// Send data well after the 80ms read deadline; only pong-driven extension keeps
	// the server's reader alive long enough to receive it.
	time.Sleep(200 * time.Millisecond)
	if err := client.Write(context.Background(), MessageText, []byte("alive")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case data := <-got:
		if string(data) != "alive" {
			t.Errorf("server received %q, want %q", data, "alive")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server read timed out — pong did not extend the read deadline")
	}
}

func TestWSConn_StartPingLoop_StopsOnContextCancel(t *testing.T) {
	pings := make(chan struct{}, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		server.StartPingLoop(ctx, 20*time.Millisecond, 100*time.Millisecond)
		close(started)
		for {
			reader, err := server.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	})
	defer client.Close(StatusNormalClosure, "")

	client.conn.SetPingHandler(func(string) error {
		select {
		case pings <- struct{}{}:
		default:
		}
		return nil
	})
	go func() {
		for {
			reader, err := client.Reader(context.Background())
			if err != nil {
				return
			}
			reader.Done()
		}
	}()

	<-started

	select {
	case <-pings:
	case <-time.After(2 * time.Second):
		t.Fatal("no ping received before cancel")
	}

	cancel()

	// Allow the loop to observe cancellation and drain any in-flight ping.
	time.Sleep(100 * time.Millisecond)
	for draining := true; draining; {
		select {
		case <-pings:
		default:
			draining = false
		}
	}

	select {
	case <-pings:
		t.Fatal("ping received after context cancel")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWSConn_Reader_ExitsOnExpiredReadDeadline(t *testing.T) {
	// Verify that setting an expired read deadline on the underlying net.Conn
	// causes a blocked Reader() call to return immediately. This is the mechanism
	// used by HTTPServer.Shutdown to unblock handler goroutines stuck in reads.
	client := setupPair(t, func(_ *testing.T, server *Conn) {
		// Server sends no data, so the client blocks in Reader waiting for it.
		// Block on a read (rather than a channel nothing closes) so this
		// goroutine — and the server Conn it owns — exit cleanly when the
		// client is torn down during cleanup, instead of leaking per -count run.
		_, _ = server.Reader(context.Background())
	})

	done := make(chan error, 1)
	go func() {
		_, err := client.Reader(context.Background())
		done <- err
	}()

	// Confirm Reader is blocked (no data coming from server)
	select {
	case <-done:
		t.Fatal("Reader returned before deadline was set")
	case <-time.After(50 * time.Millisecond):
	}

	// Set an expired read deadline — this should unblock Reader immediately
	client.SetReadDeadline(time.Now())

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Reader returned nil error after expired deadline")
		}
		var netErr interface{ Timeout() bool }
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("expected timeout error, got: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Reader did not exit within 1s after expired read deadline")
	}
}
