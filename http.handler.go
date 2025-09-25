package jonson

import (
	"encoding/json"
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

// RegisterRegexp registers a direct http func for a given regexp
func (h *HttpRegexpHandler) RegisterRegexp(pattern *regexp.Regexp, handler func(ctx *Context, w http.ResponseWriter, r *http.Request, parts []string)) {
	h.patterns[pattern] = func(w http.ResponseWriter, r *http.Request, parts []string) {
		ctx := NewContext(r.Context(), h.factory, h.methodHandler)
		ctx.StoreValue(TypeHttpRequest, &HttpRequest{
			Request: r,
		})
		ctx.StoreValue(TypeHttpResponseWriter, &HttpResponseWriter{
			ResponseWriter: w,
		})
		ctx.StoreValue(TypeSecret, h.methodHandler.errorEncoder)

		system, method, version, err := ParseRpcMethod(pattern.String())
		rpcMethod := ""
		if err == nil {
			rpcMethod = GetDefaultMethodName(system, method, version)
		}

		ctx.StoreValue(TypeRpcMeta, &RpcMeta{
			Method:     rpcMethod,
			HttpMethod: getRpcHttpMethod(r.Method),
			Source:     RpcSourceHttp,
		})

		wtr := &regexpWriter{
			ResponseWriter: w,
			written:        false,
		}

		func() {
			// catch any errors thrown by handler
			defer func() {
				recoverErr := getRecoverError(recover())

				// fwd information to the outside world
				if _, ok := recoverErr.(*Error); !ok {
					stack := string(debug.Stack())

					// panic, thrown unintentionally (cannot cast to an explicit jonson.Error)
					err = &PanicError{
						Err:    recoverErr,
						ID:     []byte("-1"),
						Method: pattern.String(),
						Stack:  stack,
					}

					// let's log the unintended panic
					h.methodHandler.getLogger(ctx).Error("method handler: panic",
						"error", recoverErr,
						"stack", stack,
						"regexpHandler", pattern.String(),
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

			// call handler
			handler(ctx, wtr, r, parts)
		}()
		ctx.Finalize(err)
		if wtr.written {
			return
		}
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

	successResp, ok := resp.(*RpcResultResponse)
	httpStatus := http.StatusOK
	var dataToMarshal = resp
	if ok {
		dataToMarshal = successResp.Result
	}

	errorResp, ok := resp.(*RpcErrorResponse)
	if ok {
		dataToMarshal = errorResp.Error
		httpStatus = HttpStatusCode(errorResp.Error)
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
