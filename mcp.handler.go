package jonson

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"reflect"
	"runtime"
	"slices"
	"strings"
)

// McpProtocolVersion is the only MCP protocol version the McpHandler speaks.
// Older, initialize-based versions are rejected with an UnsupportedProtocolVersion error.
const McpProtocolVersion = "2026-07-28"

var errMcpInvalidRequest = &Error{Code: -32600, Message: "Invalid Request"}

// McpHandler serves registered methods as tools of the Model Context Protocol (MCP).
// MCP is JSON-RPC 2.0, so a tool call runs the method through the same pipeline as
// every other handler: providers, validation, logging and error encoding.
//
// The transport is streamable HTTP: each POST carries one JSON-RPC message and is
// answered with application/json. MCP is stateless: every request carries its
// protocol version in _meta, so any replica can answer any call.
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
	// Name and Version are returned as serverInfo in every result.
	Name    string
	Version string
	// Instructions are returned on server/discover and tell the model how to use the tools.
	Instructions string
	// AllowOrigin decides whether a request with an Origin header is served.
	// MCP requires servers to validate Origin against DNS rebinding.
	// Browsers send an Origin header, so by default (nil) every browser request is
	// forbidden; set AllowOrigin to serve browser-based clients from known origins.
	// Comparing Origin to the Host header does not help: after DNS rebinding,
	// both carry the attacker's domain.
	// Requests without an Origin header (non-browser clients) are always served.
	AllowOrigin func(origin string, req *http.Request) bool
	// MissingTextSummarizerLevel reports exposed tools whose result does not implement
	// McpTextSummarizer, so that the result is sent twice (as text and as structuredContent).
	// Checked when a method is exposed, like MethodHandlerOptions.MissingValidationLevel.
	// Defaults to ignore.
	MissingTextSummarizerLevel MissingValidationLevel
	// Authorizer limits tools/list to the tools the caller may call. Only tools whose
	// method requires *Private are checked, in a single call per tools/list; the others
	// are always listed. With an Authorizer, tools/list is cached per caller (cacheScope private).
	// tools/call needs no Authorizer: *Private protects the method itself.
	// nil lists all exposed tools.
	Authorizer AuthBulkAuthorizer
}

type mcpTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	method       string
	// result is the method's result type; nil for methods returning only an error
	result reflect.Type
}

// NewMcpHandler returns a handler that serves MCP on path.
func NewMcpHandler(methodHandler *MethodHandler, path string) *McpHandler {
	return &McpHandler{
		path:          path,
		methodHandler: methodHandler,
		opts: &McpHandlerOpts{
			MaxBytes:                   5e6,
			Name:                       "jonson",
			MissingTextSummarizerLevel: MissingValidationLevelIgnore,
		},
		toolsByName: map[string]*mcpTool{},
	}
}

// WithOpts sets the mcp handler opts. Tools exposed before are checked
// against the new MissingTextSummarizerLevel.
func (h *McpHandler) WithOpts(opts *McpHandlerOpts) *McpHandler {
	if _, ok := validMissingValidationLevel[opts.MissingTextSummarizerLevel]; !ok {
		opts.MissingTextSummarizerLevel = MissingValidationLevelIgnore
	}
	h.opts = opts
	for _, tool := range h.tools {
		h.checkTextSummarizer(tool)
	}
	return h
}

// checkTextSummarizer reports a tool whose result does not summarize itself.
func (h *McpHandler) checkTextSummarizer(tool *mcpTool) {
	if tool.result == nil || tool.result.Implements(typeMcpTextSummarizer) {
		return
	}
	h.methodHandler.report(h.opts.MissingTextSummarizerLevel, "mcp handler: "+tool.method+"'s result '"+tool.result.String()+
		"' does not implement 'JonsonMcpTextContent() string' method; the tool result is sent twice\n")
}

