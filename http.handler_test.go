package jonson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

func parseHttpResponse(wtr *httptest.ResponseRecorder, out any) (*Error, error) {
	content, _ := io.ReadAll(wtr.Body)

	// For HttpMethodHandler, the status code reflects the error type.
	if wtr.Code >= 400 {
		e := &Error{}
		if err := json.Unmarshal(content, e); err != nil {
			return nil, fmt.Errorf("failed to unmarshal error response (status %d): %w. Body: %s", wtr.Code, err, string(content))
		}
		return e, nil
	}

	// we need to unmarshal result
	if out != nil {
		if err := json.Unmarshal(content, out); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func parseHttpRpcResponse(wtr *httptest.ResponseRecorder, out any) (*Error, error) {
	if wtr.Code != 200 {
		return nil, fmt.Errorf("expected status code 200, got: %d", wtr.Code)
	}

	type rpcResponse struct {
		RpcResponseHeader
		Error  *Error           `json:"error"`
		Result *json.RawMessage `json:"result"`
	}

	bodyBytes, err := io.ReadAll(wtr.Body)
	if err != nil {
		return nil, err
	}

	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("empty response body")
	}

	var singleResp *rpcResponse

	// The HttpRpcHandler may wrap a single response in an array.
	if bodyBytes[0] == '[' {
		var batchResp []*rpcResponse
		if err := json.Unmarshal(bodyBytes, &batchResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal batch response: %w. Body: %s", err, string(bodyBytes))
		}
		if len(batchResp) == 0 {
			return nil, fmt.Errorf("received empty array response")
		}
		singleResp = batchResp[0]
	} else {
		singleResp = &rpcResponse{}
		if err := json.Unmarshal(bodyBytes, singleResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal single response: %w. Body: %s", err, string(bodyBytes))
		}
	}

	if singleResp.Error != nil && singleResp.Result != nil {
		return nil, fmt.Errorf("rpc response either consists of an error _or_ a result, never both")
	}

	if singleResp.Error != nil {
		return singleResp.Error, nil
	}
	if singleResp.Result == nil {
		return nil, nil
	}

	if err := json.Unmarshal(*singleResp.Result, out); err != nil {
		return nil, err
	}

	return nil, nil
}

func TestHttpHandler(t *testing.T) {
	tm := time.Now()
	testProvider := NewTestProvider()

	factory := NewFactory()
	factory.RegisterProvider(testProvider)
	factory.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))

	testSystem := NewTestSystem()

	secret := NewDebugSecret()
	methodHandler := NewMethodHandler(factory, secret, nil)
	methodHandler.RegisterSystem(testSystem)

	httpHandler := NewHttpMethodHandler(methodHandler)

	t.Run("expect error since endpoint does not exist", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test-system/unknown.v1", nil)

		found := httpHandler.Handle(wtr, req)
		if found == true {
			t.Fatal("method does not exist, handler should not have found method")
		}
	})

	t.Run("check access to default providers", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test-system/check-requirables.v1", nil)

		httpHandler.Handle(wtr, req)
		resErr, err := parseHttpResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resErr != nil {
			t.Fatalf("expected result to have no error, got: %v", resErr.Data)
		}
	})

	t.Run("expect current-time to return current time", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test-system/current-time.v1", nil)

		httpHandler.Handle(wtr, req)
		result := &CurrentTimeV1Result{}
		_, err := parseHttpResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Ts != tm.Unix() {
			t.Fatalf("expected time to be equal, got: %d %d", result.Ts, tm.Unix())
		}
	})

	t.Run("expect current-time not to be returned due to wrong http request method", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/test-system/current-time.v1", nil)

		httpHandler.Handle(wtr, req)
		result := &CurrentTimeV1Result{}
		errResult, err := parseHttpResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if wtr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected http status method not allowed code, got: %d", wtr.Code)
		}
		if errResult.Code != ErrServerMethodNotAllowed.Code {
			t.Fatalf("server method nod allowed error expected, got: %d", errResult.Code)
		}
	})

	t.Run("calls method me.v1", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		testProvider.setLoggedIn(true)

		req, _ := http.NewRequest("GET", "/test-system/me.v1", nil)

		httpHandler.Handle(wtr, req)
		result := &MeV1Result{}
		_, err := parseHttpResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Uuid != testAccountUuid {
			t.Fatalf("expected uuids to match, got: %s | %s", result.Uuid, testAccountUuid)
		}
		if result.HttpMethod != RpcHttpMethodGet {
			t.Fatalf("expected http method GET, got %s", result.HttpMethod)
		}
	})

	t.Run("calls method me.v1 and fails due to missing permission", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		testProvider.setLoggedIn(false)

		req, _ := http.NewRequest("GET", "/test-system/me.v1", nil)

		httpHandler.Handle(wtr, req)
		errResult, err := parseHttpResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if errResult == nil {
			t.Fatal("expected http call to return an error due to missing permission")
		}
		if errResult.Code != ErrUnauthorized.Code {
			t.Fatalf("result code to match unauthorized, got: %d", errResult.Code)
		}
		if wtr.Code != http.StatusForbidden {
			t.Fatalf("expected http result code to equal status forbidden, got: %d", wtr.Code)
		}
	})

	t.Run("calls method get-profile.v1", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		testProvider.setLoggedIn(true)

		params, _ := json.Marshal(GetProfileV1Params{
			Uuid: testAccountUuid,
		})

		req, _ := http.NewRequest("POST", "/test-system/get-profile.v1", bytes.NewReader(params))

		httpHandler.Handle(wtr, req)
		result := &GetProfileV1Result{}
		_, err := parseHttpResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Name != "Silvio" {
			t.Fatalf("expected returned profile name to equal Silvio, got: %s", result.Name)
		}
		if result.HttpMethod != RpcHttpMethodPost {
			t.Fatalf("expected http method to equal post, got: %s", result.HttpMethod)
		}
	})

	t.Run("calls method get-profile.v1 with invalid http method", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		testProvider.setLoggedIn(true)

		params, _ := json.Marshal(GetProfileV1Params{
			Uuid: testAccountUuid,
		})

		req, _ := http.NewRequest("GET", "/test-system/get-profile.v1", bytes.NewReader(params))

		httpHandler.Handle(wtr, req)
		result := &GetProfileV1Result{}
		rpcErr, err := parseHttpResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if rpcErr.Code != ErrServerMethodNotAllowed.Code {
			t.Fatalf("expected method not allowed rpc response, got: %d", rpcErr.Code)
		}
		if wtr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected method not allowed http response, got: %d", wtr.Code)
		}
	})
}

