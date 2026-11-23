package jonson

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

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
