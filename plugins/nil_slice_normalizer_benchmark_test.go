package plugins

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/doejon/jonson"
	"github.com/doejon/jonson/benchmark"
)

func BenchmarkMutateFunction(b *testing.B) {
	plugin := &NilSliceNormalizer{LogLevel: LogLevelNone}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	for i := 0; i < b.N; i++ {
		payload := benchmark.NewPayload()
		plugin.Mutate(payload, logger)
	}
}

func BenchmarkEndToEnd_WithAndWithoutPlugin(b *testing.B) {
	scenarios := []struct {
		name   string
		plugin jonson.ResponseMutator
	}{
		{
			name:   "WithoutPlugin",
			plugin: nil,
		},
		{
			name:   "WithPlugin",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelNone},
		},
	}

	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			factory := jonson.NewFactory()
			secret := jonson.NewDebugSecret()
			testSystem := benchmark.NewBenchmarkSystem()

			var mutators []jonson.ResponseMutator
			if s.plugin != nil {
				mutators = append(mutators, s.plugin)
			}

			methodHandler := jonson.NewMethodHandler(factory, secret, &jonson.MethodHandlerOptions{
				ResponseMutators: mutators,
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