func TestHttpRpcHandler(t *testing.T) {
	tm := time.Now()
	testProvider := NewTestProvider()

	factory := NewFactory()
	factory.RegisterProvider(testProvider)
	factory.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))

	testSystem := NewTestSystem()

	secret := NewDebugSecret()
	methodHandler := NewMethodHandler(factory, secret, nil)
	methodHandler.RegisterSystem(testSystem)

	httpRpcHandler := NewHttpRpcHandler(methodHandler, "/rpc")

	// NewServer takes one or more jonson.Handler and returns an http.Handler
	jonsonServer := NewServer(httpRpcHandler)

	server := httptest.NewServer(jonsonServer)
	defer server.Close()

	// Helper to make real HTTP requests to the test server
	makeRequest := func(rpcMethod string, data any) *httptest.ResponseRecorder {
		reqPayload := RpcRequest{
			Version: "2.0",
			ID:      []byte(`"1"`),
			Method:  rpcMethod,
		}
		if data != nil {
			p, err := json.Marshal(data)
			if err != nil {
				t.Fatalf("Failed to marshal request params: %v", err)
			}
			reqPayload.Params = p
		}

		body, err := json.Marshal(reqPayload)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}

		// Make a real POST request to the running test server
		resp, err := http.Post(server.URL+"/rpc", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("HTTP POST request failed: %v", err)
		}

		// Create a ResponseRecorder to return the response
		wtr := httptest.NewRecorder()
		bodyBytes, _ := io.ReadAll(resp.Body)
		wtr.Body.Write(bodyBytes)
		wtr.Code = resp.StatusCode
		_ = resp.Body.Close()

		return wtr
	}

	t.Run("expect error since endpoint does not exist", func(t *testing.T) {
		wtr := makeRequest("test-system/unknown.v1", nil)
		rpcErr, err := parseHttpRpcResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if rpcErr.Code != ErrMethodNotFound.Code {
			t.Fatalf("expected error code to equal method not found, got: %d", rpcErr.Code)
		}
	})

	t.Run("check access to default providers", func(t *testing.T) {
		wtr := makeRequest("test-system/check-requirables.v1", nil)
		resErr, err := parseHttpRpcResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resErr != nil {
			t.Fatalf("expected result to have no error, got: %v", resErr)
		}
	})

	t.Run("expect current-time to return current time", func(t *testing.T) {
		wtr := makeRequest("test-system/current-time.v1", nil)
		result := &CurrentTimeV1Result{}
		_, err := parseHttpRpcResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Ts != tm.Unix() {
			t.Fatalf("expected time to be equal, got: %d %d", result.Ts, tm.Unix())
		}
	})

	t.Run("calls method me.v1", func(t *testing.T) {
		testProvider.setLoggedIn(true)
		wtr := makeRequest("test-system/me.v1", nil)
		result := &MeV1Result{}
		_, err := parseHttpRpcResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Uuid != testAccountUuid {
			t.Fatalf("expected uuids to match, got: %s | %s", result.Uuid, testAccountUuid)
		}
	})

	t.Run("calls method me.v1 and fails due to missing permission", func(t *testing.T) {
		testProvider.setLoggedIn(false)
		wtr := makeRequest("test-system/me.v1", nil)
		errResult, err := parseHttpRpcResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if errResult == nil {
			t.Fatal("expected http call to return an error due to missing permission")
		}
		if errResult.Code != ErrUnauthorized.Code {
			t.Fatalf("result code to match unauthorized, got: %d", errResult.Code)
		}
	})
}

