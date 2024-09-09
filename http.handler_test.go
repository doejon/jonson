package jonson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func parseHttpResponse(wtr *httptest.ResponseRecorder, out any) (*Error, error) {
	content, _ := io.ReadAll(wtr.Body)
	if wtr.Code > 400 {
		// we need to unmarshal error
		e := &Error{}
		err := json.Unmarshal(content, e)
		if err != nil {
			return nil, err
		}
		return e, nil
	}
	// we need to unmarshal result
	if out != nil {
		err := json.Unmarshal(content, out)
		if err != nil {
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
		Error  *Error `json:"error"`
		Result *json.RawMessage
	}

	dec := json.NewDecoder(wtr.Body)
	rpcResp := &rpcResponse{}
	if err := dec.Decode(rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil && rpcResp.Result != nil {
		return nil, fmt.Errorf("rpc response either consists of an error _or_ a result, never both")
	}

	if rpcResp.Error != nil {
		return rpcResp.Error, nil
	}
	if rpcResp.Result == nil {
		return nil, nil
	}

	// let's unmarshal expected response
	err := json.Unmarshal(*rpcResp.Result, out)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func newHttpRpcRequest(rpcMethod string, data any) *http.Request {

	req := RpcRequest{
		Version: "2.0",
		ID:      []byte("1"),
		Method:  rpcMethod,
		Params:  nil,
	}
	if data != nil {
		p, err := json.Marshal(data)
		if err != nil {
			panic(err)
		}
		req.Params = p
	}

	b := bytes.NewBuffer([]byte{})
	enc := json.NewEncoder(b)
	enc.Encode(req)

	httpReq, _ := http.NewRequest("POST", "/rpc", b)

	return httpReq
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
		if wtr.Code != http.StatusUnauthorized {
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

	t.Run("expect error since endpoint does not exist", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req := newHttpRpcRequest("test-system/unknown.v1", nil)

		httpRpcHandler.Handle(wtr, req)

		rpcErr, err := parseHttpRpcResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if rpcErr.Code != ErrMethodNotFound.Code {
			t.Fatalf("expected error code to equal method not found, got: %d", rpcErr.Code)
		}
	})

	t.Run("check access to default providers", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req := newHttpRpcRequest("test-system/check-requirables.v1", nil)

		httpRpcHandler.Handle(wtr, req)
		resErr, err := parseHttpResponse(wtr, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resErr != nil {
			t.Fatalf("expected result to have no error, got: %v", resErr)
		}
	})

	t.Run("expect current-time to return current time", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req := newHttpRpcRequest("test-system/current-time.v1", nil)

		httpRpcHandler.Handle(wtr, req)

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
		wtr := httptest.NewRecorder()
		testProvider.setLoggedIn(true)

		req := newHttpRpcRequest("test-system/me.v1", nil)

		httpRpcHandler.Handle(wtr, req)
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
		wtr := httptest.NewRecorder()
		testProvider.setLoggedIn(false)

		req := newHttpRpcRequest("test-system/me.v1", nil)

		httpRpcHandler.Handle(wtr, req)
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
