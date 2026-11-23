package jonson

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type McpTestSystem struct {
	lastSource RpcSource
}

type McpGreetV1Params struct {
	Params
	Name  string `json:"name" description:"who to greet"`
	Times int    `json:"times,omitempty"`
}

func (p *McpGreetV1Params) JonsonValidate(v *Validator) {
	if p.Name == "" {
		v.Path("name").Code(10000).Message("name is required")
	}
}

type McpGreetV1Result struct {
	Greeting string `json:"greeting"`
}

func (s *McpTestSystem) GreetV1(ctx *Context, p *McpGreetV1Params) (*McpGreetV1Result, error) {
	s.lastSource = RequireRpcMeta(ctx).Source
	return &McpGreetV1Result{Greeting: strings.Repeat("hello "+p.Name+" ", max(p.Times, 1))}, nil
}

func (s *McpTestSystem) FailV1(ctx *Context) error {
	return errors.New("db down")
}

func (s *McpTestSystem) NamesV1(ctx *Context) ([]string, error) {
	return []string{"a", "b"}, nil
}

func (s *McpTestSystem) ReadV1(ctx *Context, _ HttpGet) error {
	return nil
}

func (s *McpTestSystem) HiddenV1(ctx *Context) error {
	return nil
}

func newMcpTestServer(t *testing.T) (http.Handler, *McpTestSystem) {
	t.Helper()
	sys := &McpTestSystem{}
	mh := NewMethodHandler(NewFactory(), NewDebugSecret(), nil)
	mh.RegisterSystem(sys)
	mcp := NewMcpHandler(mh, "/mcp").
		Expose(sys.GreetV1, "Greets someone.").
		Expose(sys.FailV1, "Always fails.").
		ExposeMethod("mcp-test-system/names.v1", "Lists names.")
	return NewServer(mcp), sys
}

// mcpPost sends body to /mcp and returns the status code and the decoded JSON-RPC response.
func mcpPost(t *testing.T, srv http.Handler, body string, header ...string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Body.Len() == 0 {
		return w.Code, nil
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, w.Body.String())
	}
	return w.Code, out
}

func mcpCall(t *testing.T, srv http.Handler, tool string, args string) map[string]any {
	t.Helper()
	code, resp := mcpPost(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+tool+`","arguments":`+args+`}}`)
	if code != http.StatusOK || resp["result"] == nil {
		t.Fatalf("tools/call %s = %d %v, want a result", tool, code, resp)
	}
	return resp["result"].(map[string]any)
}

