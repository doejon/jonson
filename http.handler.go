package jonson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"runtime/debug"
)

func init() {
	respMethodNotAllowed, _ = json.Marshal(NewRpcErrorResponse(nil, ErrServerMethodNotAllowed))
}

var respMethodNotAllowed []byte

var TypeHttpRequest = reflect.TypeOf((**HttpRequest)(nil)).Elem()

type HttpRequest struct {
	Shareable
	ShareableAcrossImpersonation
	*http.Request
}

// RequireHttpRequest returns the current http request.
// In case the connection was created using websockets,
// the underlying http.Request opening the connection
// will be returned.
func RequireHttpRequest(ctx *Context) *HttpRequest {
	if v := ctx.Require(TypeHttpRequest); v != nil {
		return v.(*HttpRequest)
	}
	return nil
}

type HttpResponseWriter struct {
	Shareable
	ShareableAcrossImpersonation
	http.ResponseWriter
}

var TypeHttpResponseWriter = reflect.TypeOf((**HttpResponseWriter)(nil)).Elem()

// RequireHttpResponseWriter returns the current http response writer
// which is used to handle the ongoing request's response.
func RequireHttpResponseWriter(ctx *Context) *HttpResponseWriter {
	if v := ctx.Require(TypeHttpResponseWriter); v != nil {
		return v.(*HttpResponseWriter)
	}
	return nil
}

type HttpRegexpMatchedParts struct {
	Shareable
	ShareableAcrossImpersonation
	Parts []string
}

var TypeHttpRegexpMatchedParts = reflect.TypeOf((**HttpRegexpMatchedParts)(nil)).Elem()

// The HttpRegexpHandler will accept regular expressions and
// will register those as default http endpoints. Those methods cannot
// be called within the rpc world
type HttpRegexpHandler struct {
	factory       *Factory
	methodHandler *MethodHandler
	patterns      map[*regexp.Regexp]func(http.ResponseWriter, *http.Request, []string)
}

func NewHttpRegexpHandler(factory *Factory, methodHandler *MethodHandler) *HttpRegexpHandler {
	return &HttpRegexpHandler{
		factory:       factory,
		methodHandler: methodHandler,
		patterns:      map[*regexp.Regexp]func(http.ResponseWriter, *http.Request, []string){},
	}
}

// Handle will handle an incoming http request
func (h *HttpRegexpHandler) Handle(w http.ResponseWriter, req *http.Request) bool {
	// check for prefix calls
	for p, h := range h.patterns {
		if parts := p.FindStringSubmatch(req.URL.Path); len(parts) > 0 {
			h(w, req, parts)
			return true
		}
	}
	return false
}

type regexpWriter struct {
	http.ResponseWriter
	written bool
}