func TestHttpHandlerRegexpAuth(t *testing.T) {
	tm := time.Now()

	factory := NewFactory()
	factory.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))
	factory.RegisterProvider(NewTestProvider())

	tac := &testAuthClient{}

	factory.RegisterProvider(NewAuthProvider(tac))

	testSystem := NewTestSystem()

	secret := NewDebugSecret()
	methodHandler := NewMethodHandler(factory, secret, nil)
	methodHandler.RegisterSystem(testSystem)
	httpHandler := NewHttpMethodHandler(methodHandler)
	regexpHandler := NewHttpRegexpHandler(factory, methodHandler)

	regexpHandler.RegisterRegexp(regexp.MustCompile("/sys/get.v1"), func(ctx *Context, w http.ResponseWriter, r *http.Request, parts []string) {
		RequirePrivate(ctx)
		w.Write([]byte("OK"))
	})

	regexpHandler.RegisterRegexp(regexp.MustCompile("/sys/panic.v1"), func(ctx *Context, w http.ResponseWriter, r *http.Request, parts []string) {
		panic("internal panic")
	})

	regexpHandler.RegisterRegexp(regexp.MustCompile("/sys/rpcmeta.v1"), func(ctx *Context, w http.ResponseWriter, r *http.Request, parts []string) {
		meta := RequireRpcMeta(ctx)
		b, _ := json.Marshal(meta)
		w.Write(b)
	})

	server := NewServer(httpHandler, regexpHandler)

	t.Run("panic is caught and returned", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sys/panic.v1", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != 500 {
			t.Fatalf("expected http status code 500, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != `{"code":-32603,"message":"Internal error","data":{"debug":"internal panic"}}` {
			t.Fatalf("expected returned body to equal panic, got: %s", string(content))
		}
	})

	t.Run("fails with unauthorized call", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sys/get.v1", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != 403 {
			t.Fatalf("expected http status code 403, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != `{"code":-32001,"message":"Not authorized"}` {
			t.Fatalf("expected returned body to equal 'OK', got: %s", string(content))
		}
	})

	t.Run("succeeds with authorized call", func(t *testing.T) {
		tac.isAuthorized = true

		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sys/get.v1", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != 200 {
			t.Fatalf("expected http status code 200, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != `OK` {
			t.Fatalf("expected returned body to equal 'OK', got: %s", string(content))
		}
	})

	t.Run("can retrieve rpc meta", func(t *testing.T) {
		tac.isAuthorized = true

		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sys/rpcmeta.v1", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != 200 {
			t.Fatalf("expected http status code 200, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != `{"Method":"sys/rpcmeta.v1","HttpMethod":"GET","Source":"http"}` {
			t.Fatalf("expected returned body to equal rpc meta, got: %s", string(content))
		}
	})
}
