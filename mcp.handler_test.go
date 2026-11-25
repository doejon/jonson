package jonson

import (
	"encoding/json"
	"errors"
	"fmt"
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

func (r *McpGreetV1Result) JonsonMcpTextContent() string {
	return "Greeted: " + strings.TrimSpace(r.Greeting)
}

func (s *McpTestSystem) GreetV1(ctx *Context, p *McpGreetV1Params) (*McpGreetV1Result, error) {
	s.lastSource = RequireRpcMeta(ctx).Source
	return &McpGreetV1Result{Greeting: strings.Repeat("hello "+p.Name+" ", max(p.Times, 1))}, nil
}

func (*McpTestSystem) JonsonMethodDescriptions() map[string]string {
	return map[string]string{"mcp-test-system/fail.v1": "Always fails."}
}

func (s *McpTestSystem) FailV1(ctx *Context) error {
	return errors.New("db down")
}

type McpNamesV1Result struct {
	Names []string `json:"names"`
}

func (s *McpTestSystem) NamesV1(ctx *Context) (*McpNamesV1Result, error) {
	return &McpNamesV1Result{Names: []string{"a", "b"}}, nil
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
		Expose(sys.FailV1).
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

const mcpMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

// mcpRequest sends a 2026-07-28 request: params (object members without braces) get the
// required _meta, and the request gets the required headers, overridable by header.
func mcpRequest(t *testing.T, srv http.Handler, method, params string, header ...string) (int, map[string]any) {
	t.Helper()
	if params != "" {
		params += ","
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{` + params + mcpMeta + `}}`
	return mcpPost(t, srv, body, append([]string{"MCP-Protocol-Version", McpProtocolVersion, "Mcp-Method", method}, header...)...)
}

func mcpCall(t *testing.T, srv http.Handler, tool string, args string) map[string]any {
	t.Helper()
	code, resp := mcpRequest(t, srv, "tools/call", `"name":"`+tool+`","arguments":`+args, "Mcp-Name", tool)
	if code != http.StatusOK || resp["result"] == nil {
		t.Fatalf("tools/call %s = %d %v, want a result", tool, code, resp)
	}
	return resp["result"].(map[string]any)
}

// mcpCheckComplete fails unless result is a complete result carrying the server info.
func mcpCheckComplete(t *testing.T, result map[string]any) {
	t.Helper()
	if result["resultType"] != "complete" {
		t.Errorf("resultType = %v, want complete", result["resultType"])
	}
	if info := result["_meta"].(map[string]any)["io.modelcontextprotocol/serverInfo"].(map[string]any); info["name"] != "jonson" {
		t.Errorf("serverInfo = %v, want name jonson", info)
	}
}

func TestMcpHandler_Discover(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	_, resp := mcpRequest(t, srv, "server/discover", "")
	result := resp["result"].(map[string]any)
	mcpCheckComplete(t, result)
	if v := result["supportedVersions"].([]any); len(v) != 1 || v[0] != McpProtocolVersion {
		t.Errorf("supportedVersions = %v, want [%s]", v, McpProtocolVersion)
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("capabilities = %v, want tools", result["capabilities"])
	}
	if result["ttlMs"] != float64(mcpCacheTTLMs) || result["cacheScope"] != "public" {
		t.Errorf("ttlMs, cacheScope = %v, %v", result["ttlMs"], result["cacheScope"])
	}
}

func TestMcpHandler_ToolsList(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	_, resp := mcpRequest(t, srv, "tools/list", "")
	result := resp["result"].(map[string]any)
	mcpCheckComplete(t, result)
	if result["ttlMs"] != float64(mcpCacheTTLMs) || result["cacheScope"] != "public" {
		t.Errorf("ttlMs, cacheScope = %v, %v", result["ttlMs"], result["cacheScope"])
	}
	tools := result["tools"].([]any)
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
	if d := tools[1].(map[string]any)["description"]; d != "Always fails." {
		t.Errorf("description from MethodDescriber = %v", d)
	}
	schema, _ := json.Marshal(greet["inputSchema"])
	want := `{"additionalProperties":false,"properties":{"name":{"description":"who to greet","type":"string"},"times":{"type":"integer"}},"required":["name"],"type":"object"}`
	if string(schema) != want {
		t.Errorf("inputSchema = %s, want %s", schema, want)
	}
	if fail, _ := json.Marshal(tools[1].(map[string]any)["inputSchema"]); string(fail) != `{"type":"object"}` {
		t.Errorf("inputSchema without params = %s, want {\"type\":\"object\"}", fail)
	}

	outputs := map[string]string{
		"mcp-test-system_greet_v1": `{"properties":{"greeting":{"type":"string"}},"required":["greeting"],"type":"object"}`,
		"mcp-test-system_fail_v1":  `null`, // returns only an error
		"mcp-test-system_names_v1": `{"properties":{"names":{"items":{"type":"string"},"type":["array","null"]}},"required":["names"],"type":"object"}`,
	}
	for _, tool := range tools {
		tool := tool.(map[string]any)
		if got, _ := json.Marshal(tool["outputSchema"]); string(got) != outputs[tool["name"].(string)] {
			t.Errorf("%s outputSchema = %s, want %s", tool["name"], got, outputs[tool["name"].(string)])
		}
	}
}

func TestMcpHandler_ToolsCall(t *testing.T) {
	srv, sys := newMcpTestServer(t)

	t.Run("success has structured content and the result's text content", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_greet_v1", `{"name":"bob"}`)
		mcpCheckComplete(t, res)
		if res["isError"] != nil {
			t.Fatalf("isError = %v, want unset", res["isError"])
		}
		if got := res["structuredContent"].(map[string]any)["greeting"]; got != "hello bob " {
			t.Errorf("greeting = %q", got)
		}
		text := res["content"].([]any)[0].(map[string]any)["text"]
		if text != "Greeted: hello bob" {
			t.Errorf("text = %v, want the JonsonMcpTextContent summary", text)
		}
		if sys.lastSource != RpcSourceMcp {
			t.Errorf("RpcMeta.Source = %q, want %q", sys.lastSource, RpcSourceMcp)
		}
	})

	t.Run("validation error is a tool error", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_greet_v1", `{"name":""}`)
		mcpCheckComplete(t, res)
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
		_, resp := mcpRequest(t, srv, "tools/call", `"name":"mcp-test-system_fail_v1"`, "Mcp-Name", "mcp-test-system_fail_v1")
		if resp["result"].(map[string]any)["isError"] != true {
			t.Errorf("result = %v, want the call to run and fail", resp)
		}
	})

	t.Run("result without text content is sent as JSON text", func(t *testing.T) {
		res := mcpCall(t, srv, "mcp-test-system_names_v1", `{}`)
		if got := res["structuredContent"]; !reflect.DeepEqual(got, map[string]any{"names": []any{"a", "b"}}) {
			t.Errorf("structuredContent = %v, want names [a b]", got)
		}
		if text := res["content"].([]any)[0].(map[string]any)["text"]; text != `{"names":["a","b"]}` {
			t.Errorf("text = %v", text)
		}
	})

	t.Run("base64 Mcp-Name is decoded", func(t *testing.T) {
		code, _ := mcpRequest(t, srv, "tools/call", `"name":"mcp-test-system_names_v1"`, "Mcp-Name", "=?base64?bWNwLXRlc3Qtc3lzdGVtX25hbWVzX3Yx?=")
		if code != http.StatusOK {
			t.Errorf("status = %d, want 200", code)
		}
	})

	t.Run("unknown tool is a protocol error", func(t *testing.T) {
		_, resp := mcpRequest(t, srv, "tools/call", `"name":"mcp-test-system_hidden_v1","arguments":{}`, "Mcp-Name", "mcp-test-system_hidden_v1")
		if code := resp["error"].(map[string]any)["code"]; code != float64(ErrInvalidParams.Code) {
			t.Errorf("error code = %v, want %d", code, ErrInvalidParams.Code)
		}
	})
}

func TestMcpHandler_RequestValidation(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	tests := []struct {
		name      string
		method    string
		params    string
		header    []string
		wantCode  int
		wantError int
	}{
		{name: "protocol version header missing", method: "tools/list", header: []string{"MCP-Protocol-Version", ""}, wantCode: http.StatusBadRequest, wantError: -32020},
		{name: "protocol version header differs from body", method: "tools/list", header: []string{"MCP-Protocol-Version", "2025-11-25"}, wantCode: http.StatusBadRequest, wantError: -32020},
		{name: "Mcp-Method differs from body", method: "tools/list", header: []string{"Mcp-Method", "tools/call"}, wantCode: http.StatusBadRequest, wantError: -32020},
		{name: "Mcp-Name missing", method: "tools/call", params: `"name":"mcp-test-system_names_v1"`, wantCode: http.StatusBadRequest, wantError: -32020},
		{name: "Mcp-Name differs from body", method: "tools/call", params: `"name":"mcp-test-system_names_v1"`, header: []string{"Mcp-Name", "mcp-test-system_fail_v1"}, wantCode: http.StatusBadRequest, wantError: -32020},
		{name: "broken base64 Mcp-Name", method: "tools/call", params: `"name":"x"`, header: []string{"Mcp-Name", "=?base64?!!?="}, wantCode: http.StatusBadRequest, wantError: -32020},
		{name: "ping is gone", method: "ping", wantCode: http.StatusNotFound, wantError: ErrMethodNotFound.Code},
		{name: "unknown method", method: "resources/list", wantCode: http.StatusNotFound, wantError: ErrMethodNotFound.Code},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, resp := mcpRequest(t, srv, tc.method, tc.params, tc.header...)
			if code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %v", code, tc.wantCode, resp)
			}
			if got := resp["error"].(map[string]any)["code"]; got != float64(tc.wantError) {
				t.Errorf("error code = %v, want %d", got, tc.wantError)
			}
		})
	}

	for name, body := range map[string]string{
		"missing _meta":            `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		"null client capabilities": `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":null}}}`,
	} {
		t.Run(name+" is invalid params", func(t *testing.T) {
			code, resp := mcpPost(t, srv, body, "MCP-Protocol-Version", McpProtocolVersion, "Mcp-Method", "tools/list")
			if code != http.StatusBadRequest || resp["error"].(map[string]any)["code"] != float64(ErrInvalidParams.Code) {
				t.Errorf("response = %d %v, want 400 invalid params", code, resp)
			}
		})
	}

	unsupported := map[string]struct{ body, requested string }{
		"unsupported version": {
			body:      `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`,
			requested: "1900-01-01",
		},
		"legacy initialize": {
			body:      `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
			requested: "2025-11-25",
		},
		"legacy initialize with undecodable params": {
			body:      `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":20251125}}`,
			requested: "",
		},
	}
	for name, tc := range unsupported {
		t.Run(name+" lists the supported version", func(t *testing.T) {
			code, resp := mcpPost(t, srv, tc.body, "MCP-Protocol-Version", tc.requested)
			e := resp["error"].(map[string]any)
			if code != http.StatusBadRequest || e["code"] != float64(-32022) {
				t.Fatalf("response = %d %v, want 400 -32022", code, resp)
			}
			want := map[string]any{"supported": []any{McpProtocolVersion}, "requested": tc.requested}
			if !reflect.DeepEqual(e["data"], want) {
				t.Errorf("data = %v, want %v", e["data"], want)
			}
		})
	}
}

func TestMcpHandler_Transport(t *testing.T) {
	srv, _ := newMcpTestServer(t)
	list := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{` + mcpMeta + `}}`
	listHeader := []string{"MCP-Protocol-Version", McpProtocolVersion, "Mcp-Method", "tools/list"}
	tests := []struct {
		name      string
		method    string
		body      string
		header    []string
		wantCode  int
		wantError int
	}{
		{name: "notification is accepted", method: http.MethodPost, body: `{"jsonrpc":"2.0","method":"notifications/cancelled"}`, wantCode: http.StatusAccepted},
		{name: "response is rejected", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":1,"result":{}}`, wantCode: http.StatusBadRequest, wantError: -32600},
		{name: "batch is rejected", method: http.MethodPost, body: `[` + list + `]`, wantCode: http.StatusBadRequest, wantError: -32600},
		{name: "broken json", method: http.MethodPost, body: `{"jsonrpc":`, wantCode: http.StatusBadRequest, wantError: ErrParse.Code},
		{name: "GET has no event stream", method: http.MethodGet, wantCode: http.StatusMethodNotAllowed},
		{name: "foreign origin is forbidden", method: http.MethodPost, body: list, header: append([]string{"Origin", "https://evil.example"}, listHeader...), wantCode: http.StatusForbidden},
		{name: "same origin is served", method: http.MethodPost, body: list, header: append([]string{"Origin", "http://example.com"}, listHeader...), wantCode: http.StatusOK},
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
		{"no description and no doc comment", func(h *McpHandler) { h.Expose(sys.GreetV1) }},
		{"two descriptions", func(h *McpHandler) { h.Expose(sys.GreetV1, "a", "b") }},
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
	Opt      *int               `json:"opt"`  // nil-able: optional without omitempty
	Meta     map[string]string  `json:"meta"` // nil-able: optional without omitempty
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

