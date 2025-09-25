package jonson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

// MethodDefinition is used by MustRegisterAPI
type MethodDefinition struct {
	System        string
	Method        string
	Version       uint64
	HandlerFunc   any
	methodContext reflect.Value
}

var (
	validIdentifierName = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	matchMethodName     = regexp.MustCompile(`^(.+)V([0-9]+)$`)
)

// SplitGoMethodName is used to parse a method used within go code (e.g. MyEndpointV1 --> method: my-endpoint version: 1)
func SplitGoMethodName(method string) (string, uint64) {
	sub := matchMethodName.FindStringSubmatch(method)
	if len(sub) != 3 {
		return "", 0
	}
	// we have found a valid method signature, build definition and try to register
	methodName := ToKebabCase(sub[1])
	version, _ := strconv.ParseUint(sub[2], 10, 64)
	return methodName, version
}

func GetDefaultMethodName(system string, method string, version uint64) string {
	return system + "/" + method + ".v" + strconv.FormatUint(version, 10)
}

func ParseRpcMethod(slug string) (_system string, _method string, _version uint64, _err error) {
	slug = strings.TrimPrefix(slug, "/")
	parts := strings.Split(slug, "/")
	if len(parts) != 2 {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case")
	}
	system := parts[0]
	method := parts[1]
	parts = strings.Split(method, ".v")
	if len(parts) != 2 {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; missing version")
	}
	method = parts[0]
	version, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; version not of type integer")
	}
	if version <= 0 {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; version <= 0")
	}

	if !validIdentifierName.MatchString(system) {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; system contains invalid characters")
	}

	if !validIdentifierName.MatchString(method) {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; method contains invalid characters")
	}

	if ToKebabCase(system) != system {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; system is not kebab-case")
	}
	if ToKebabCase(method) != method {
		return "", "", 0, errors.New("an rpc method follows the format of <system>/<method>.v<version> and needs to be kebab-case; method is not kebab-case")
	}

	return system, method, uint64(version), nil

}

type apiEndpoint struct {
	def           *MethodDefinition
	handlerFunc   reflect.Value
	methodContext reflect.Value
	paramsPos     int
	paramsType    reflect.Type
}

type MethodHandler struct {
	factory *Factory
	// methodName func(system string, method string, version uint64) string

	systems      map[reflect.Type]any
	endpoints    map[string]apiEndpoint
	errorEncoder Secret
	opts         *MethodHandlerOptions
}

// MissingValidationLevel allows us to set
// a validation level within the method handlers to
// enforce parameter validation or ignore
// validation if not present
type MissingValidationLevel string

const (
	MissingValidationLevelIgnore MissingValidationLevel = "ignore"
	MissingValidationLevelInfo   MissingValidationLevel = "info"
	MissingValidationLevelWarn   MissingValidationLevel = "warn"
	MissingValidationLevelError  MissingValidationLevel = "error"
	MissingValidationLevelFatal  MissingValidationLevel = "fatal"
)

var validMissingValidationLevel = map[MissingValidationLevel]struct{}{
	MissingValidationLevelIgnore: {},
	MissingValidationLevelInfo:   {},
	MissingValidationLevelWarn:   {},
	MissingValidationLevelError:  {},
	MissingValidationLevelFatal:  {},
}

type MethodHandlerOptions struct {
	MissingValidationLevel MissingValidationLevel
	// allows you to intercept the rpc log message before it reaches the final
	// log; This allows you to e.g. hide parameters
	RpcRequestLogInfoInterceptor func(info *RpcRequestLogInfo) *RpcRequestLogInfo
	JsonHandler                  JsonHandler
}