var typeMcpTextSummarizer = reflect.TypeFor[McpTextSummarizer]()

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
	// methods return (result, error) or just error
	var resultType reflect.Type
	if ft := endpoint.handlerFunc.Type(); ft.NumOut() == 2 {
		resultType = ft.Out(0)
	}
	tool := &mcpTool{
		Name:         name,
		Description:  desc,
		InputSchema:  mcpInputSchema(endpoint.paramsType),
		OutputSchema: mcpOutputSchema(resultType),
		method:       method,
		result:       resultType,
	}
	h.checkTextSummarizer(tool)
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
		h.write(w, http.StatusBadRequest, NewRpcErrorResponse(nil, ErrParse.Coalesce(err)))
		return true
	}

	// MCP sends exactly one JSON-RPC message per POST; batches are not part of the protocol.
	if trimmed := strings.TrimSpace(string(body)); !strings.HasPrefix(trimmed, "{") {
		h.write(w, http.StatusBadRequest, NewRpcErrorResponse(nil, errMcpInvalidRequest))
		return true
	}

	var msg RpcRequest
	if err := h.methodHandler.opts.JsonHandler.Unmarshal(body, &msg); err != nil {
		h.write(w, http.StatusBadRequest, NewRpcErrorResponse(nil, ErrParse))
		return true
	}
	// clients must not send responses: there are no server requests to answer
	if msg.Method == "" {
		h.write(w, http.StatusBadRequest, NewRpcErrorResponse(msg.ID, errMcpInvalidRequest))
		return true
	}
	// notifications need no answer
	if msg.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return true
	}

	status, resp := h.dispatch(w, req, &msg)
	h.write(w, status, resp)
	return true
}

type mcpRequestParams struct {
	Name string `json:"name"`
	// ProtocolVersion is only sent by legacy clients on initialize.
	ProtocolVersion string `json:"protocolVersion"`
	Meta            struct {
		ProtocolVersion    string          `json:"io.modelcontextprotocol/protocolVersion"`
		ClientCapabilities json.RawMessage `json:"io.modelcontextprotocol/clientCapabilities"`
	} `json:"_meta"`
}

func (h *McpHandler) dispatch(w http.ResponseWriter, req *http.Request, msg *RpcRequest) (int, any) {
	var params mcpRequestParams
	var err error
	if len(msg.Params) > 0 {
		err = h.methodHandler.opts.JsonHandler.Unmarshal(msg.Params, &params)
	}
	// legacy clients open with initialize and have no way to fall forward;
	// tell them which version this server speaks, even if their params don't decode
	if msg.Method == "initialize" {
		return http.StatusBadRequest, mcpUnsupportedVersion(msg.ID, params.ProtocolVersion)
	}
	if err != nil {
		return http.StatusBadRequest, NewRpcErrorResponse(msg.ID, ErrInvalidParams)
	}
	// a JSON null decodes into the raw message "null"
	if caps := params.Meta.ClientCapabilities; params.Meta.ProtocolVersion == "" || len(caps) == 0 || string(caps) == "null" {
		return http.StatusBadRequest, NewRpcErrorResponse(msg.ID, ErrInvalidParams)
	}
	if req.Header.Get("MCP-Protocol-Version") != params.Meta.ProtocolVersion {
		return http.StatusBadRequest, mcpHeaderMismatch(msg.ID, "MCP-Protocol-Version")
	}
	if params.Meta.ProtocolVersion != McpProtocolVersion {
		return http.StatusBadRequest, mcpUnsupportedVersion(msg.ID, params.Meta.ProtocolVersion)
	}
	if req.Header.Get("Mcp-Method") != msg.Method {
		return http.StatusBadRequest, mcpHeaderMismatch(msg.ID, "Mcp-Method")
	}

	switch msg.Method {
	case "server/discover":
		result := map[string]any{
			"supportedVersions": []string{McpProtocolVersion},
			"capabilities":      map[string]any{"tools": map[string]any{}},
			"ttlMs":             mcpCacheTTLMs,
			"cacheScope":        "public",
		}
		if h.opts.Instructions != "" {
			result["instructions"] = h.opts.Instructions
		}
		return http.StatusOK, h.complete(msg.ID, result)
	case "tools/list":
		tools, err := h.toolList(w, req)
		if err != nil {
			return http.StatusOK, NewRpcErrorResponse(msg.ID, err)
		}
		scope := "public"
		if h.opts.Authorizer != nil {
			scope = "private"
		}
		return http.StatusOK, h.complete(msg.ID, map[string]any{
			"tools":      tools,
			"ttlMs":      mcpCacheTTLMs,
			"cacheScope": scope,
		})
	case "tools/call":
		if name, ok := mcpDecodeHeader(req.Header.Get("Mcp-Name")); !ok || name != params.Name {
			return http.StatusBadRequest, mcpHeaderMismatch(msg.ID, "Mcp-Name")
		}
		return http.StatusOK, h.callTool(w, req, msg)
	default:
		return http.StatusNotFound, NewRpcErrorResponse(msg.ID, ErrMethodNotFound)
	}
}

