package jonson

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"syscall"
	"testing"
	"time"
)

func TestGraceful(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// killServer simulates a server shutdown
	killServer := func(provider *GracefulProvider) {
		keepLooping := true
		for keepLooping {
			select {
			case provider.quitChan <- syscall.SIGINT:
				keepLooping = false
			default:
				continue
			}
		}
	}

	callProcessEndpoint := func(port string) {
		for i := 0; i < 10; i++ {
			clnt := &http.Client{}
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost%s/process", port), nil)
			res, err := clnt.Do(req)
			if err != nil {
				log.Printf("please accept incoming requests in case you're asked to do so: %s", err)
				time.Sleep(time.Second * 2)
				continue
			}
			if res.StatusCode != 200 {
				log.Printf("status code != 200, please accept incoming requests in case you're asked to do so")
				time.Sleep(time.Second * 2)
				continue
			}
			// successfully reached endpoint, stop processing
			break
		}
	}

	getPort := func() string {
		port, err := GetFreePort()
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf(":%d", port)
	}

	t.Run("graceful shutdown works without nothing blocking", func(t *testing.T) {
		port := getPort()
		srv := NewServer()
		prov := NewGracefulProvider().WithDefaultHttpServer(srv, port).WithLogger(logger)

		cnt := 0
		var err error
		down := make(chan struct{})
		go func() {
			err = prov.ListenAndServe()
			cnt++
			close(down)
		}()
		// kill the server
		killServer(prov)

		<-down
		if err != nil {
			t.Fatal("failed to start graceful shutdown")
		}
		if cnt == 0 {
			t.Fatal("expected graceful shutdown to be reached")
		}
	})

	t.Run("graceful shutdown does not work since a process is still operating", func(t *testing.T) {
		fac := NewFactory()
		methods := NewMethodHandler(fac, nil, nil)
		regexpHandler := NewHttpRegexpHandler(fac, methods)

		startedProcessing := make(chan struct{})
		regexpHandler.RegisterRegexp(regexp.MustCompile("/process"), func(ctx *Context) {
			close(startedProcessing)
			for {
				time.Sleep(time.Second * 1)
			}
		})

		srv := NewServer(regexpHandler)
		port := getPort()
		prov := NewGracefulProvider().WithDefaultHttpServer(srv, port).WithLogger(logger).WithTimeout(time.Second * 2)

		cnt := 0
		var err error

		up := make(chan struct{})
		down := make(chan struct{})
		go func() {
			close(up)
			err = prov.ListenAndServe()
			cnt++
			close(down)
		}()

		// send a request to process once the
		// server is up to block the server
		// from gracefully shutting down

		go func() {
			<-up
			callProcessEndpoint(port)
		}()

		// wait for the server to have started and for a request being sent
		<-startedProcessing
		log.Printf("started processing, trying to kill server")

		// kill the server
		killServer(prov)

		<-down
		if err == nil {
			t.Fatal("expected graceful shutdown to fail")
		}
		if cnt == 0 {
			t.Fatal("expected graceful shutdown to be reached")
		}
	})

	t.Run("graceful shutdown is being handled by checking for shutdown", func(t *testing.T) {
		fac := NewFactory()
		methods := NewMethodHandler(fac, nil, nil)
		regexpHandler := NewHttpRegexpHandler(fac, methods)

		startedProcessing := make(chan struct{})
		regexpHandler.RegisterRegexp(regexp.MustCompile("/process"), func(ctx *Context) {
			graceful := RequireGraceful(ctx)
			close(startedProcessing)
			for graceful.IsUp() {
				time.Sleep(time.Second * 1)
			}
		})

		srv := NewServer(regexpHandler)
		port := getPort()
		prov := NewGracefulProvider().WithDefaultHttpServer(srv, port).WithLogger(logger).WithTimeout(time.Second * 2)
		fac.RegisterProvider(prov)
		cnt := 0
		var err error

		up := make(chan struct{})
		down := make(chan struct{})
		go func() {
			close(up)
			err = prov.ListenAndServe()
			cnt++
			close(down)
		}()

		// send a request to process once the
		// server is up to block the server
		// from gracefully shutting down

		go func() {
			<-up
			callProcessEndpoint(port)
		}()

		// wait for the server to have started and for a request being sent
		<-startedProcessing
		log.Printf("started processing, trying to kill server")

		// kill the server
		killServer(prov)

		<-down
		if err != nil {
			t.Fatal("expected graceful shutdown to succeed")
		}
		if cnt == 0 {
			t.Fatal("expected graceful shutdown to be reached")
		}
	})

	t.Run("graceful shutdown is being handled by checking for shutdown by using convenience method IsDown", func(t *testing.T) {
		fac := NewFactory()
		methods := NewMethodHandler(fac, nil, nil)
		regexpHandler := NewHttpRegexpHandler(fac, methods)

		startedProcessing := make(chan struct{})
		regexpHandler.RegisterRegexp(regexp.MustCompile("/process"), func(ctx *Context) {
			graceful := RequireGraceful(ctx)
			close(startedProcessing)
			for !graceful.IsDown() {
				time.Sleep(time.Second * 1)
			}
		})

		srv := NewServer(regexpHandler)
		port := getPort()
		prov := NewGracefulProvider().WithDefaultHttpServer(srv, port).WithLogger(logger).WithTimeout(time.Second * 2)
		fac.RegisterProvider(prov)
		cnt := 0
		var err error

		up := make(chan struct{})
		down := make(chan struct{})
		go func() {
			close(up)
			err = prov.ListenAndServe()
			cnt++
			close(down)
		}()

		// send a request to process once the
		// server is up to block the server
		// from gracefully shutting down

		go func() {
			<-up
			callProcessEndpoint(port)
		}()

		// wait for the server to have started and for a request being sent
		<-startedProcessing
		log.Printf("started processing, trying to kill server")

		// kill the server
		killServer(prov)

		<-down
		if err != nil {
			t.Fatal("expected graceful shutdown to succeed")
		}
		if cnt == 0 {
			t.Fatal("expected graceful shutdown to be reached")
		}
	})
}
