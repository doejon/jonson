package jonson

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var TypeWSClient = reflect.TypeFor[*WSClient]()

func RequireWSClient(ctx *Context) *WSClient {
	if v := ctx.Require(TypeWSClient); v != nil {
		return v.(*WSClient)
	}
	return nil
}

// The websocket handler allows us to provide
// websocket functionality to the server.
type WebsocketHandler struct {
	path          string
	methodHandler *MethodHandler
	options       *WebsocketOptions
}

type WebsocketOptions struct {
	Upgrader              *websocket.Upgrader
	MaxMessageSize        int64
	PingPeriod            time.Duration
	PongWait              time.Duration
	WriteWait             time.Duration
	MaxConcurrentMessages uint64
}

// DefaultMaxConcurrentMessages defines the max amount of concurrent messages per connection being processed in case not otherwise defined (== 0).
const DefaultMaxConcurrentMessages = 64

func NewWebsocketOptions() *WebsocketOptions {
	return &WebsocketOptions{
		Upgrader: &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		WriteWait:             10 * time.Second,
		PongWait:              60 * time.Second,
		PingPeriod:            (60 * time.Second * 9) / 10,
		MaxMessageSize:        1 << 22,
		MaxConcurrentMessages: DefaultMaxConcurrentMessages,
	}
}

func NewWebsocketHandler(
	methodHandler *MethodHandler,
	path string,
	options *WebsocketOptions,
) *WebsocketHandler {

	if options.MaxConcurrentMessages == 0 {
		options.MaxConcurrentMessages = DefaultMaxConcurrentMessages
	}

	return &WebsocketHandler{
		path:          path,
		methodHandler: methodHandler,
		options:       options,
	}
}

// Handle will compare the defined path within the websocket handler
// to the requested url path. In case paths match, a new websocket client will be registered.
func (wb *WebsocketHandler) Handle(w http.ResponseWriter, req *http.Request) bool {
	if req.URL.Path != wb.path {
		return false
	}

	conn, err := wb.options.Upgrader.Upgrade(w, req, nil)
	if err != nil {
		wb.methodHandler.getLogger(nil).Warn("websocket: failed to upgrade", "error", err)
		return true
	}
	client := NewWSClient(wb, wb.methodHandler, conn, req)
	client.run()
	return true
}

type WSClient struct {
	Shareable
	ShareableAcrossImpersonation
	ws            *WebsocketHandler
	methodHandler *MethodHandler
	conn          *websocket.Conn
	httpRequest   *http.Request
	send          chan []byte
	// done is closed once writer() returns upon a closed connection.
	// This allows sending goroutine to ublock and exit.
	done chan struct{}
}

func NewWSClient(ws *WebsocketHandler, methodHandler *MethodHandler, conn *websocket.Conn, r *http.Request) *WSClient {
	return &WSClient{
		ws:            ws,
		methodHandler: methodHandler,
		conn:          conn,
		httpRequest:   r,
		send:          make(chan []byte, 512),
		done:          make(chan struct{}),
	}
}

func (w *WSClient) run() {
	go w.reader()
	// we need to keep the run method blocking
	w.writer()
}

// make sure to abort sending in case the websocket has been closed

