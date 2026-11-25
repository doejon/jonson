package jonson

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"slices"
	"strings"
)

// McpProtocolVersions lists the MCP protocol versions the McpHandler speaks, newest first.
var McpProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

var errMcpInvalidRequest = &Error{Code: -32600, Message: "Invalid Request"}

// McpHandler serves registered methods as tools of the Model Context Protocol (MCP).
// MCP is JSON-RPC 2.0, so a tool call runs the method through the same pipeline as
// every other handler: providers, validation, logging and error encoding.
//
// The transport is stateless streamable HTTP: each POST carries one JSON-RPC
// message and is answered with application/json. There are no sessions and no
// server-sent events, so any replica can answer any call.
// Only methods passed to Expose become tools.
type McpHandler struct {
	path          string
	methodHandler *MethodHandler
	opts          *McpHandlerOpts
	tools         []*mcpTool
	toolsByName   map[string]*mcpTool
}

// McpHandlerOpts configures the McpHandler.
type McpHandlerOpts struct {
	// Limits max bytes to be read.
	// In case MaxBytes is set to bytes <= 0, MaxBytes will be ignored
	MaxBytes int64
	// Name and Version are returned as serverInfo on initialize.
	Name    string
	Version string
	// Instructions are returned on initialize and tell the model how to use the tools.
	Instructions string
	// AllowOrigin decides whether a request with an Origin header is served.
	// MCP requires servers to validate Origin against DNS rebinding.
	// By default, only an Origin whose host equals the request's Host is allowed.
	// Requests without an Origin header (non-browser clients) are always served.
	AllowOrigin func(origin string, req *http.Request) bool
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	method      string
}

// NewMcpHandler returns a handler that serves MCP on path.
func NewMcpHandler(methodHandler *MethodHandler, path string) *McpHandler {
	return &McpHandler{
		path:          path,
		methodHandler: methodHandler,
		opts: &McpHandlerOpts{
			MaxBytes: 5e6,
			Name:     "jonson",
		},
		toolsByName: map[string]*mcpTool{},
	}
}

// WithOpts sets the mcp handler opts
func (h *McpHandler) WithOpts(opts *McpHandlerOpts) *McpHandler {
	h.opts = opts
	return h
}

// McpToolName returns the tool name of a method: system/method.v1 becomes system_method_v1.
// MCP clients only accept [a-zA-Z0-9_-] in tool names.
func McpToolName(method string) string {
	return strings.NewReplacer("/", "_", ".", "_").Replace(method)
}

// Expose makes a method of a registered system available as a tool. Pass the method
// value, such as account.GetProfileV1 for a registered *Account; only its type and name
// are used, the call goes to the registered system.
// The description tells the model what the tool does and when to use it. Without one,
// the method's doc comment is used (see MethodDescriber).
// Expose panics on anything but a method value of a registered system, a missing
// description or a method requiring GET, like RegisterMethod panics on an invalid definition.
func (h *McpHandler) Expose(fn any, description ...string) *McpHandler {
	method, ok := methodValueName(fn)
	if !ok {
		panic("mcp handler: Expose needs a method value such as system.MethodV1")
	}
	return h.ExposeMethod(method, description...)
}

// ExposeSystem exposes all methods of a registered system, such as account for a
// registered *Account, as tools. Descriptions come from the generated doc comments
// (see MethodDescriber). Methods requiring GET and methods that are already exposed
// are skipped. ExposeSystem panics on an unregistered system or a method without description.
func (h *McpHandler) ExposeSystem(sys any) *McpHandler {
	rt := reflect.TypeOf(sys)
	if _, ok := h.methodHandler.systems[rt]; !ok {
		panic(fmt.Sprintf("mcp handler: system %v is not registered", rt))
	}
	return h.exposeWhere(func(e *apiEndpoint) bool {
		return e.methodContext.IsValid() && e.methodContext.Type() == rt
	})
}

// ExposeAll exposes all endpoints registered so far as tools, like ExposeSystem does
// for a single system. Endpoints registered afterwards are not exposed.
func (h *McpHandler) ExposeAll() *McpHandler {
	return h.exposeWhere(func(*apiEndpoint) bool { return true })
}

