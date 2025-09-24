package jonson

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
)

// Rpc internal errors
var (
	ErrParse                  = &Error{Code: -32700, Message: "Parse error"}
	ErrMethodNotFound         = &Error{Code: -32601, Message: "Method not found"}
	ErrInvalidParams          = &Error{Code: -32602, Message: "Invalid params"}
	ErrInternal               = &Error{Code: -32603, Message: "Internal error"}
	ErrServerMethodNotAllowed = &Error{Code: -32000, Message: "Server error: method not allowed"}
	ErrUnauthorized           = &Error{Code: -32001, Message: "Not authorized"}
	ErrUnauthenticated        = &Error{Code: -32002, Message: "Not authenticated"}
	ErrTooManyRequests        = &Error{Code: -32003, Message: "Too many requests"}
)

func HttpStatusCode(r *Error) int {
	switch r.Code {
	case ErrServerMethodNotAllowed.Code:
		return http.StatusMethodNotAllowed
	case ErrInvalidParams.Code:
		fallthrough
	case ErrParse.Code:
		return http.StatusBadRequest
	case ErrUnauthorized.Code:
		fallthrough // do not use 401 -> triggers basic auth
	case ErrUnauthenticated.Code:
		return http.StatusForbidden
	case ErrMethodNotFound.Code:
		return http.StatusNotFound
	case ErrTooManyRequests.Code:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// RpcRequest object
type RpcRequest struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type RpcRequestLogInfo struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params string          `json:"params"`
}

func (r *RpcRequest) getLogInfo(secret Secret) *RpcRequestLogInfo {
	p := ""
	if r.Params == nil {
		p = "<nil>"
	} else {
		// make sure to encode params
		p = secret.Encode(string(r.Params))
	}
	return &RpcRequestLogInfo{
		ID:     r.ID,
		Method: r.Method,
		Params: p,
	}
}

// RpcNotification object
type RpcNotification struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func NewRpcNotification(ctx *Context, method string, payload any) *RpcNotification {
	var params json.RawMessage
	if payloadRaw, ok := payload.(json.RawMessage); ok {
		params = payloadRaw
	} else {
		params, _ = RequireJsonHandler(ctx).Marshal(payload)
	}

	return &RpcNotification{
		Version: "2.0",
		Method:  method,
		Params:  params,
	}
}

// UnmarshalAndValidate fills the given interface with the supplied params
func (r *RpcRequest) UnmarshalAndValidate(
	ctx *Context,
	errEncoder Secret,
	out any,
) error {
	dec := RequireJsonHandler(ctx).NewDecoder(bytes.NewReader([]byte(r.Params)))

	if err := dec.Decode(out); err != nil {
		return ErrInvalidParams.CloneWithData(&ErrorData{
			Debug: errEncoder.Encode(err.Error()),
			Details: []*Error{
				{
					Code:    ErrInternal.Code,
					Message: "failed to decode params",
					Data: &ErrorData{
						Debug: errEncoder.Encode(string(r.Params)),
					},
				},
			},
		})
	}

	// start validation process in case the
	// params can be validated
	canValidate, ok := out.(ValidatedParams)
	if ok {
		result := Validate(ctx, errEncoder, canValidate)
		if result == nil {
			return nil
		}
		return result
	}
	return nil
}

// RpcResponseHeader object
type RpcResponseHeader struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
}

// NewRpcResponseHeader returns a new ResponseHeader
func NewRpcResponseHeader(id json.RawMessage) RpcResponseHeader {
	return RpcResponseHeader{
		Version: "2.0",
		ID:      id,
	}
}

// RpcErrorResponse object
type RpcErrorResponse struct {
	RpcResponseHeader
	Error *Error `json:"error"`
}

// NewRpcErrorResponse returns a new ErrorResponse
func NewRpcErrorResponse(id json.RawMessage, e *Error) *RpcErrorResponse {
	return &RpcErrorResponse{
		RpcResponseHeader: NewRpcResponseHeader(id),
		Error:             e,
	}
}

// RpcResultResponse object
type RpcResultResponse struct {
	RpcResponseHeader
	Result any `json:"result"`
}

// NewRpcResultResponse returns a new ResultResponse
func NewRpcResultResponse(id json.RawMessage, result any) *RpcResultResponse {
	return &RpcResultResponse{
		RpcResponseHeader: NewRpcResponseHeader(id),
		Result:            result,
	}
}

type RpcHttpMethod string

const (
	RpcHttpMethodGet     RpcHttpMethod = "GET"
	RpcHttpMethodPost    RpcHttpMethod = "POST"
	RpcHttpMethodUnknown RpcHttpMethod = "UNKNOWN"
)

// getRpcHttpMethod converts a string to rpc http method which can be used within jonson
func getRpcHttpMethod(s string) RpcHttpMethod {
	switch s {
	case "GET":
		return RpcHttpMethodGet
	case "POST":
		return RpcHttpMethodPost
	default:
		return RpcHttpMethodUnknown
	}
}

type RpcSource string

const (
	RpcSourceHttp    RpcSource = "http"
	RpcSourceHttpRpc RpcSource = "httpRpc"
	RpcSourceWs      RpcSource = "ws"

	// This rpc call will be set in case
	// one rpc calls another rpc
	RpcSourceInternal RpcSource = "internal"
)

// RpcMeta contains Rpc call meta data information that has been set whenever
// a call towards an Rpc method happened
type RpcMeta struct {
	Method     string
	HttpMethod RpcHttpMethod
	Source     RpcSource
}

var TypeRpcMeta = reflect.TypeOf((**RpcMeta)(nil)).Elem()

func RequireRpcMeta(ctx *Context) *RpcMeta {
	if v := ctx.Require(TypeRpcMeta); v != nil {
		// we do return a copy here
		x := *v.(*RpcMeta)
		return &x
	}
	return nil
}
