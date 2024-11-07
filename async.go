package jonson

import "sync"

type Async struct {
	lock sync.Mutex
}

// RequireAsync makes sure that we can require something in an async
func RequireAsync(ctx *Context) {

}