func (w *regexpWriter) WriteHeader(status int) {
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *regexpWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

type registeredRegexp struct {
	rpcMethod string
}

// WithRpcMethod defines the rpc method which will be used
// instead of the default rpc method derived from the regexp pattern
func (r *registeredRegexp) WithRpcMethod(method string) {
	r.rpcMethod = method
}

// RegisterRegexp registers a direct http func for a given regexp
func (h *HttpRegexpHandler) RegisterRegexp(pattern *regexp.Regexp, handler any) *registeredRegexp {
	strPattern := pattern.String()
	out := &registeredRegexp{
		rpcMethod: strPattern,
	}

	system, method, version, err := ParseRpcMethod(strPattern)
	if err == nil {
		out.rpcMethod = GetDefaultMethodName(system, method, version)
	}

	rt := reflect.TypeOf(handler)
	rv := reflect.ValueOf(handler)

	// check params provided by handler
	if err := h.checkParams(rt); err != nil {
		panic(err)
	}

	// check if pattern has been registered twice
	for k := range h.patterns {
		if k.String() == strPattern {
			panic(fmt.Sprintf("tried to register regexp pattern %s twice", strPattern))
		}
	}

	h.patterns[pattern] = func(w http.ResponseWriter, r *http.Request, parts []string) {
		wtr := &regexpWriter{
			ResponseWriter: w,
			written:        false,
		}

		ctx := NewContext(r.Context(), h.factory, h.methodHandler)
		ctx.StoreValue(TypeHttpRequest, &HttpRequest{
			Request: r,
		})
		ctx.StoreValue(TypeHttpResponseWriter, &HttpResponseWriter{
			ResponseWriter: wtr,
		})
		ctx.StoreValue(TypeSecret, h.methodHandler.errorEncoder)
		ctx.StoreValue(TypeHttpRegexpMatchedParts, &HttpRegexpMatchedParts{
			Parts: parts,
		})

		ctx.StoreValue(TypeRpcMeta, &RpcMeta{
			Method:     out.rpcMethod,
			HttpMethod: getRpcHttpMethod(r.Method),
			Source:     RpcSourceHttp,
		})

		func() {
			// catch any errors thrown by handler
			defer func() {
				e := recover()
				if e == nil {
					return
				}
				recoverErr := getRecoverError(e)

				// fwd information to the outside world
				if _, ok := recoverErr.(*Error); !ok {
					stack := string(debug.Stack())

					// panic, thrown unintentionally (cannot cast to an explicit jonson.Error)
					err = &PanicError{
						Err:    recoverErr,
						ID:     []byte("-1"),
						Method: out.rpcMethod,
						Stack:  stack,
					}

					// let's log the unintended panic
					h.methodHandler.getLogger(ctx).Error("method handler: panic",
						"error", recoverErr,
						"stack", stack,
						"regexpHandler", strPattern,
						"httpMethod", r.Method,
					)
				} else {
					// the function threw an error we must handle
					// and return to the outside world;
					// this error was most likely thrown intentionally since it's following
					// the jonson conventions
					err = recoverErr
					// no need to log, the developer caused the panic intentionally
				}
			}()

			args, err := h.buildArgs(ctx, strPattern, rt)
			if err != nil {
				panic(err)
			}

			// call handler
			rv.Call(args)
		}()
		// finalize context
		err = ctx.Finalize(err)

		if wtr.written {
			return
		}

		// in case error was thrown but no response has been written yet, we got
		// ourselves a panic. Let's respond with a well-formatted error message
		// using json
		if err != nil {
			// write error response
			errorResp, ok := err.(*Error)
			if !ok {
				errorResp = ErrInternal.CloneWithData(&ErrorData{
					Debug: h.methodHandler.errorEncoder.Encode(err.Error()),
				})
			}
			b, _ := h.methodHandler.opts.JsonHandler.Marshal(errorResp)
			w.Header().Set("Content-Type", "application/json")
			wtr.WriteHeader(HttpStatusCode(errorResp))
			wtr.Write(b)
			return
		}
	}
	return out
}

func (h *HttpRegexpHandler) buildArgs(ctx *Context, pattern string, tp reflect.Type) ([]reflect.Value, error) {
	out := []reflect.Value{
		reflect.ValueOf(ctx),
	}

	// first is always context, skip
	for i := 1; i < tp.NumIn(); i++ {
		rti := tp.In(i)
		// call provider using panic recovery
		v, err := func() (v any, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = getRecoverError(r)
				}
			}()
			v = ctx.Require(rti)
			return
		}()
		if err != nil {
			h.methodHandler.getLogger(ctx).Warn(fmt.Sprintf("http regexp handler: provider for type '%s' failed", rti.String()),
				"error", err,
				"regexpHandler", pattern,
			)
			return nil, err
		}
		out = append(out, reflect.ValueOf(v))
	}

	return out, nil
}

func (h *HttpRegexpHandler) checkParams(rt reflect.Type) error {
	if rt.Kind() != reflect.Func {
		panic(errors.New("http regexp handler: handler function must be a method"))
	}

	seenArgs := map[reflect.Type]struct{}{}
	providerTypes := h.factory.Types()
	providerTypes = append(
		providerTypes,

		TypeContext,
		TypeHttpRequest,
		TypeHttpResponseWriter,
		TypeHttpRegexpMatchedParts,
		TypeRpcMeta,
	)

	for i := 0; i < rt.NumIn(); i++ {
		rti := rt.In(i)
		if _, ok := seenArgs[rti]; ok {
			return fmt.Errorf("http regexp handler: tried to require type twice: %s", rti.String())
		}

		if !isTypeSupported(providerTypes, rti) {
			return fmt.Errorf("http regexp handler: type not supported: %s", rti.String())
		}

		seenArgs[rti] = struct{}{}
		if i == 0 && rti != TypeContext {
			return fmt.Errorf("http regexp handler: first argument needs to be of type jonson.Context")
		}
	}

	if len(seenArgs) <= 0 {
		return fmt.Errorf("http regexp handler: no function arguments provided; at least context.Context as the first argument is required")
	}

	return nil
}

