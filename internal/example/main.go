package main

import (
	"log"

	"github.com/doejon/jonson"
	"github.com/doejon/jonson/internal/example/systems/account"
)

func main() {

	// let's spin up all necessary providers
	providers := jonson.NewFactory()
	providers.RegisterProvider(account.NewAuthenticationProvider())

	// let's declare the methods to be handled
	methods := jonson.NewMethodHandler(providers, jonson.NewAESSecret("962C27B021AD53CC1110E81E6F6C09D7A14F7911C508A43A"), nil)
	accountSystem := account.NewAccount()
	methods.RegisterSystem(accountSystem)

	// let's declare _how_ we want to handle our calls
	rpc := jonson.NewHttpRpcHandler(methods, "/rpc")
	httpMethod := jonson.NewHttpMethodHandler(methods)

	// start the server
	server := jonson.NewServer(rpc, httpMethod)

	log.Printf("listening on port 8080")
	server.ListenAndServe(":8080")
}