type mcpDescribedNested struct {
	A string `json:"a"`
}

func (*mcpDescribedNested) JonsonFieldDescriptions() map[string]string {
	return map[string]string{"A": "nested comment"}
}

type mcpDescribedParams struct {
	Params
	Name   string             `json:"name"`
	Tagged string             `json:"tagged" description:"from tag"`
	Nested mcpDescribedNested `json:"nested"`
}

func (*mcpDescribedParams) JonsonFieldDescriptions() map[string]string {
	return map[string]string{"Name": "from comment", "Tagged": "ignored, the tag wins"}
}

func TestMcpInputSchemaFieldDescriptions(t *testing.T) {
	got, _ := json.Marshal(mcpInputSchema(reflect.TypeFor[mcpDescribedParams]()))
	want := `{"additionalProperties":false,"properties":{` +
		`"name":{"description":"from comment","type":"string"},` +
		`"nested":{"additionalProperties":false,"properties":{"a":{"description":"nested comment","type":"string"}},"required":["a"],"type":"object"},` +
		`"tagged":{"description":"from tag","type":"string"}},` +
		`"required":["name","tagged","nested"],"type":"object"}`
	if string(got) != want {
		t.Errorf("schema =\n%s\nwant\n%s", got, want)
	}
}

func TestMcpInputSchema(t *testing.T) {
	got, _ := json.Marshal(mcpInputSchema(reflect.TypeFor[mcpSchemaParams]()))
	want := `{"additionalProperties":false,"properties":{` +
		`"Untagged":{"type":"boolean"},` +
		`"any":{},` +
		`"blob":{"type":"string"},` +
		`"count":{"type":"string"},` +
		`"inline":{"type":"string"},` +
		`"labels":{"additionalProperties":{"type":"integer"},"type":"object"},` +
		`"meta":{"additionalProperties":{"type":"string"},"type":"object"},` +
		`"nested":{"additionalProperties":false,"properties":{"A":{"type":"string"}},"required":["A"],"type":"object"},` +
		`"opt":{"type":"integer"},` +
		`"ptr":{"type":"number"},` +
		`"raw":{},` +
		`"tags":{"items":{"type":"string"},"type":"array"},` +
		`"tree":{"additionalProperties":false,"properties":{"children":{"items":{"type":"object"},"type":"array"}},"type":"object"},` +
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

// mcpTextID encodes as text, like uuid.UUID
type mcpTextID [2]byte

func (mcpTextID) MarshalText() ([]byte, error) { return []byte("id"), nil }

// mcpJsonValue has its own JSON encoding
type mcpJsonValue struct{ A int }

func (*mcpJsonValue) UnmarshalJSON([]byte) error { return nil }

type mcpOutputResult struct {
	mcpSchemaEmbedded
	Name    string            `json:"name"`
	Tags    []string          `json:"tags"`
	Labels  map[string]string `json:"labels"`
	Ptr     *int              `json:"ptr"`
	Count   *int64            `json:"count,string"`
	ID      mcpTextID         `json:"id"`
	Custom  mcpJsonValue      `json:"custom"`
	Note    string            `json:"note,omitempty"`
	Child   *mcpOutputResult  `json:"child,omitempty"`
	Skipped string            `json:"-"`
}

func TestMcpOutputSchema(t *testing.T) {
	if s := mcpOutputSchema(nil); s != nil {
		t.Errorf("schema without result = %v, want nil", s)
	}
	got, _ := json.Marshal(mcpOutputSchema(reflect.TypeFor[*mcpOutputResult]()))
	// fields without omitempty are always written, nil-able ones as null; no additionalProperties
	want := `{"properties":{` +
		`"child":{"type":"object"},` +
		`"count":{"type":["string","null"]},` +
		`"custom":{},` +
		`"id":{"type":"string"},` +
		`"inline":{"type":"string"},` +
		`"labels":{"additionalProperties":{"type":"string"},"type":["object","null"]},` +
		`"name":{"type":"string"},` +
		`"note":{"type":"string"},` +
		`"ptr":{"type":["integer","null"]},` +
		`"tags":{"items":{"type":"string"},"type":["array","null"]}},` +
		`"required":["inline","name","tags","labels","ptr","count","id","custom"],"type":"object"}`
	if string(got) != want {
		t.Errorf("schema =\n%s\nwant\n%s", got, want)
	}
}

type McpDocSystem struct{}

func (*McpDocSystem) JonsonMethodDescriptions() map[string]string {
	return map[string]string{
		"mcp-doc-system/a.v1": "Does a.",
		"mcp-doc-system/b.v1": "Does b.",
	}
}

func (s *McpDocSystem) AV1(ctx *Context) error            { return nil }
func (s *McpDocSystem) BV1(ctx *Context) error            { return nil }
func (s *McpDocSystem) CV1(ctx *Context, _ HttpGet) error { return nil }

func mcpToolDescriptions(h *McpHandler) map[string]string {
	out := map[string]string{}
	for _, tool := range h.tools {
		out[tool.Name] = tool.Description
	}
	return out
}

func TestMcpHandler_ExposeSystemAndAll(t *testing.T) {
	doc := &McpDocSystem{}
	mh := NewMethodHandler(NewFactory(), NewDebugSecret(), nil)
	mh.RegisterSystem(doc)

	t.Run("system skips GET and keeps already exposed tools", func(t *testing.T) {
		h := NewMcpHandler(mh, "/mcp").Expose(doc.AV1, "custom").ExposeSystem(doc)
		want := map[string]string{"mcp-doc-system_a_v1": "custom", "mcp-doc-system_b_v1": "Does b."}
		if got := mcpToolDescriptions(h); !reflect.DeepEqual(got, want) {
			t.Errorf("tools = %v, want %v", got, want)
		}
	})

	t.Run("all exposes every documented endpoint in name order", func(t *testing.T) {
		h := NewMcpHandler(mh, "/mcp").ExposeAll()
		if len(h.tools) != 2 || h.tools[0].Name != "mcp-doc-system_a_v1" || h.tools[1].Name != "mcp-doc-system_b_v1" {
			t.Errorf("tools = %v", mcpToolDescriptions(h))
		}
	})

	for name, expose := range map[string]func(){
		"unregistered system": func() { NewMcpHandler(mh, "/mcp").ExposeSystem(&McpTestSystem{}) },
		"undocumented method": func() {
			mh := NewMethodHandler(NewFactory(), NewDebugSecret(), nil)
			mh.RegisterSystem(&McpTestSystem{})
			NewMcpHandler(mh, "/mcp").ExposeAll()
		},
	} {
		t.Run(name+" panics", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("did not panic")
				}
			}()
			expose()
		})
	}
}

