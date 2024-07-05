package jonson

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"reflect"
	"regexp"
)

var TypeHTTPRequest = reflect.TypeOf((**http.Request)(nil)).Elem()

var respMethodNotAllowed []byte

func init() {
	respMethodNotAllowed, _ = json.Marshal(NewRPCErrorResponse(nil, ErrServerMethodNotAllowed))
}

func RequireHTTPRequest(ctx *Context) *http.Request {
	if v := ctx.Require(TypeHTTPRequest); v != nil {
		return v.(*http.Request)
	}
	return nil
}

var TypeHTTPResponseWriter = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()

func RequireHTTPResponseWriter(ctx *Context) http.ResponseWriter {
	if v := ctx.Require(TypeHTTPResponseWriter); v != nil {
		return v.(http.ResponseWriter)
	}
	return nil
}

// The HttpRegexpHandler will accept regular expressions and
// will register those as default http endpoints. Those methods cannot
// be called within the rpc world
type HttpRegexpHandler struct {
	provider      Provider
	methodHandler *MethodHandler
	patterns      map[*regexp.Regexp]func(http.ResponseWriter, *http.Request, []string)
}

func NewHttpRegexpHandler(provider Provider, methodHandler *MethodHandler) *HttpRegexpHandler {
	return &HttpRegexpHandler{
		provider:      provider,
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
		ctx := NewContext(r.Context(), h.provider, h.methodHandler)
		ctx.StoreValue(TypeHTTPRequest, r)
		ctx.StoreValue(TypeHTTPResponseWriter, w)
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
		log.Print("rpc http handler: read error: ", err)
		resp = []any{NewRPCErrorResponse(nil, ErrParse)}
	} else {
		resp, batch = h.methodHandler.processMessages(req, w, nil, body)
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
	acceptedHttpMethod := "POST"
	if endpoint.paramsPos < 0 {
		// no params available, we accept only GET
		acceptedHttpMethod = "GET"
	}
	if acceptedHttpMethod != req.Method {
		w.WriteHeader(http.StatusMethodNotAllowed)
		b, _ := json.Marshal(ErrServerMethodNotAllowed)
		w.Write(b)
		return true
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

	if err != nil {
		log.Print("rpc http handler: read error: ", err)
		resp = NewRPCErrorResponse(nil, ErrParse)
	} else {
		resp = h.methodHandler.processMessage(req, w, nil, &RPCRequest{
			Version: "2.0",
			Method:  p,
			// we do not have any IDs here -> set to -1
			ID:     []byte("-1"),
			Params: pl,
		}, nil)
	}

	successResp, ok := resp.(*RPCResultResponse)
	httpStatus := http.StatusOK
	var dataToMarshal = resp
	if ok {
		dataToMarshal = successResp.Result
	}
	errorResp, ok := resp.(*RPCErrorResponse)
	if ok {
		dataToMarshal = errorResp.Error
		switch errorResp.Error.Code{
		case ErrInvalidParams.Code:
		case ErrParse.Code:
			httpStatus = http.StatusBadRequest
			break
		case ErrUnauthorized.Code.Code:
			httpStatus = http.StatusForbidden
			break
		case ErrUnauthenticated.Code:
			httStatus = http.StatusForbidden
			break
		case ErrMethodNotFound.Code:
			httpStatus =  http.StatusNotFound
			break
		default:
			httpStatus = http.StatusInternalServerError
		}
		if errorResp.Error.Code == ErrInvalidParams.Code{

		} else if 
		httpStatus = http.StatusInternalServerError
	}

	// single response for these calls allowed only
	b, _ := json.Marshal(dataToMarshal)
	w.WriteHeader(httpStatus)
	w.Write(b)
	return true

}