func NewMethodHandler(
	factory *Factory,
	errorEncoder Secret,
	opts *MethodHandlerOptions,
) *MethodHandler {
	if opts == nil {
		opts = &MethodHandlerOptions{}
	}
	if _, ok := validMissingValidationLevel[opts.MissingValidationLevel]; !ok {
		opts.MissingValidationLevel = MissingValidationLevelInfo
	}
	if opts.RpcRequestLogInfoInterceptor == nil {
		// by default, forward log info as is
		opts.RpcRequestLogInfoInterceptor = func(x *RpcRequestLogInfo) *RpcRequestLogInfo {
			return x
		}
	}
	if opts.JsonHandler == nil {
		opts.JsonHandler = NewDefaultJsonHandler()
	}

	// register the error encoder
	factory.RegisterProvider(newSecretProvider(errorEncoder))
	factory.RegisterProvider(newJsonHandlerProvider(opts.JsonHandler))

	return &MethodHandler{
		factory:      factory,
		systems:      map[reflect.Type]any{},
		endpoints:    map[string]apiEndpoint{},
		errorEncoder: errorEncoder,
		opts:         opts,
	}
}

// in order to make sure to get an initialized logger with all the
// information about the "surrounding" context, let's first try
// to fetch the logger from the context before using it
func (m *MethodHandler) getLogger(ctx *Context) (out *slog.Logger) {
	// in case no context is given, return the default logger
	if ctx == nil {
		return m.factory.logger
	}

	defer func() {
		if err := recover(); err != nil {
			out = m.factory.logger
		}
	}()
	out = RequireLogger(ctx)
	return out
}

// GetSystem returns a system. The function will panic in
// case system does not exist
func (m *MethodHandler) GetSystem(sys any) any {
	tof := reflect.TypeOf(sys)
	out, ok := m.systems[tof]
	if !ok {
		panic(fmt.Errorf("getSystem: system %v does not exist", tof))
	}
	return out
}

// RegisterSystem registers an entire system using reflect based method lookups
func (m *MethodHandler) RegisterSystem(sys any, routeDebugger ...func(s string)) {
	rv := reflect.ValueOf(sys)
	rt := reflect.TypeOf(sys)
	m.systems[rt] = sys

	if rt.Kind() != reflect.Ptr {
		panic(errors.New("registerSystem: expected ptr to struct"))
	}

	rte := rt.Elem()
	if rte.Kind() != reflect.Struct {
		panic(errors.New("registerSystem: expected ptr to struct"))
	}
	systemName := ToKebabCase(rte.Name())

	for i := 0; i < rt.NumMethod(); i++ {
		rtm := rt.Method(i)

		if methodName, version := SplitGoMethodName(rtm.Name); version > 0 {
			// we have found a valid method signature, build definition and try to register
			for _, v := range routeDebugger {
				v(GetDefaultMethodName(systemName, methodName, version))
			}

			m.RegisterMethod(&MethodDefinition{
				System:        systemName,
				Method:        methodName,
				Version:       version,
				HandlerFunc:   rtm.Func.Interface(),
				methodContext: rv,
			})
		}
	}
}

