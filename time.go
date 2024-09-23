package jonson

import (
	"reflect"
	"time"
)

var TypeTime = reflect.TypeOf((*Time)(nil)).Elem()

// RequireHttpResponseWriter returns the current http response writer
// which is used to handle the ongoing request's response.
func RequireTime(ctx *Context) Time {
	if v := ctx.Require(TypeTime); v != nil {
		return v.(Time)
	}
	return nil
}

// TimeProvider allows us to provide a time within our application.
// In order to allow for mocking times, we can use this
// pre-defined provider to allow for specifying a pre-defined time in our tests
// which will allow us to move forward/backwards (if needed)
type TimeProvider struct {
	inst func() Time
}

// NewTimeProvider returns a new TimeProvider
func NewTimeProvider(inst ...func() Time) *TimeProvider {
	out := &TimeProvider{
		// by default, we will provide real time
		inst: func() Time {
			return NewRealTime()
		},
	}
	// in case any time instances have been provided,
	// let's pick the last one and use it as
	// our time provider
	for _, v := range inst {
		out.inst = v
	}
	return out
}

func (t *TimeProvider) NewTime(ctx *Context) Time {
	return t.inst()
}

// Time is the interface that can be used within your application.
// You can mock this interface within your tests.
type Time interface {
	Shareable
	ShareableAcrossImpersonation
	Now() time.Time
	Sleep(time.Duration)
}

// RealTime implements time
type RealTime struct {
	Shareable
	ShareableAcrossImpersonation
}

// type safeguard
var _ Time = &RealTime{}

// Now returns current time as UTC
func (t *RealTime) Now() time.Time {
	return time.Now().UTC()
}

// Sleep for duration
func (t *RealTime) Sleep(dur time.Duration) {
	time.Sleep(dur)
}

// NewTime returns a time instance which provides us with
// real time information. You will probably use this
// time instance for your production build.
func NewRealTime() *RealTime {
	return &RealTime{}
}
