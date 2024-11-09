package jonsontest

import (
	"testing"
	"time"

	"github.com/doejon/jonson"
)

func TestGraceful(t *testing.T) {

	mock := NewAuthClientMock()
	fac := jonson.NewFactory()
	fac.RegisterProvider(jonson.NewAuthProvider(mock))
	gracefulProvider := NewGracefulProvider()
	fac.RegisterProvider(gracefulProvider)

	mtd := jonson.NewMethodHandler(fac, jonson.NewDebugSecret(), nil)
	mtd.RegisterSystem(&System{})

	accSuperUser := mock.NewAccount("e6dd1e60-8969-4f08-a854-80a29b69d7f3").Authorized()

	t.Run("accSuperUser can access set and get", func(t *testing.T) {
		active := make(chan struct{})
		done := make(chan struct{})
		go func() {
			NewContextBoundary(t, fac, mtd, accSuperUser.Provide).MustRun(func(ctx *jonson.Context) error {
				go func() {
					// give the function a  bit of time to start working on a process
					time.Sleep(time.Millisecond * 300)
					close(active)
				}()
				out := ProcessV1(ctx)
				return out
			})
			close(done)
		}()
		<-active
		gracefulProvider.Shutdown()
		<-done
	})

	t.Run("accSuperUser can access set and get using ProcessV2", func(t *testing.T) {
		gracefulProvider.Restart()

		active := make(chan struct{})
		done := make(chan struct{})
		go func() {
			NewContextBoundary(t, fac, mtd, accSuperUser.Provide).MustRun(func(ctx *jonson.Context) error {
				go func() {
					// give the function a  bit of time to start working on a process
					time.Sleep(time.Millisecond * 300)
					close(active)
				}()
				out := ProcessV2(ctx)
				return out
			})
			close(done)
		}()
		<-active
		gracefulProvider.Shutdown()
		<-done
	})

}
