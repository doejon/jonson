package jonson

import (
	"bytes"
	"encoding/json"
	"log"
	"reflect"
)

// RPC internal errors
var (
	ErrParse                  = &Error{Code: -32700, Message: "Parse error"}
	ErrMethodNotFound         = &Error{Code: -32601, Message: "Method not found"}
	ErrInvalidParams          = &Error{Code: -32602, Message: "Invalid params"}
	ErrInternal               = &Error{Code: -32603, Message: "Internal error"}
	ErrServerMethodNotAllowed = &Error{Code: -32000, Message: "Server error: method not allowed"}
)

// RPCRequest object
type RPCRequest struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// RPCNotification object
type RPCNotification struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func NewRPCNotification(method string, payload any) *RPCNotification {
	var params json.RawMessage
	if payloadRaw, ok := payload.(json.RawMessage); ok {
		params = payloadRaw
	} else {
		params, _ = json.Marshal(payload)
	}

	return &RPCNotification{
		Version: "2.0",
		Method:  method,
		Params:  params,
	}
}

// UnmarshalAndValidate fills the given interface with the supplied params
func (r *RPCRequest) UnmarshalAndValidate(errEncoder Secret, out any, bindata []byte) error {
	dec := json.NewDecoder(bytes.NewReader([]byte(r.Params)))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		log.Printf("failed to decode payload: %s", err)
		return ErrInvalidParams.CloneWithData(&ErrorData{
			Debug: errEncoder.Encode(err.Error()),
		})
	}

	// optional: if bindata is set set a field called BinData in the target struct
	if bindata != nil {
		rv := reflect.ValueOf(out).Elem()
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			if rt.Field(i).Name == "Bindata" {
				rv.Field(i).Set(reflect.ValueOf(bindata))
				break
			}
		}
	}

	// start validation process
	// TODO: return Validate(errEncoder, out)
	return nil
}

// RPCResponseHeader object
type RPCResponseHeader struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
}

// NewRPCResponseHeader returns a new ResponseHeader
func NewRPCResponseHeader(id json.RawMessage) RPCResponseHeader {
	return RPCResponseHeader{
		Version: "2.0",
		ID:      id,
	}
}

// RPCErrorResponse object
type RPCErrorResponse struct {
	RPCResponseHeader
	Error *Error `json:"error"`
}

// NewRPCErrorResponse returns a new ErrorResponse
func NewRPCErrorResponse(id json.RawMessage, e *Error) *RPCErrorResponse {
	return &RPCErrorResponse{
		RPCResponseHeader: NewRPCResponseHeader(id),
		Error:             e,
	}
}

// RPCResultResponse object
type RPCResultResponse struct {
	RPCResponseHeader
	Result any `json:"result"`
}

// NewRPCResultResponse returns a new ResultResponse
func NewRPCResultResponse(id json.RawMessage, result any) *RPCResultResponse {
	return &RPCResultResponse{
		RPCResponseHeader: NewRPCResponseHeader(id),
		Result:            result,
	}
}