type HttpRpcHandler struct {
	path          string
	methodHandler *MethodHandler
}

func NewHttpRpcHandler(methodHandler *MethodHandler, path string) *HttpRpcHandler {
	return &HttpRpcHandler{
		path:          path,
		methodHandler: methodHandler,
	}
}

// Handle will handle an incoming http request
func (h *HttpRpcHandler) Handle(w http.ResponseWriter, req *http.Request) bool {
	// check for exact matches
	if req.URL.Path != h.path {
		return false
	}

	// the http rpc handler only accepts post to prevent from xss scripting
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write(respMethodNotAllowed)
		return true
	}

	var (
		resp  []any
		batch bool
	)

	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.methodHandler.getLogger(nil).Warn("rpc http handler: read error", "error", err)
		resp = []any{NewRpcErrorResponse(nil, ErrParse)}
	} else {
		resp, batch = h.methodHandler.processRpcMessages(RpcSourceHttpRpc, RpcHttpMethodPost, req, w, nil, body)
	}

	if len(resp) == 0 {
		// nothing to return but obviously everything was ok
		w.WriteHeader(http.StatusOK)
		return true
	}

	var b []byte
	if !batch {
		// For a single request, marshal only the first (and only) element.
		b, _ = h.methodHandler.opts.JsonHandler.Marshal(resp[0])
	} else {
		// For a batch request, marshal the entire slice.
		b, _ = h.methodHandler.opts.JsonHandler.Marshal(resp)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)

	return true
}

// HttpMethodHandler will register all methods within the methodHandler
// as separate http endpoints, such as:
// system/method.v1,
// system/another-method.v1
// in case the method accepts params, POST is enforced,
// otherwise GET will be used as the accepting http method.
type HttpMethodHandler struct {
	methodHandler *MethodHandler
}

func NewHttpMethodHandler(methodHandler *MethodHandler) *HttpMethodHandler {
	return &HttpMethodHandler{
		methodHandler: methodHandler,
	}
}

// Handle handles the incoming http request and parses the payload.
// Since we do not need the json rpc wrapper for these calls (method is the http path),
// we only expect a _single_ data json object inside the body.
// in case
func (h *HttpMethodHandler) Handle(w http.ResponseWriter, req *http.Request) bool {
	p := req.URL.Path
	if len(p) > 0 {
		// trim leading slash
		p = p[1:]
	}
	endpoint, ok := h.methodHandler.endpoints[p]
	if !ok {
		return false
	}

	pl := json.RawMessage{}
	var resp any
	var err error

	// we need to unmarshal the body _only_ in case
	// parameters are expected; Otherwise the body
	// can/will be empty
	if endpoint.paramsPos >= 0 {
		err = h.methodHandler.opts.JsonHandler.NewDecoder(req.Body).Decode(&pl)
	}

	method := RpcHttpMethod(req.Method)

	if err != nil {
		h.methodHandler.getLogger(nil).Warn("http method handler: read error", "error", err)
		resp = NewRpcErrorResponse(nil, ErrParse)
	} else {
		resp = h.methodHandler.processRpcMessage(RpcSourceHttp, method, req, w, nil, &RpcRequest{
			Version: "2.0",
			Method:  p,
			// we do not have any IDs here -> set to -1
			ID:     []byte("-1"),
			Params: pl,
		})
	}

	// in case no data is preset, we return a NoContent response by default
	httpStatus := http.StatusNoContent
	var dataToMarshal any
	successResp, ok := resp.(*RpcResultResponse)
	if ok {
		// got data, switch to OK
		httpStatus = http.StatusOK
		dataToMarshal = successResp.Result
	}

	errorResp, ok := resp.(*RpcErrorResponse)
	if ok {
		httpStatus = HttpStatusCode(errorResp.Error)
		dataToMarshal = errorResp.Error
	}

	// single response for these calls allowed only
	b, _ := h.methodHandler.opts.JsonHandler.Marshal(dataToMarshal)

	// make sure we're responding with application/json for everything
	if len(b) > 0 {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(httpStatus)
	w.Write(b)
	return true
}