// exposeWhere exposes the matching endpoints in name order, skipping GET methods
// and the ones already exposed.
func (h *McpHandler) exposeWhere(match func(e *apiEndpoint) bool) *McpHandler {
	for _, method := range slices.Sorted(maps.Keys(h.methodHandler.endpoints)) {
		e := h.methodHandler.endpoints[method]
		if !match(e) || e.httpMethod == RpcHttpMethodGet {
			continue
		}
		if _, exposed := h.toolsByName[McpToolName(method)]; exposed {
			continue
		}
		h.ExposeMethod(method)
	}
	return h
}

// MethodDescriber is implemented by systems whose methods are documented, e.g. by
// jonson-generate, which turns method doc comments into JonsonMethodDescriptions.
// Descriptions are keyed by endpoint name, such as "account/get-profile.v1".
type MethodDescriber interface {
	JonsonMethodDescriptions() map[string]string
}

// methodValueName returns the endpoint name of a method value, e.g. "account/get-profile.v1"
// for account.GetProfileV1, derived from the symbol the compiler gives it: "pkg.(*Account).GetProfileV1-fm".
func methodValueName(fn any) (string, bool) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func || v.IsNil() {
		return "", false
	}
	f := runtime.FuncForPC(v.Pointer())
	if f == nil {
		return "", false
	}
	// only method values carry the -fm suffix; plain funcs and closures don't have a receiver
	name, ok := strings.CutSuffix(f.Name(), "-fm")
	if !ok {
		return "", false
	}
	i := strings.LastIndex(name, ".")
	recv, goMethod := name[:i], name[i+1:]
	recv = strings.Trim(recv[strings.LastIndex(recv, ".")+1:], "(*)")
	methodName, version := SplitGoMethodName(goMethod)
	if version == 0 {
		return "", false
	}
	return GetDefaultMethodName(ToKebabCase(recv), methodName, version), true
}

// ExposeMethod is Expose for a method given by name, such as "account/get-profile.v1";
// use it for methods registered with RegisterMethod under a custom name.
func (h *McpHandler) ExposeMethod(method string, description ...string) *McpHandler {
	endpoint, ok := h.methodHandler.endpoints[method]
	if !ok {
		panic("mcp handler: method " + method + " is not registered")
	}
	if len(description) > 1 {
		panic("mcp handler: method " + method + " has more than one description")
	}
	desc := strings.Join(description, "")
	if desc == "" && endpoint.methodContext.IsValid() {
		if d, ok := endpoint.methodContext.Interface().(MethodDescriber); ok {
			desc = d.JonsonMethodDescriptions()[method]
		}
	}
	if strings.TrimSpace(desc) == "" {
		panic("mcp handler: method " + method + " needs a description")
	}
	if endpoint.httpMethod == RpcHttpMethodGet {
		panic("mcp handler: method " + method + " requires GET; mcp only calls via POST")
	}
	name := McpToolName(method)
	if _, exists := h.toolsByName[name]; exists {
		panic("mcp handler: method " + method + " is already exposed")
	}
	tool := &mcpTool{
		Name:        name,
		Description: desc,
		InputSchema: mcpInputSchema(endpoint.paramsType),
		method:      method,
	}
	h.tools = append(h.tools, tool)
	h.toolsByName[name] = tool
	return h
}

// Handle will handle an incoming http request
func (h *McpHandler) Handle(w http.ResponseWriter, req *http.Request) bool {
	if req.URL.Path != h.path {
		return false
	}
	if origin := req.Header.Get("Origin"); origin != "" && !h.allowOrigin(origin, req) {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	// stateless: no server-sent event stream (GET) and no sessions to delete (DELETE)
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}

	applyMaxBytesReader(w, req, h.opts.MaxBytes)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.methodHandler.getLogger(nil).Warn("mcp handler: read error", "error", err)
		h.write(w, NewRpcErrorResponse(nil, ErrParse.Coalesce(err)))
		return true
	}

	// MCP sends exactly one JSON-RPC message per POST; batches are not part of the protocol.
	if trimmed := strings.TrimSpace(string(body)); !strings.HasPrefix(trimmed, "{") {
		h.write(w, NewRpcErrorResponse(nil, errMcpInvalidRequest))
		return true
	}

	var msg struct {
		RpcRequest
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := h.methodHandler.opts.JsonHandler.Unmarshal(body, &msg); err != nil {
		h.write(w, NewRpcErrorResponse(nil, ErrParse))
		return true
	}

	// notifications (no id) and responses to server requests need no answer
	if msg.ID == nil || msg.Method == "" {
		w.WriteHeader(http.StatusAccepted)
		return true
	}

	h.write(w, h.dispatch(w, req, &msg.RpcRequest))
	return true
}

