package jonson

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestServer(t *testing.T) {
	tm := time.Now()

	factory := NewFactory()
	factory.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))
	factory.RegisterProvider(NewTestProvider())

	testSystem := NewTestSystem()

	secret := NewDebugSecret()
	methodHandler := NewMethodHandler(factory, secret, nil)
	methodHandler.RegisterSystem(testSystem)
	httpHandler := NewHttpMethodHandler(methodHandler)
	regexpHandler := NewHttpRegexpHandler(factory, methodHandler)

	regexpHandler.RegisterRegexp(regexp.MustCompile("/health"), func(ctx *Context, w *HttpResponseWriter) {
		w.Write([]byte("OK"))
	})

	regexpHandler.RegisterRegexp(regexp.MustCompile("^/[a-zA-Z0-9]+-[a-zA-Z0-9]+$"), func(ctx *Context, w *HttpResponseWriter, match *HttpRegexpMatchedParts) {
		w.Write([]byte(strings.Join(match.Parts, ",")))
	})

	server := NewServer(httpHandler, regexpHandler)

	t.Run("handle method does serve registered rpc endpoints", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test-system/current-time.v1", nil)

		server.ServeHTTP(wtr, req)
		result := &CurrentTimeV1Result{}
		_, err := parseHttpResponse(wtr, result)
		if err != nil {
			t.Fatal(err)
		}
		if result.Ts != tm.Unix() {
			t.Fatalf("expected time to be equal, got: %d %d", result.Ts, tm.Unix())
		}

	})

	t.Run("handle method does serve registered regexp endpoints", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != 200 {
			t.Fatalf("expected http status code 200, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != "OK" {
			t.Fatalf("expected returned body to equal 'OK', got: %s", string(content))
		}
	})

	t.Run("handle method does serve registered regexp endpoint with pattern matching", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/a-a", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != 200 {
			t.Fatalf("expected http status code 200, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != "/a-a" {
			t.Fatalf("expected returned body to equal 'OK', got: %s", string(content))
		}
	})

	t.Run("unknown endpoints return status not found", func(t *testing.T) {
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/unknown", nil)

		server.ServeHTTP(wtr, req)
		if wtr.Code != http.StatusNotFound {
			t.Fatalf("expected http status code 200, got: %d", wtr.Code)
		}

		content, _ := io.ReadAll(wtr.Body)
		if string(content) != "" {
			t.Fatalf("expected returned body to equal <empty body>, got: %s", string(content))
		}
	})
}
