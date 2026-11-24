package jonson

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// WsTestSystem exposes a method that blocks until released,
// so tests can observe how many messages are processed concurrently.
type WsTestSystem struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	release     chan struct{}
}

type BlockV1Params struct {
	Params
}

func (s *WsTestSystem) BlockV1(ctx *Context, params *BlockV1Params) (bool, error) {
	n := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		m := s.maxInFlight.Load()
		if n <= m || s.maxInFlight.CompareAndSwap(m, n) {
			break
		}
	}
	<-s.release
	return true, nil
}

type wsTestEnv struct {
	sys *WsTestSystem
	// handlerDone receives once the websocket handler returned (connection fully torn down)
	handlerDone chan struct{}
	conn        *websocket.Conn
	// responses receives every message the server sends to the client
	responses chan map[string]any
}

func newWsTestEnv(t *testing.T, opts *WebsocketOptions) *wsTestEnv {
	t.Helper()
	sys := &WsTestSystem{release: make(chan struct{})}
	methodHandler := NewMethodHandler(NewFactory(), NewDebugSecret(), nil)
	methodHandler.RegisterSystem(sys)
	wsHandler := NewWebsocketHandler(methodHandler, "/ws", opts)

	env := &wsTestEnv{
		sys:         sys,
		handlerDone: make(chan struct{}, 1),
		responses:   make(chan map[string]any, 1024),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsHandler.Handle(w, r)
		env.handlerDone <- struct{}{}
	}))
	t.Cleanup(srv.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	env.conn = conn

	// the client needs to keep reading, otherwise pings are never answered
	go func() {
		for {
			_, b, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				panic(err)
			}
			env.responses <- m
		}
	}()
	return env
}