// RegisterMethod registers a new method
func (m *MethodHandler) RegisterMethod(def *MethodDefinition) {
	if !validIdentifierName.MatchString(def.System) {
		panic(errors.New("method handler: invalid system"))
	}
	if !validIdentifierName.MatchString(def.Method) {
		panic(errors.New("method handler: invalid method"))
	}

	endpoint := GetDefaultMethodName(def.System, def.Method, def.Version)
	if _, exists := m.endpoints[endpoint]; exists {
		panic(errors.New("method handler: endpoint already registered"))
	}

	rv := reflect.ValueOf(def.HandlerFunc)
	rt := rv.Type()
	if rt.Kind() != reflect.Func {
		panic(errors.New("method handler: handler function must be a method"))
	}

	paramShift := 0
	if !def.methodContext.IsNil() {
		// we have received a bounded method. we need to pass its context as first argument
		paramShift = 1
	}

	// we need to scan each argument from handlerFunc to see if it's compatible with our assumptions
	var (
		seenTypes           = map[reflect.Type]struct{}{}
		typeParams          reflect.Type
		argPosParams        = -1
		paramsSafeguardType = reflect.TypeOf((*paramsSafeguard)(nil)).Elem()
		validatedParamsType = reflect.TypeOf((*ValidatedParams)(nil)).Elem()
		providerTypes       = m.factory.Types()
	)

	// add types we implicitly support
	providerTypes = append(
		providerTypes,
		TypeContext,
		TypeHttpRequest,
		TypeHttpResponseWriter,
		TypeWSClient,
		TypeRpcMeta,
	)

	for i := paramShift; i < rt.NumIn(); i++ {
		rti := rt.In(i)

		// check if already seen
		if _, seen := seenTypes[rti]; seen {
			panic(errors.New("method handler:" + endpoint + " has multiple instances of " + rti.String()))
		}

		// check if we have a provider
		if isTypeSupported(providerTypes, rti) {
			seenTypes[rti] = struct{}{}
			continue
		}

		// allow one custom *struct{} type for params
		if rti.Implements(paramsSafeguardType) {
			if typeParams != nil {
				panic(errors.New("method handler:" + endpoint + " has additional param instance of " + rti.String()))
			}

			// do we have a ptr to a struct?
			if rti.Kind() != reflect.Ptr || rti.Elem().Kind() != reflect.Struct {
				panic(errors.New("method handler:" + endpoint + " has non ptr-to-struct param instance of " + rti.String()))
			}

			// does the param implement ValidatedParams?
			if !rti.Implements(validatedParamsType) {
				// not implemented
				errStr := endpoint + "'s param '" + rti.String() + "' does not implement 'JonsonValidate(v *jonson.Validator)' method;\n"
				switch m.opts.MissingValidationLevel {
				case MissingValidationLevelIgnore:
					// do nothing
				case MissingValidationLevelInfo:
					m.getLogger(nil).Info(errStr)
				case MissingValidationLevelWarn:
					m.getLogger(nil).Warn(errStr)
				case MissingValidationLevelError:
					m.getLogger(nil).Error(errStr)
				case MissingValidationLevelFatal:
					fallthrough
				default:
					panic(errStr)
				}

			}

			argPosParams = i
			typeParams = rti.Elem()
			continue
		}

		// fail
		panic(errors.New("method handler: " + endpoint + " requires unknown type " + rti.String()))
	}

	// check return types
	if rt.NumOut() < 1 || rt.NumOut() > 2 {
		panic(errors.New("method handler: " + endpoint + " may only return one or two arguments"))
	}
	et := rt.Out(rt.NumOut() - 1)
	if et.Kind() != reflect.Interface || et.Name() != "error" || et.PkgPath() != "" {
		panic(errors.New("method handler: " + endpoint + " must return error interface as last argument"))
	}

	m.endpoints[endpoint] = apiEndpoint{
		def:           def,
		handlerFunc:   rv,
		methodContext: def.methodContext,
		paramsPos:     argPosParams,
		paramsType:    typeParams,
	}
}

var _callMethodId = json.RawMessage("-1")

func (m *MethodHandler) CallMethod(_ctx *Context, method string, rpcHttpMethod RpcHttpMethod, payload any) (any, error) {
	jsonPayload, err := m.opts.JsonHandler.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// we need to make sure to create a new context here;
	ctx := _ctx.Fork()

	for _, v := range _ctx.values {
		if !v.valid {
			continue
		}
		// we only keep those values that have
		// been marked explicitly shareable
		if _, ok := v.val.(Shareable); ok {
			ctx.StoreValue(v.rt, v.val)
		}
	}

	ctx.StoreValue(TypeRpcMeta, &RpcMeta{
		Method:     method,
		HttpMethod: rpcHttpMethod,
		Source:     RpcSourceInternal,
	})

	res, err := m.callMethod(ctx, &RpcRequest{
		Version: "2.0",
		Method:  method,
		// we do not really care about the id here since we return the value right away
		ID:     _callMethodId,
		Params: json.RawMessage(jsonPayload),
	})

	// finalize the context used for the current call
	err = ctx.Finalize(err)

	if err != nil {
		return nil, err
	}
	if res != nil {
		return res, nil
	}
	return nil, nil
}