func TestMcpHandler_MissingTextSummarizerLevel(t *testing.T) {
	sys := &McpTestSystem{}
	mh := NewMethodHandler(NewFactory(), NewDebugSecret(), nil)
	mh.RegisterSystem(sys)
	fatal := &McpHandlerOpts{MissingTextSummarizerLevel: MissingValidationLevelFatal}

	mustPanic := func(t *testing.T, expose func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "mcp-test-system/names.v1's result") {
				t.Errorf("recovered %v, want the missing summarizer report for names.v1", r)
			}
		}()
		expose()
	}

	t.Run("ignored by default", func(t *testing.T) {
		NewMcpHandler(mh, "/mcp").ExposeMethod("mcp-test-system/names.v1", "Lists names.")
	})
	t.Run("summarizer and error-only methods pass", func(t *testing.T) {
		NewMcpHandler(mh, "/mcp").WithOpts(fatal).Expose(sys.GreetV1, "Greets.").Expose(sys.FailV1)
	})
	t.Run("missing summarizer is reported on expose", func(t *testing.T) {
		mustPanic(t, func() {
			NewMcpHandler(mh, "/mcp").WithOpts(fatal).ExposeMethod("mcp-test-system/names.v1", "Lists names.")
		})
	})
	t.Run("tools exposed before WithOpts are checked", func(t *testing.T) {
		mustPanic(t, func() {
			NewMcpHandler(mh, "/mcp").ExposeMethod("mcp-test-system/names.v1", "Lists names.").WithOpts(fatal)
		})
	})
	t.Run("invalid level is ignore", func(t *testing.T) {
		NewMcpHandler(mh, "/mcp").WithOpts(&McpHandlerOpts{MissingTextSummarizerLevel: "nope"}).ExposeMethod("mcp-test-system/names.v1", "Lists names.")
	})
}
