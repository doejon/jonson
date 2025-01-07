package jonson

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLog(t *testing.T) {

	tm := time.Now()
	testProvider := NewTestProvider()

	buf := bytes.NewBuffer([]byte{})

	log := slog.New(slog.NewJSONHandler(buf, nil))

	factory := NewFactory(&FactoryOptions{
		Logger:        log,
		LoggerOptions: (&LoggerOptions{}).WithCallerFunction().WithCallerRpcMeta(),
	})
	factory.RegisterProvider(testProvider)
	factory.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))

	testSystem := NewTestSystem()

	secret := NewDebugSecret()
	methodHandler := NewMethodHandler(factory, secret, nil)
	methodHandler.RegisterSystem(testSystem)

	httpHandler := NewHttpMethodHandler(methodHandler)

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

		logs := strings.Split(buf.String(), "\n")
		// remove empty line
		logs = logs[:len(logs)-1]

		t.Logf("%v", logs)
		if len(logs) != 2 {
			t.Fatalf("expected two logs, got %d", len(logs))
		}

		type log struct {
			Time        string `json:"time"`
			CurrentTime int64  `json:"currentTime"`
			Level       string `json:"level"`
			Function    struct {
				Name   string `json:"name"`
				Struct string `json:"struct"`
			} `json:"function"`
			Endpoint *RpcMeta `json:"rpcMeta"`
		}

		parsedLogs := []*log{}
		for _, v := range logs {
			out := &log{}
			err := json.Unmarshal([]byte(v), out)
			if err != nil {
				t.Fatal(err)
			}
			parsedLogs = append(parsedLogs, out)
		}

		firstLog := parsedLogs[0]
		if firstLog.Endpoint.HttpMethod != "GET" {
			t.Fatal("expected http method to equal 'GET'", firstLog.Endpoint.HttpMethod)
		}
		if firstLog.Endpoint.Method != "test-system/current-time.v1" {
			t.Fatal("expected method to equal 'test-system/current-time.v1'", firstLog.Endpoint.Method)
		}
		if firstLog.Endpoint.Source != "http" {
			t.Fatal("expected source to equal 'http'", firstLog.Endpoint.Source)
		}
		if firstLog.Level != "INFO" {
			t.Fatal("expected level to equal 'INFO'", firstLog.Level)
		}
		if firstLog.Function.Name != "getCurrentTime" {
			t.Fatal("expected fun to equal 'getCurrentTime'", firstLog.Function.Name)
		}
		if firstLog.Function.Struct != "jonson.(*TestSystem).getCurrentTime" {
			t.Fatal("expected struct func to equal 'jonson.(*TestSystem).getCurrentTime'", firstLog.Function.Struct)
		}

		secondLog := parsedLogs[1]
		if secondLog.Endpoint.HttpMethod != "GET" {
			t.Fatal("expected http method to equal 'GET'", secondLog.Endpoint.HttpMethod)
		}
		if secondLog.Endpoint.Method != "test-system/current-time.v1" {
			t.Fatal("expected method to equal 'test-system/current-time.v1'", secondLog.Endpoint.Method)
		}
		if secondLog.Endpoint.Source != "http" {
			t.Fatal("expected source to equal 'http'", secondLog.Endpoint.Source)
		}
		if secondLog.Level != "INFO" {
			t.Fatal("expected level to equal 'INFO'", secondLog.Level)
		}
		if secondLog.Function.Name != "CurrentTimeV1" {
			t.Fatal("expected fun to equal 'CurrentTimeV1'", secondLog.Function.Name)
		}
		if secondLog.Function.Struct != "jonson.(*TestSystem).CurrentTimeV1" {
			t.Fatal("expected struct func to equal 'jonson.(*TestSystem).CurrentTimeV1'", secondLog.Function.Struct)
		}
	})

}