func (e *wsTestEnv) sendBlock(t *testing.T, id int) {
	t.Helper()
	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ws-test-system/block.v1","params":{}}`, id)
	if err := e.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls cond until it holds or the timeout is reached
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// expectResponses collects n responses and fails on any error response
func (e *wsTestEnv) expectResponses(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	for i := range n {
		select {
		case m := <-e.responses:
			if m["error"] != nil {
				t.Fatalf("response %d: unexpected error: %v", i, m["error"])
			}
		case <-time.After(timeout):
			t.Fatalf("got %d of %d responses", i, n)
		}
	}
}

func TestWebsocketHandler(t *testing.T) {
	t.Run("defaults MaxConcurrentMessages when zero", func(t *testing.T) {
		opts := NewWebsocketOptions()
		opts.MaxConcurrentMessages = 0
		NewWebsocketHandler(NewMethodHandler(NewFactory(), NewDebugSecret(), nil), "/ws", opts)
		if opts.MaxConcurrentMessages != DefaultMaxConcurrentMessages {
			t.Fatalf("expected %d, got %d", DefaultMaxConcurrentMessages, opts.MaxConcurrentMessages)
		}
	})

	t.Run("never exceeds MaxConcurrentMessages and answers every message instead of rejecting", func(t *testing.T) {
		opts := NewWebsocketOptions()
		opts.MaxConcurrentMessages = 2
		env := newWsTestEnv(t, opts)

		const total = 20
		for i := range total {
			env.sendBlock(t, i)
		}

		waitFor(t, time.Second, "limit to be reached", func() bool { return env.sys.inFlight.Load() == 2 })
		// give the server a chance to (wrongly) start more work
		time.Sleep(50 * time.Millisecond)
		if got := env.sys.maxInFlight.Load(); got != 2 {
			t.Fatalf("expected max 2 messages in flight, got %d", got)
		}
		// nothing must have been rejected while at the limit
		select {
		case m := <-env.responses:
			t.Fatalf("expected no response while blocked, got: %v", m)
		default:
		}

		close(env.sys.release)
		env.expectResponses(t, total, 2*time.Second)
		if got := env.sys.maxInFlight.Load(); got != 2 {
			t.Fatalf("expected max 2 messages in flight, got %d", got)
		}
	})

	t.Run("does not spawn a goroutine per queued message", func(t *testing.T) {
		opts := NewWebsocketOptions()
		opts.MaxConcurrentMessages = 2
		env := newWsTestEnv(t, opts)

		env.sendBlock(t, 0)
		env.sendBlock(t, 1)
		waitFor(t, time.Second, "limit to be reached", func() bool { return env.sys.inFlight.Load() == 2 })
		base := runtime.NumGoroutine()

		// flood the connection; write from a goroutine since backpressure may block the client
		const flood = 500
		go func() {
			for i := range flood {
				msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ws-test-system/block.v1","params":{}}`, i+2)
				if env.conn.WriteMessage(websocket.TextMessage, []byte(msg)) != nil {
					return
				}
			}
		}()
		time.Sleep(200 * time.Millisecond)

		// allow some slack for runtime/test goroutines, but nowhere near one per message
		if grown := runtime.NumGoroutine() - base; grown > 20 {
			t.Fatalf("goroutines grew by %d while flooding with %d messages", grown, flood)
		}
		close(env.sys.release)
		env.expectResponses(t, flood+2, 5*time.Second)
	})

	t.Run("keeps a connection alive that waits longer than PongWait for a free slot", func(t *testing.T) {
		opts := NewWebsocketOptions()
		opts.MaxConcurrentMessages = 1
		opts.PongWait = 300 * time.Millisecond
		opts.PingPeriod = 100 * time.Millisecond
		env := newWsTestEnv(t, opts)

		env.sendBlock(t, 0)
		env.sendBlock(t, 1) // read by the server, then waits for the single slot
		waitFor(t, time.Second, "first message to be processed", func() bool { return env.sys.inFlight.Load() == 1 })

		// block well beyond PongWait; the reader is stuck on the limiter and can't process pongs
		time.Sleep(3 * opts.PongWait)
		close(env.sys.release)
		env.expectResponses(t, 2, 2*time.Second)

		// connection must still be usable
		env.sendBlock(t, 2)
		env.expectResponses(t, 1, 2*time.Second)
	})

	t.Run("tears down connection promptly once the client disconnects", func(t *testing.T) {
		opts := NewWebsocketOptions()
		opts.MaxConcurrentMessages = 2
		env := newWsTestEnv(t, opts)

		for i := range 5 {
			env.sendBlock(t, i)
		}
		waitFor(t, time.Second, "limit to be reached", func() bool { return env.sys.inFlight.Load() == 2 })

		env.conn.Close()
		close(env.sys.release)

		// default PingPeriod is 54s; handler must not wait for the next ping to notice
		select {
		case <-env.handlerDone:
		case <-time.After(2 * time.Second):
			t.Fatal("websocket handler did not return after client disconnect")
		}
		waitFor(t, time.Second, "in-flight work to drain", func() bool { return env.sys.inFlight.Load() == 0 })
	})

	t.Run("tears down idle connection promptly once the client disconnects", func(t *testing.T) {
		env := newWsTestEnv(t, NewWebsocketOptions())
		env.conn.Close()

		select {
		case <-env.handlerDone:
		case <-time.After(2 * time.Second):
			t.Fatal("websocket handler did not return after client disconnect")
		}
	})
}

func TestWebsocketHandlerRejectsGet(t *testing.T) {
	testProvider := NewTestProvider()
	testProvider.setLoggedIn(true)
	factory := NewFactory()
	factory.RegisterProvider(testProvider)
	factory.RegisterProvider(NewTimeProvider(func() Time { return newMockTime(time.Now()) }))
	mh := NewMethodHandler(factory, NewDebugSecret(), nil)
	mh.RegisterSystem(NewTestSystem())

	srv := httptest.NewServer(NewServer(NewWebsocketHandler(mh, "/ws", NewWebsocketOptions())))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	call := func(method, params string) map[string]any {
		t.Helper()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`)); err != nil {
			t.Fatal(err)
		}
		var resp map[string]any
		if err := conn.ReadJSON(&resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := call("test-system/current-time.v1", "{}")
	if e, _ := resp["error"].(map[string]any); e == nil || e["code"] != float64(ErrServerMethodNotAllowed.Code) {
		t.Errorf("GET method: got %v, want method not allowed", resp)
	}
	resp = call("test-system/get-profile.v1", `{"uuid":"`+testAccountUuid+`"}`)
	if resp["error"] != nil {
		t.Errorf("POST method: got %v, want result", resp)
	}
}