func TestMcpHandler_Initialize(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	tests := []struct {
		name   string
		client string
		want   string
	}{
		{name: "supported version is echoed", client: "2025-06-18", want: "2025-06-18"},
		{name: "unknown version gets the newest", client: "1999-01-01", want: McpProtocolVersions[0]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, resp := mcpPost(t, srv, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+tc.client+`","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
			result := resp["result"].(map[string]any)
			if got := result["protocolVersion"]; got != tc.want {
				t.Errorf("protocolVersion = %v, want %v", got, tc.want)
			}
			if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
				t.Errorf("capabilities = %v, want tools", result["capabilities"])
			}
		})
	}
}

func TestMcpHandler_Ping(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	_, resp := mcpPost(t, srv, `{"jsonrpc":"2.0","id":"p","method":"ping"}`)
	if resp["id"] != "p" || resp["result"] == nil {
		t.Errorf("ping = %v, want an empty result with id p", resp)
	}
}

func TestMcpHandler_ToolsList(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	_, resp := mcpPost(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := resp["result"].(map[string]any)["tools"].([]any)
	var names []string
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	if want := []string{"mcp-test-system_greet_v1", "mcp-test-system_fail_v1", "mcp-test-system_names_v1"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v (hidden.v1 is not exposed)", names, want)
	}
	greet := tools[0].(map[string]any)
	if greet["description"] != "Greets someone." {
		t.Errorf("description = %v", greet["description"])
	}
	schema, _ := json.Marshal(greet["inputSchema"])
	want := `{"additionalProperties":false,"properties":{"name":{"description":"who to greet","type":"string"},"times":{"type":"integer"}},"required":["name"],"type":"object"}`
	if string(schema) != want {
		t.Errorf("inputSchema = %s, want %s", schema, want)
	}
	if fail, _ := json.Marshal(tools[1].(map[string]any)["inputSchema"]); string(fail) != `{"type":"object"}` {
		t.Errorf("inputSchema without params = %s, want {\"type\":\"object\"}", fail)
	}
}

func TestMcpHandler_ToolsCall(t *testing.T) {
	srv, sys := newMcpTestServer(t)

	t.Run("success has structured content", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_greet_v1", `{"name":"bob"}`)
		if res["isError"] != nil {
			t.Fatalf("isError = %v, want unset", res["isError"])
		}
		if got := res["structuredContent"].(map[string]any)["greeting"]; got != "hello bob " {
			t.Errorf("greeting = %q", got)
		}
		text := res["content"].([]any)[0].(map[string]any)["text"]
		if text != `{"greeting":"hello bob "}` {
			t.Errorf("text = %v", text)
		}
		if sys.lastSource != RpcSourceMcp {
			t.Errorf("RpcMeta.Source = %q, want %q", sys.lastSource, RpcSourceMcp)
		}
	})

	t.Run("validation error is a tool error", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_greet_v1", `{"name":""}`)
		text := res["content"].([]any)[0].(map[string]any)["text"].(string)
		if res["isError"] != true || !strings.Contains(text, "name is required") {
			t.Errorf("result = %v, want isError with the validation message", res)
		}
	})

	t.Run("unknown argument is a tool error", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_greet_v1", `{"name":"bob","age":3}`)
		if res["isError"] != true {
			t.Errorf("result = %v, want isError", res)
		}
	})

	t.Run("internal error is encoded by the secret", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_fail_v1", `{}`)
		text := res["content"].([]any)[0].(map[string]any)["text"].(string)
		if res["isError"] != true || !strings.Contains(text, "-32603") || !strings.Contains(text, "db down") {
			t.Errorf("result = %v, want isError with the internal error (debug secret)", res)
		}
	})

	t.Run("missing arguments become an empty object", func(t *testing.T) {
		_, resp := mcpPost(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mcp-test-system_fail_v1"}}`)
		if resp["result"].(map[string]any)["isError"] != true {
			t.Errorf("result = %v, want the call to run and fail", resp)
		}
	})

	t.Run("non-object result has text only", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_names_v1", `{}`)
		if _, ok := res["structuredContent"]; ok {
			t.Errorf("structuredContent = %v, want none for an array", res["structuredContent"])
		}
		if text := res["content"].([]any)[0].(map[string]any)["text"]; text != `["a","b"]` {
			t.Errorf("text = %v", text)
		}
	})

	t.Run("unknown tool is a protocol error", func(t *testing.T) {
		_, resp := mcpPost(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mcp-test-system_hidden_v1","arguments":{}}}`)
		if code := resp["error"].(map[string]any)["code"]; code != float64(ErrInvalidParams.Code) {
			t.Errorf("error code = %v, want %d", code, ErrInvalidParams.Code)
		}
	})
}

func TestMcpHandler_Transport(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	tests := []struct {
		name      string
		method    string
		body      string
		header    []string
		wantCode  int
		wantError int
	}{
		{name: "notification is accepted", method: http.MethodPost, body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`, wantCode: http.StatusAccepted},
		{name: "unknown method", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, wantCode: http.StatusOK, wantError: ErrMethodNotFound.Code},
		{name: "batch is rejected", method: http.MethodPost, body: `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, wantCode: http.StatusOK, wantError: -32600},
		{name: "broken json", method: http.MethodPost, body: `{"jsonrpc":`, wantCode: http.StatusOK, wantError: ErrParse.Code},
		{name: "GET has no event stream", method: http.MethodGet, wantCode: http.StatusMethodNotAllowed},
		{name: "foreign origin is forbidden", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"method":"ping"}`, header: []string{"Origin", "https://evil.example"}, wantCode: http.StatusForbidden},
		{name: "same origin is served", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"method":"ping"}`, header: []string{"Origin", "http://example.com"}, wantCode: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://example.com/mcp", strings.NewReader(tc.body))
			for i := 0; i+1 < len(tc.header); i += 2 {
				req.Header.Set(tc.header[i], tc.header[i+1])
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantCode, w.Body.String())
			}
			if tc.wantError == 0 {
				return
			}
			var resp RpcErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Error == nil || resp.Error.Code != tc.wantError {
				t.Errorf("response = %s, want error %d", w.Body.String(), tc.wantError)
			}
		})
	}
}

type McpUnregisteredSystem struct{}

