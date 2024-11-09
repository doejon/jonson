package jonsontest

import "github.com/doejon/jonson"

// GracefulProvider can be used for tests
// which rely on jonson.Graceful to be provided.
// You can mimick a server shutdown by using jonsontest.GracefulProvider
type GracefulProvider struct {
	isActive chan struct{}
}

func NewGracefulProvider() *GracefulProvider {
	return &GracefulProvider{
		isActive: make(chan struct{}),
	}
}
func (g *GracefulProvider) NewGraceful(ctx *jonson.Context) jonson.Graceful {
	return &graceful{
		g: g,
	}
}

// Shutdown mimicks a server shutdown
func (g *GracefulProvider) Shutdown() {
	close(g.isActive)
}

// Start mimicks a server restart
func (g *GracefulProvider) Restart() {
	g.isActive = make(chan struct{})
}

// graceful implements a version for tests
// of graceful
type graceful struct {
	jonson.Shareable
	jonson.ShareableAcrossImpersonation
	g *GracefulProvider
}

func (g *graceful) IsUp() bool {
	select {
	case _, ok := <-g.g.isActive:
		return ok
	default:
		return true
	}
}

func (g *graceful) IsDown() bool {
	return !g.IsUp()
}