func (m *MethodHandler) processRpcMessages(
	source RpcSource,
	httpMethod RpcHttpMethod,
	r *http.Request,
	w http.ResponseWriter,
	ws *WSClient,
	data []byte,
) (resp []any, batch bool) {
	if len(data) == 0 {
		m.getLogger(nil).Info("method handler: empty body received")
		resp = []any{NewRpcErrorResponse(nil, ErrParse)}
		return
	}

	var (
		rpcRequests []json.RawMessage
	)

	dec := m.opts.JsonHandler.NewDecoder(bytes.NewReader(data))

	// we might either get an array of calls or a single call. let's inspect the first character in body to decide
	switch data[0] {
	case '[':
		// unmarshal array
		if err := dec.Decode(&rpcRequests); err != nil {
			m.getLogger(nil).Warn("method handler: parse error: ", "error", err)
			resp = []any{NewRpcErrorResponse(nil, ErrParse)}
			return
		}

		// fail on empty array
		if len(rpcRequests) == 0 {
			m.getLogger(nil).Warn("method handler: empty request array received")
			resp = []any{NewRpcErrorResponse(nil, ErrParse)}
			return
		}

		batch = true

	case '{':
		// unmarshal single item
		var rawRequest json.RawMessage
		if err := dec.Decode(&rawRequest); err != nil {
			m.getLogger(nil).Warn("method handler: parse error: ", "error", err)
			resp = []any{NewRpcErrorResponse(nil, ErrParse)}
			return
		}
		rpcRequests = []json.RawMessage{rawRequest}

	default:
		// fail on anything except arrays and objects
		m.getLogger(nil).Warn("method handler: invalid payload received; could not find neither an array nor an object")
		resp = []any{NewRpcErrorResponse(nil, ErrParse)}
		return
	}

	for _, _rpcRequest := range rpcRequests {
		// try to unmarshal the request message into an
		// rpc request format
		rpcRequest := &RpcRequest{}
		if err := m.opts.JsonHandler.Unmarshal(_rpcRequest, rpcRequest); err != nil {
			m.getLogger(nil).Warn("method handler: parse error: ", "error", err)
			resp = append(resp, NewRpcErrorResponse(nil, ErrParse))
			continue
		}
		if rpcResponse := m.processRpcMessage(source, httpMethod, r, w, ws, rpcRequest); rpcResponse != nil {
			// ares is nil if we don't have to add a response (notifications)
			resp = append(resp, rpcResponse)
		}
	}

	return
}

func (m *MethodHandler) processRpcMessage(
	source RpcSource,
	httpMethod RpcHttpMethod,
	r *http.Request,
	w http.ResponseWriter,
	ws *WSClient,
	rpcRequest *RpcRequest,
) any {
	// create bounded context and store request details
	ctx := NewContext(r.Context(), m.factory, m)
	ctx.StoreValue(TypeHttpRequest, &HttpRequest{
		Request: r,
	})
	ctx.StoreValue(TypeHttpResponseWriter, &HttpResponseWriter{
		ResponseWriter: w,
	})
	if ws != nil {
		ctx.StoreValue(TypeWSClient, ws)
	}

	ctx.StoreValue(TypeRpcMeta, &RpcMeta{
		Method:     rpcRequest.Method,
		HttpMethod: httpMethod,
		Source:     source,
	})

	// do the actual api call
	res, err := m.callMethod(ctx, rpcRequest)

	// finalize our context
	err = ctx.Finalize(err)

	// error response
	if err != nil {
		if err, ok := err.(*Error); ok {
			return NewRpcErrorResponse(rpcRequest.ID, err)
		}

		return NewRpcErrorResponse(rpcRequest.ID, ErrInternal.CloneWithData(&ErrorData{
			Debug: m.errorEncoder.Encode(err.Error()),
		}))
	}

	if rpcRequest.ID == nil {
		// jsonrpc 2.0 notification
		return nil
	}

	return NewRpcResultResponse(rpcRequest.ID, res)

}

