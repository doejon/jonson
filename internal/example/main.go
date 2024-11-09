package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/doejon/jonson"
	"github.com/doejon/jonson/internal/example/systems/account"
)

func main() {

	// let's spin up all necessary providers
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	providers := jonson.NewFactory(logger)
	providers.RegisterProvider(account.NewAuthenticationProvider())

	// let's declare the methods to be handled
	methods := jonson.NewMethodHandler(providers, jonson.NewAESSecret("962C27B021AD53CC1110E81E6F6C09D7A14F7911C508A43A"), &jonson.MethodHandlerOptions{
		MissingValidationLevel: jonson.MissingValidationLevelError,
	})
	accountSystem := account.NewAccount()
	methods.RegisterSystem(accountSystem)

	// let's declare _how_ we want to handle our calls
	rpc := jonson.NewHttpRpcHandler(methods, "/rpc")
	httpMethod := jonson.NewHttpMethodHandler(methods)

	// start the server
	server := jonson.NewServer(rpc, httpMethod)

	// create a new graceful shutdown provider
	graceful := jonson.NewGracefulProvider().WithDefaultHttpServer(server, ":8080").WithTimeout(time.Second * 5).WithLogger(logger)
	// make the graceful provider available to consuming endpoints
	providers.RegisterProvider(graceful)

	graceful.ListenAndServe()
}
