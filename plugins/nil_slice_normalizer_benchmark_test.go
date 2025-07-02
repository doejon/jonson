package plugins

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/doejon/jonson"
	"github.com/doejon/jonson/benchmark"
)

func BenchmarkMutateFunction(b *testing.B) {
	plugin := NewNilSliceNormalizer()

	for i := 0; i < b.N; i++ {
		payload := benchmark.NewPayload()
		plugin.MutateEncode(payload)
	}
}

func BenchmarkEndToEnd_WithAndWithoutPlugin(b *testing.B) {
	scenarios := []struct {
		name        string
		jsonHandler func() jonson.JsonHandler
	}{
		{
			name:        "WithoutPlugin",
			jsonHandler: func() jonson.JsonHandler { return jonson.NewDefaultJsonHandler() },
		},
		{
			name: "WithPlugin",
			jsonHandler: func() jonson.JsonHandler {
				return NewJsonMutatorHandler(jonson.NewDefaultJsonHandler(), NewNilSliceNormalizer())
			},
		},
	}

	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			factory := jonson.NewFactory()
			secret := jonson.NewDebugSecret()
			testSystem := benchmark.NewBenchmarkSystem()

			methodHandler := jonson.NewMethodHandler(factory, secret, &jonson.MethodHandlerOptions{
				JsonHandler: s.jsonHandler(),
			})
			methodHandler.RegisterSystem(testSystem)

			jonsonServer := jonson.NewServer(jonson.NewHttpRpcHandler(methodHandler, "/rpc"))
			server := httptest.NewServer(jonsonServer)
			defer server.Close()

			client := server.Client()
			rpcReq, _ := json.Marshal(jonson.RpcRequest{
				Version: "2.0",
				ID:      []byte(`"1"`),
				Method:  "benchmark-system/complex-response.v1",
			})

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				resp, err := client.Post(server.URL+"/rpc", "application/json", bytes.NewReader(rpcReq))
				if err != nil {
					b.Fatalf("Request failed: %v", err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		})
	}
}
