package jonson

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"
)

// Graceful allows us to check
// whether a server is still up
// or trying to gracefully shut down.
// Use Graceful in loops which might need to be interrupted
// once a server starts to shut down.
//
//	graceful := jonson.RequireGraceful(ctx)
//	for graceful.IsUp() {
//	  // ...
//	}
type Graceful interface {
	Shareable
	ShareableAcrossImpersonation

	IsUp() bool
	IsDown() bool
}

type graceful struct {
	g *GracefulProvider
	Shareable
	ShareableAcrossImpersonation
}

var TypeGraceful = reflect.TypeOf((*Graceful)(nil)).Elem()

// RequireGraceful requires the current instance of graceful and
// allows us to check for server shutdown attempts
func RequireGraceful(ctx *Context) Graceful {
	if v := ctx.Require(TypeGraceful); v != nil {
		return v.(Graceful)
	}
	return nil
}

// IsUp returns true as long as the server
// is actively serving requests.
// Once the server starts shutting down, IsUp will
// return false
func (g *graceful) IsUp() bool {
	select {
	case _, ok := <-g.g.checkStatusChan:
		return ok
	default:
		return true
	}
}

// IsUp returns false as long as the server
// is actively serving requests.
// Once the server starts shutting down, IsDown will
// return true
func (g *graceful) IsDown() bool {
	return !g.IsUp()
}

type GracefulProvider struct {
	httpServer *http.Server
	timeout    *time.Duration
	logger     *slog.Logger

	// checkStatusChan allows us to check for
	// the server being in shutdown mode by other goroutines
	checkStatusChan chan struct{}
	quitChan        chan (os.Signal)
}

// NewGracefulProvider returns a new Graceful provider
func NewGracefulProvider() *GracefulProvider {
	return &GracefulProvider{
		httpServer:      nil,
		logger:          slog.New(slog.NewJSONHandler(io.Discard, nil)),
		checkStatusChan: make(chan struct{}),
	}
}

func (g *GracefulProvider) NewGraceful(ctx *Context) Graceful {
	return &graceful{
		g: g,
	}
}

// WithHttpServer will replace the default http server
func (g *GracefulProvider) WithHttpServer(server *http.Server) *GracefulProvider {
	g.httpServer = server
	return g
}

// WithDefaultHttpServer instantiates the graceful provider with a default
// http server. In case no address is provided, a random address will be selected
// by asking the operating system for a free port.
func (g *GracefulProvider) WithDefaultHttpServer(s *Server, addr string) *GracefulProvider {
	g.httpServer = &http.Server{
		Addr:    addr,
		Handler: s,
	}
	return g
}

// WithTimeout allows you to specify a specific timeout to wait for a
// proper graceful shutdown of your server. In case timeout is reached,
// the server will leave the shutdown routine and exit.
func (g *GracefulProvider) WithTimeout(duration time.Duration) *GracefulProvider {
	g.timeout = &duration
	return g
}

// WithLogger allows you to set a logger to log startup and shutdown information
func (g *GracefulProvider) WithLogger(logger *slog.Logger) *GracefulProvider {
	g.logger = logger
	return g
}

// ListenAndServe listens and serve on given address.
// In case you did provide a server, address will be ignored (if present).
// In case no server was provided,
func (g *GracefulProvider) ListenAndServe() error {
	// start the server in a goroutine
	go func() {
		g.logger.Info(fmt.Sprintf("graceful.ListenAndServe: accepting incoming requests on: %s", g.httpServer.Addr))
		if err := g.httpServer.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				g.logger.Error("graceful.ListenAndServe: failed to listen and serve", "error", err)
				// we need to exit right away, no need to
				// keep the process up and running
				os.Exit(-1)
			}
		}
	}()

	// wait for sigterm
	g.quitChan = make(chan os.Signal, 1)
	signal.Notify(g.quitChan, syscall.SIGINT, syscall.SIGTERM)
	<-g.quitChan

	// shutdown with timeout?
	var ctx context.Context
	var cancelFunc context.CancelFunc = func() {}
	if g.timeout != nil {
		ctx, cancelFunc = context.WithTimeout(context.Background(), *g.timeout)
		g.logger.Info(fmt.Sprintf("graceful.ListenAndServe: gracefully shutting down with a timeout of %.2f seconds", g.timeout.Seconds()))
	} else {
		ctx = context.Background()
		g.logger.Info("graceful.ListenAndServe: gracefully shutting down with no timeout")
	}
	defer cancelFunc()

	// shutdown
	nw := time.Now()
	close(g.checkStatusChan)
	if err := g.httpServer.Shutdown(ctx); err != nil {
		g.logger.Info("graceful.ListenAndServe: failed to shutdown server", "error", err)
		return err
	}
	g.logger.Info(fmt.Sprintf("graceful.ListenAndServe: gracefully stopped server after %.4f seconds", time.Since(nw).Seconds()))
	return nil
}

// GetFreePort returns a free port by consulting the operating system
func GetFreePort() (int, error) {
	a, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", a)
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}