// mcpCacheTTLMs tells clients how long they may cache tools/list and server/discover.
// ponytail: fixed 5 minutes, the tool set only changes on deploy; make it an option if that is too long.
const mcpCacheTTLMs = 300_000

// complete marks result as final and identifies the server, as every MCP result must.
func (h *McpHandler) complete(id json.RawMessage, result map[string]any) any {
	result["resultType"] = "complete"
	result["_meta"] = map[string]any{
		"io.modelcontextprotocol/serverInfo": map[string]any{"name": h.opts.Name, "version": h.opts.Version},
	}
	return NewRpcResultResponse(id, result)
}

// mcpErrorResponse is an RpcErrorResponse whose data is not jonson's ErrorData.
type mcpErrorResponse struct {
	RpcResponseHeader
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	} `json:"error"`
}

func mcpUnsupportedVersion(id json.RawMessage, requested string) any {
	resp := &mcpErrorResponse{RpcResponseHeader: NewRpcResponseHeader(id)}
	resp.Error.Code = -32022
	resp.Error.Message = "Unsupported protocol version: this server speaks MCP " + McpProtocolVersion
	resp.Error.Data = map[string]any{"supported": []string{McpProtocolVersion}, "requested": requested}
	return resp
}

func mcpHeaderMismatch(id json.RawMessage, header string) any {
	return NewRpcErrorResponse(id, &Error{Code: -32020, Message: "Header mismatch: " + header + " header is missing or does not match the request body"})
}

// mcpDecodeHeader decodes a header value that clients may send as =?base64?...?=.
func mcpDecodeHeader(v string) (string, bool) {
	enc, ok := strings.CutPrefix(v, "=?base64?")
	if !ok {
		return v, true
	}
	enc, ok = strings.CutSuffix(enc, "?=")
	if !ok {
		return "", false
	}
	b, err := base64.StdEncoding.DecodeString(enc)
	return string(b), err == nil
}

// toolList returns the exposed tools, without the private ones the Authorizer denies.
func (h *McpHandler) toolList(w http.ResponseWriter, req *http.Request) ([]*mcpTool, *Error) {
	var private []*RpcMeta
	for _, tool := range h.tools {
		if h.methodHandler.endpoints[tool.method].private {
			private = append(private, &RpcMeta{Method: tool.method, HttpMethod: RpcHttpMethodPost, Source: RpcSourceMcp})
		}
	}
	if h.opts.Authorizer == nil || len(private) == 0 {
		return append([]*mcpTool{}, h.tools...), nil
	}

	ctx := NewContext(req.Context(), h.methodHandler.factory, h.methodHandler)
	ctx.StoreValue(TypeHttpRequest, &HttpRequest{Request: req})
	ctx.StoreValue(TypeHttpResponseWriter, &HttpResponseWriter{ResponseWriter: w})
	allowed, err := h.opts.Authorizer.AuthorizedMethods(ctx, private)
	if err = ctx.Finalize(err); err != nil {
		h.methodHandler.getLogger(ctx).Warn("mcp handler: authorizer failed", "error", err)
		if e, ok := err.(*Error); ok {
			return nil, e
		}
		return nil, ErrInternal.CloneWithData(&ErrorData{Debug: h.methodHandler.errorEncoder.Encode(err.Error())})
	}

	tools := []*mcpTool{}
	for _, tool := range h.tools {
		if !h.methodHandler.endpoints[tool.method].private || allowed[tool.method] {
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
		return h.complete(msg.ID, map[string]any{
			"content": []mcpContent{{Type: "text", Text: string(text)}},
			"isError": true,
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
	text := string(b)
	if t, ok := result.(McpTextSummarizer); ok {
		text = t.JonsonMcpTextContent()
	}
	out := map[string]any{"content": []mcpContent{{Type: "text", Text: text}}}
	// a method returning only an error has no structuredContent
	if string(b) != "null" {
		out["structuredContent"] = json.RawMessage(b)
	}
	return h.complete(msg.ID, out)
}

func (h *McpHandler) write(w http.ResponseWriter, status int, resp any) {
	b, _ := h.methodHandler.opts.JsonHandler.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func (h *McpHandler) allowOrigin(origin string, req *http.Request) bool {
	return h.opts.AllowOrigin != nil && h.opts.AllowOrigin(origin, req)
}
