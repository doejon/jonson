package jonson

import "reflect"

// httpMethodProvider allows us to ensure
// a specific http method has been used when using
// the HttpMethodHandler.
// The HttpMethodProvider will be provided automatically.
type httpMethodProvider struct {
}

func newHttpMethodProvider() *httpMethodProvider {
	return &httpMethodProvider{}
}

func (h *httpMethodProvider) NewHttpPost(ctx *Context) HttpPost {
	meta := RequireRpcMeta(ctx)
	// we only care for http sources
	if meta.Source != RpcSourceHttp {
		return &httpPost{}
	}
	req := RequireHttpRequest(ctx)
	if req.Method != "POST" {
		panic(ErrServerMethodNotAllowed)
	}

	return &httpPost{}
}

func (h *httpMethodProvider) NewHttpGet(ctx *Context) HttpGet {
	meta := RequireRpcMeta(ctx)
	switch meta.Source {
	case RpcSourceInternal:
		return &httpGet{}
	case RpcSourceHttp:
	default:
		// http rpc, ws and mcp are POST-only
		panic(ErrServerMethodNotAllowed)
	}
	req := RequireHttpRequest(ctx)
	if req.Method != "GET" {
		panic(ErrServerMethodNotAllowed)
	}
	return &httpGet{}
}

var TypeHttpPost = reflect.TypeFor[HttpPost]()

// HttpPost can be used in case you want to enforce POST within your remote procedures
// served over http.
// In case you're using websockets or http rpc, POST cannot be enforced.
// For rpc over http (single endpoint), POST will be enforced by default.
// func (s *System) UpdateProfileV1(ctx *jonson.Context, _ jonson.HttpPost) error{}
type HttpPost interface {
	__post()
}
type httpPost struct{}

func (h *httpPost) __post() {}

var TypeHttpGet = reflect.TypeFor[HttpGet]()

// HttpGet can be used in case you want to enforce GET within your remote procedures
// served over http.
// Http rpc, websockets and mcp only speak POST: calls to a HttpGet method over
// them fail with ErrServerMethodNotAllowed, and McpHandler.Expose panics on them.
// Example:
// func (s *System) GetProfileV1(ctx *jonson.Context, _ jonson.HttpGet) error{}
type HttpGet interface {
	__get()
}
type httpGet struct{}

func (h *httpGet) __get() {}