func (s *McpUnregisteredSystem) GreetV1(ctx *Context) error { return nil }

func mcpPlainFuncV1(ctx *Context) error { return nil }

func TestMcpHandler_Expose(t *testing.T) {
	sys := &McpTestSystem{}
	mh := NewMethodHandler(NewFactory(), NewDebugSecret(), nil)
	mh.RegisterSystem(sys)

	if got, _ := methodValueName(sys.GreetV1); got != "mcp-test-system/greet.v1" {
		t.Errorf("methodValueName(sys.GreetV1) = %q", got)
	}

	tests := []struct {
		name   string
		expose func(h *McpHandler)
	}{
		{"unregistered system", func(h *McpHandler) { h.Expose((&McpUnregisteredSystem{}).GreetV1, "x") }},
		{"method expression", func(h *McpHandler) { h.Expose((*McpTestSystem).GreetV1, "x") }},
		{"plain func", func(h *McpHandler) { h.Expose(mcpPlainFuncV1, "x") }},
		{"closure", func(h *McpHandler) { h.Expose(func() {}, "x") }},
		{"not a func", func(h *McpHandler) { h.Expose("mcp-test-system/greet.v1", "x") }},
		{"empty description", func(h *McpHandler) { h.Expose(sys.GreetV1, " ") }},
		{"GET method", func(h *McpHandler) { h.Expose(sys.ReadV1, "x") }},
		{"exposed twice", func(h *McpHandler) { h.Expose(sys.GreetV1, "x").ExposeMethod("mcp-test-system/greet.v1", "x") }},
		{"unknown method name", func(h *McpHandler) { h.ExposeMethod("mcp-test-system/nope.v1", "x") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("did not panic")
				}
			}()
			tc.expose(NewMcpHandler(mh, "/mcp"))
		})
	}
}

type mcpSchemaNode struct {
	Children []*mcpSchemaNode `json:"children"`
}

type mcpSchemaEmbedded struct {
	Inline string `json:"inline"`
}

type mcpSchemaParams struct {
	Params
	mcpSchemaEmbedded
	When     time.Time          `json:"when"`
	Tags     []string           `json:"tags,omitempty"`
	Labels   map[string]int     `json:"labels,omitzero"`
	Ptr      *float64           `json:"ptr,omitempty"`
	Raw      json.RawMessage    `json:"raw,omitempty"`
	Blob     []byte             `json:"blob,omitempty"`
	Count    int64              `json:"count,string"`
	Tree     *mcpSchemaNode     `json:"tree,omitempty"`
	Skipped  string             `json:"-"`
	Untagged bool               // no json tag: the Go field name is used
	unexp    string             // unexported: skipped
	Any      any                `json:"any,omitempty"`
	Nested   struct{ A string } `json:"nested,omitempty"`
}

type mcpLooseParams struct {
	Params
	Name string `json:"name"`
}

func (p *mcpLooseParams) JonsonAllowUnknownFields() {}

func TestMcpInputSchema(t *testing.T) {
	got, _ := json.Marshal(mcpInputSchema(reflect.TypeFor[mcpSchemaParams]()))
	want := `{"additionalProperties":false,"properties":{` +
		`"Untagged":{"type":"boolean"},` +
		`"any":{},` +
		`"blob":{"type":"string"},` +
		`"count":{"type":"string"},` +
		`"inline":{"type":"string"},` +
		`"labels":{"additionalProperties":{"type":"integer"},"type":"object"},` +
		`"nested":{"additionalProperties":false,"properties":{"A":{"type":"string"}},"required":["A"],"type":"object"},` +
		`"ptr":{"type":"number"},` +
		`"raw":{},` +
		`"tags":{"items":{"type":"string"},"type":"array"},` +
		`"tree":{"additionalProperties":false,"properties":{"children":{"items":{"type":"object"},"type":"array"}},"required":["children"],"type":"object"},` +
		`"when":{"format":"date-time","type":"string"}},` +
		`"required":["inline","when","count","Untagged"],"type":"object"}`
	if string(got) != want {
		t.Errorf("schema =\n%s\nwant\n%s", got, want)
	}

	loose, _ := json.Marshal(mcpInputSchema(reflect.TypeFor[mcpLooseParams]()))
	if strings.Contains(string(loose), "additionalProperties") {
		t.Errorf("schema with AllowUnknownFieldsParams = %s, want no additionalProperties", loose)
	}
}