func (h *McpHandler) dispatch(w http.ResponseWriter, req *http.Request, msg *RpcRequest) any {
	switch msg.Method {
	case "initialize":
		return h.initialize(msg)
	case "ping":
		return NewRpcResultResponse(msg.ID, struct{}{})
	case "tools/list":
		return NewRpcResultResponse(msg.ID, map[string]any{"tools": h.toolList()})
	case "tools/call":
		return h.callTool(w, req, msg)
	default:
		return NewRpcErrorResponse(msg.ID, ErrMethodNotFound)
	}
}

func (h *McpHandler) initialize(msg *RpcRequest) any {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(msg.Params) > 0 {
		if err := h.methodHandler.opts.JsonHandler.Unmarshal(msg.Params, &params); err != nil {
			return NewRpcErrorResponse(msg.ID, ErrInvalidParams)
		}
	}
	version := McpProtocolVersions[0]
	if slices.Contains(McpProtocolVersions, params.ProtocolVersion) {
		version = params.ProtocolVersion
	}
	result := map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": h.opts.Name, "version": h.opts.Version},
	}
	if h.opts.Instructions != "" {
		result["instructions"] = h.opts.Instructions
	}
	return NewRpcResultResponse(msg.ID, result)
}

func (h *McpHandler) toolList() []*mcpTool {
	if h.tools == nil {
		return []*mcpTool{}
	}
	return h.tools
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallToolResult struct {
	Content           []mcpContent    `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

func (h *McpHandler) callTool(w http.ResponseWriter, req *http.Request, msg *RpcRequest) any {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := h.methodHandler.opts.JsonHandler.Unmarshal(msg.Params, &params); err != nil {
		return NewRpcErrorResponse(msg.ID, ErrInvalidParams)
	}
	tool, ok := h.toolsByName[params.Name]
	if !ok {
		// an unknown tool is a protocol error, not a tool error
		return NewRpcErrorResponse(msg.ID, ErrInvalidParams)
	}
	if len(params.Arguments) == 0 || string(params.Arguments) == "null" {
		params.Arguments = json.RawMessage("{}")
	}

	resp := h.methodHandler.processRpcMessage(RpcSourceMcp, RpcHttpMethodPost, req, w, nil, &RpcRequest{
		Version: "2.0",
		ID:      msg.ID,
		Method:  tool.method,
		Params:  params.Arguments,
	})

	// a failed call is a tool result with isError, so that the model sees the error and can retry
	if e, ok := resp.(*RpcErrorResponse); ok {
		text, _ := h.methodHandler.opts.JsonHandler.Marshal(e.Error)
		return NewRpcResultResponse(msg.ID, &mcpCallToolResult{
			Content: []mcpContent{{Type: "text", Text: string(text)}},
			IsError: true,
		})
	}

	var result any
	if r, ok := resp.(*RpcResultResponse); ok {
		result = r.Result
	}
	b, err := h.methodHandler.opts.JsonHandler.Marshal(result)
	if err != nil {
		return NewRpcErrorResponse(msg.ID, ErrInternal.CloneWithData(&ErrorData{
			Debug: h.methodHandler.errorEncoder.Encode(err.Error()),
		}))
	}
	out := &mcpCallToolResult{Content: []mcpContent{{Type: "text", Text: string(b)}}}
	// structuredContent must be a JSON object
	if len(b) > 0 && b[0] == '{' {
		out.StructuredContent = b
	}
	return NewRpcResultResponse(msg.ID, out)
}

func (h *McpHandler) write(w http.ResponseWriter, resp any) {
	b, _ := h.methodHandler.opts.JsonHandler.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (h *McpHandler) allowOrigin(origin string, req *http.Request) bool {
	if h.opts.AllowOrigin != nil {
		return h.opts.AllowOrigin(origin, req)
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && u.Host == req.Host
}