func (w *WSClient) sendMessage(b []byte) {
	select {
	case w.send <- b:
	case <-w.done:
	}
}
func (w *WSClient) reader() {
	defer func() {
		w.conn.Close()
	}()

	w.conn.SetReadLimit(w.ws.options.MaxMessageSize)
	w.conn.SetReadDeadline(time.Now().Add(w.ws.options.PongWait))
	w.conn.SetPongHandler(func(string) error {
		w.conn.SetReadDeadline(time.Now().Add(w.ws.options.PongWait))
		return nil
	})

	// make sure to not spawn too many goroutines in parallel, otherwise we might run into memory issues.
	// This is a semaphore channel that will limit the number of concurrent messages being processed.
	limiter := make(chan struct{}, int(w.ws.options.MaxConcurrentMessages))

	// make sure to abort sending in case the websocket has been closed

	for {
		messageType, p, err := w.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, 1001, 1005, 1006) {
				w.methodHandler.getLogger(nil).Warn("websocket: unexpected close error", "error", err)
			}
			return
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			go func() {
				select {
				case limiter <- struct{}{}:
				case <-w.done:
					// writer already shut down; stop spawning new work
					return
				default:
					// limit reached, abort
					errResp := NewRpcErrorResponse(nil, ErrTooManyRequests.CloneWithData(&ErrorData{
						Debug: fmt.Sprintf("max of %d parallel requests allowed per websocket connection", w.ws.options.MaxConcurrentMessages),
					}))
					// batch response
					b, _ := w.methodHandler.opts.JsonHandler.Marshal(errResp)
					w.sendMessage(b)
					return
				}
				// release limit
				defer func() { <-limiter }()

				// The initial call to processRpcMessages remains the same.
				resp, batch := w.methodHandler.processRpcMessages(RpcSourceWs, RpcHttpMethodPost, w.httpRequest, nil, w, p)

				if len(resp) == 0 {
					// nothing to return but obviously everything was ok
					return
				}

				var b []byte
				if !batch {
					// single response
					b, _ = w.methodHandler.opts.JsonHandler.Marshal(resp[0])

				} else {
					// batch response
					b, _ = w.methodHandler.opts.JsonHandler.Marshal(resp)
				}

				// make sure to abort sending in case the websocket has been closed
				w.sendMessage(b)
			}()
		}
	}
}

func (w *WSClient) writer() {
	ticker := time.NewTicker(w.ws.options.PingPeriod)
	defer func() {
		ticker.Stop()
		w.conn.Close()
		// unblock all goroutines (created by reader and sendNotification)
		close(w.done)
	}()

	for {
		select {
		case next, ok := <-w.send:
			w.conn.SetWriteDeadline(time.Now().Add(w.ws.options.WriteWait))
			if !ok {
				w.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := w.conn.WriteMessage(websocket.TextMessage, next); err != nil {
				if err != websocket.ErrCloseSent && !errors.Is(err, net.ErrClosed) {
					w.methodHandler.getLogger(nil).Warn("websocket: failed to write text message", "error", err)
				}
				return
			}

		case <-ticker.C:
			w.conn.SetWriteDeadline(time.Now().Add(w.ws.options.WriteWait))
			if err := w.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if err != websocket.ErrCloseSent && !errors.Is(err, net.ErrClosed) {
					w.methodHandler.getLogger(nil).Warn("websocket: failed to write ping message", "error", err)
				}
				return
			}
		}
	}
}

func (w *WSClient) SendNotification(msg *RpcNotification) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(error); ok {
				err = re
			} else {
				panic(r)
			}
		}
	}()

	raw, _ := w.methodHandler.opts.JsonHandler.Marshal(msg)
	w.sendMessage(raw)
	return
}

// IPAddress returns the request's ip address
// Note: we do look into X-Forwarded-For-Headers for trusted environments.
// For cases where the X-Forwarded-For-Header is malformed or otherwise
// not available, we fall back to RemoteAddr
func IPAddress(r *http.Request) string {
	//gets comma-space separated forwarding list (client, proxy1, proxy2, ...)
	//Note: the X-FORWARDED-FOR header can be set by the client so this assumes we are using a trusted proxy that
	//strips this header from client requests
	ipsStr := r.Header.Get("X-Forwarded-For")

	if ipsStr != "" {
		//return first client if present
		ip := strings.TrimSpace(strings.Split(ipsStr, ",")[0])
		if ip != "" {
			return ip
		}
	}

	//fallback to remote address
	return r.RemoteAddr
}

// IPAddress returns the underlying ip address which has been
// used when opening websocket connection
func (w *WSClient) IPAddress() string {
	return IPAddress(w.httpRequest)
}

// UserAgent returns the underlying user agent
// which was sent with the initial opening request
func (w *WSClient) UserAgent() string {
	return w.httpRequest.UserAgent()
}
