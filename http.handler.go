package jonson

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
)

func init() {
	respMethodNotAllowed, _ = json.Marshal(NewRpcErrorResponse(nil, ErrServerMethodNotAllowed))
}

var respMethodNotAllowed []byte

var TypeHttpRequest = reflect.TypeOf((**HttpRequest)(nil)).Elem()

type HttpRequest struct {
	Shareable
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
		defer ctx.Finalize(nil)

		handler(ctx, w, r, parts)
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
	// check for prefix calls
	// check for /api/rpc request
	if req.URL.Path != h.path {
		return false
	}

	// the http rpc handler only accepts post to prevent from xss scripting
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write(respMethodNotAllowed)
		return true
	}

	var (
		resp  []any
		batch bool
	)

	body, err := io.ReadAll(req.Body)
	if err != nil {
		h.methodHandler.logger.Warn("rpc http handler: read error", "error", err)
		resp = []any{NewRpcErrorResponse(nil, ErrParse)}
	} else {
		resp, batch = h.methodHandler.processRpcMessages(RpcSourceHttpRpc, RpcHttpMethodPost, req, w, nil, body)
	}

	if len(resp) == 0 {
		// nothing to return but obviously everything was ok
		w.WriteHeader(http.StatusOK)
		return true
	}

	// no batch response
	if !batch {
		// single response
		b, _ := json.Marshal(resp[0])
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return true
	}

	// batch response
	b, _ := json.Marshal(resp)
	w.WriteHeader(http.StatusOK)
	w.Write(b)
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
		err = json.NewDecoder(req.Body).Decode(&pl)
	}

	method := RpcHttpMethod(req.Method)

	if err != nil {
		h.methodHandler.logger.Warn("rpc http handler: read error", "error", err)
		resp = NewRpcErrorResponse(nil, ErrParse)
	} else {
		resp = h.methodHandler.processRpcMessage(RpcSourceHttp, method, req, w, nil, &RpcRequest{
			Version: "2.0",
			Method:  p,
			// we do not have any IDs here -> set to -1
			ID:     []byte("-1"),
			Params: pl,
		}, nil)
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
		switch errorResp.Error.Code {
		case ErrServerMethodNotAllowed.Code:
			httpStatus = http.StatusMethodNotAllowed
		case ErrInvalidParams.Code:
			fallthrough
		case ErrParse.Code:
			httpStatus = http.StatusBadRequest
		case ErrUnauthorized.Code:
			fallthrough
		case ErrUnauthenticated.Code:
			httpStatus = http.StatusForbidden
		case ErrMethodNotFound.Code:
			httpStatus = http.StatusNotFound
		default:
			httpStatus = http.StatusInternalServerError
		}
	}

	// single response for these calls allowed only
	b, _ := json.Marshal(dataToMarshal)
	w.WriteHeader(httpStatus)
	w.Write(b)
	return true

}
