package jonson

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var TypeWSClient = reflect.TypeOf((**WSClient)(nil)).Elem()

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
	Upgrader       *websocket.Upgrader
	MaxMessageSize int64
	PingPeriod     time.Duration
	PongWait       time.Duration
	WriteWait      time.Duration
}

func NewWebsocketOptions() *WebsocketOptions {
	return &WebsocketOptions{
		Upgrader: &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		WriteWait:      10 * time.Second,
		PongWait:       60 * time.Second,
		PingPeriod:     (60 * time.Second * 9) / 10,
		MaxMessageSize: 1 << 22,
	}
}

func NewWebsocketHandler(
	methodHandler *MethodHandler,
	path string,
	options *WebsocketOptions,
) *WebsocketHandler {

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
		wb.methodHandler.logger.Warn("websocketHandler.Handle", err)
		return true
	}
	client := NewWSClient(wb, wb.methodHandler, conn, req)
	client.run()
	return true
}

type WSClient struct {
	Shareable
	ws            *WebsocketHandler
	methodHandler *MethodHandler
	conn          *websocket.Conn
	httpRequest   *http.Request
	send          chan []byte
}

func NewWSClient(ws *WebsocketHandler, methodHandler *MethodHandler, conn *websocket.Conn, r *http.Request) *WSClient {
	return &WSClient{
		ws:            ws,
		methodHandler: methodHandler,
		conn:          conn,
		httpRequest:   r,
		send:          make(chan []byte, 512),
	}
}

func (w *WSClient) run() {
	go w.reader()
	// we need to keep the run method blocking
	w.writer()
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

	for {
		messageType, p, err := w.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, 1001, 1005, 1006) {
				w.methodHandler.logger.Warn("wsClient.read", err)
			}
			return
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			go func() {
				resp, batch := w.methodHandler.processRpcMessages(RpcSourceWs, w.httpRequest, nil, w, p)

				if len(resp) == 0 {
					// nothing to return but obviously everything was ok
					return
				}

				if !batch {
					// single response
					b, _ := json.Marshal(resp[0])
					w.send <- b
					return
				}

				// batch response
				b, _ := json.Marshal(resp)
				w.send <- b
			}()
		}
	}
}

func (w *WSClient) writer() {
	ticker := time.NewTicker(w.ws.options.PingPeriod)
	defer func() {
		ticker.Stop()
		w.conn.Close()
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
					w.methodHandler.logger.Warn("wsClient.writer", err)
				}
				return
			}

		case <-ticker.C:
			w.conn.SetWriteDeadline(time.Now().Add(w.ws.options.WriteWait))
			if err := w.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if err != websocket.ErrCloseSent && !errors.Is(err, net.ErrClosed) {
					w.methodHandler.logger.Warn("wsClient.writer", err)
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

	raw, _ := json.Marshal(msg)
	w.send <- raw
	return
}

// IPAddress returns the request's ip address
func IPAddress(r *http.Request) string {
	//gets comma-space separated forwarding list (client, proxy1, proxy2, ...)
	//Note: the X-FORWARDED-FOR header can be set by the client so this assumes we are using a trusted proxy that
	//strips this header from client requests
	ipsStr := r.Header.Get("X-FORWARDED-FOR")
	ips := strings.SplitN(ipsStr, ", ", 1)

	//return first client if present
	if len(ips) > 0 {
		return ips[0]
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