func (m *MethodHandler) callMethod(ctx *Context, rpcRequest *RpcRequest) (any, error) {

	// retrieve rpc handler
	handler, ok := m.endpoints[rpcRequest.Method]
	if !ok {
		m.getLogger(ctx).Warn("method handler: endpoint not found", "method", rpcRequest.Method)
		return nil, ErrMethodNotFound
	}

	var (
		rv         = handler.handlerFunc
		rt         = rv.Type()
		args       = make([]reflect.Value, rt.NumIn())
		paramShift = 0
	)

	if !handler.methodContext.IsNil() {
		// we have a methodContext we need to pass as hidden first argument
		args[0] = handler.methodContext
		paramShift = 1
	}

	// walk through arguments and assign them
	for i := paramShift; i < rt.NumIn(); i++ {
		// params
		if i == handler.paramsPos {
			params := reflect.New(handler.paramsType)

			// in case anything panics inside the
			// params validation or unmarshal,
			// let's capture the error here
			err := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						err = getRecoverError(r)
					}
				}()
				err = rpcRequest.UnmarshalAndValidate(
					ctx,
					m.errorEncoder,
					params.Interface(),
				)
				return
			}()

			if err != nil {
				m.getLogger(ctx).Info(
					"method handler: validation error",
					"error", err,
					"rpcRequest", m.opts.RpcRequestLogInfoInterceptor(rpcRequest.getLogInfo(m.errorEncoder)),
				)
				return nil, err
			}
			args[i] = params
			continue
		}

		rti := rt.In(i)

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
			m.getLogger(ctx).Warn(fmt.Sprintf("method handler: provider for type '%s' failed", rti.String()),
				"error", err,
				"rpcRequest", m.opts.RpcRequestLogInfoInterceptor(rpcRequest.getLogInfo(m.errorEncoder)))
			return nil, err
		}

		args[i] = reflect.ValueOf(v)
	}

	var err error

	// call handler and capture panics
	handlerResult := func() []reflect.Value {
		defer func() {
			// catch any errors
			if r := recover(); r != nil {
				recoverErr := getRecoverError(r)

				// fwd information to the outside world
				if _, ok := recoverErr.(*Error); !ok {
					stack := string(debug.Stack())

					// panic, thrown unintentionally (cannot cast to an explicit jonson.Error)
					err = &PanicError{
						Err:    recoverErr,
						ID:     rpcRequest.ID,
						Method: rpcRequest.Method,
						Stack:  stack,
					}

					// let's log the unintended panic
					m.getLogger(ctx).Error("method handler: panic",
						"error", recoverErr,
						"stack", stack,
						"rpcRequest", m.opts.RpcRequestLogInfoInterceptor(rpcRequest.getLogInfo(m.errorEncoder)),
					)
				} else {
					// the function threw an error we must handle
					// and return to the outside world;
					// this error was most likely thrown intentionally since it's following
					// the jonson conventions
					err = recoverErr

					// no need to log, the developer caused the panic intentionally
				}
			}
		}()
		return handler.handlerFunc.Call(args)
	}()
	if err != nil {
		// we had a panic -> recover already logs the panic
		return nil, err
	}

	// error is either on position 1 (data, err) or position 0 (err)
	errIndex := len(handlerResult) - 1

	var res any
	// our method has (any, error) - use it
	if len(handlerResult) == 2 {
		res = handlerResult[0].Interface()
	}

	if handlerResult[errIndex].Interface() != nil {
		err = handlerResult[errIndex].Interface().(error)
	}

	if err != nil {
		return nil, err
	}
	if res != nil {
		return res, nil
	}

	return nil, nil
}

func getRecoverError(e any) error {
	err, ok := e.(error)
	if ok {
		return err
	}
	s, ok := e.(string)
	if ok {
		return errors.New(s)
	}
	return fmt.Errorf("%v", e)
}

func isTypeSupported(list []reflect.Type, rt reflect.Type) bool {
	for i := range list {
		if list[i] == rt {
			return true
		}
	}
	return false
}
