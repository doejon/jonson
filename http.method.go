package jonson

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
	// we only care for http sources
	if meta.Source != RpcSourceHttp {
		return &httpGet{}
	}
	req := RequireHttpRequest(ctx)
	if req.Method != "GET" {
		panic(ErrServerMethodNotAllowed)
	}
	return &httpGet{}
}

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

// HttpGet can be used in case you want to enforce GET within your remote procedures
// served over http.
// In case you're using websockets or http rpc, GET cannot be enforced.
// For rpc over http (single endpoint), POST will be enforced by default.
// Example:
// func (s *System) GetProfileV1(ctx *jonson.Context, _ jonson.HttpGet) error{}
type HttpGet interface {
	__get()
}
type httpGet struct{}

func (h *httpGet) __get() {}
