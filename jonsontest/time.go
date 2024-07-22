package jonsontest

import (
	"time"

	"github.com/doejon/jonson"
)

// MockTime is a test time that can be implemented by multiple test mocks
type MockTime interface {
	jonson.Time
	jonson.Shareable
	SetNow(tm time.Time)
	AddDate(years int, months int, days int) MockTime
	Add(dur time.Duration) MockTime
}

type FrozenTime struct {
	jonson.Shareable
	instant time.Time
	sleep   func(time.Duration)
}

func (f *FrozenTime) SetNow(tm time.Time) {
	f.instant = tm
}

func (f *FrozenTime) AddDate(years int, months int, days int) MockTime {
	f.instant = f.instant.AddDate(years, months, days)
	return f
}

func (f *FrozenTime) SetTime(hour int, min int, sec int) {
	t := f.instant
	f.instant = time.Date(t.Year(), t.Month(), t.Day(), hour, min, sec, 0, t.Location())
}

func (f *FrozenTime) Add(dur time.Duration) MockTime {
	f.instant = f.instant.Add(dur)
	return f
}

func (f *FrozenTime) Now() time.Time {
	return f.instant
}

func (f *FrozenTime) Sleep(dur time.Duration) {
	f.sleep(dur)
}

var _ = MockTime(&FrozenTime{})

// NewFrozenTime returns a new frozen time.
// You can use the methods add, addDate, setTime to move forwards/backwards in time
func NewFrozenTime(now ...time.Time) *FrozenTime {
	t := time.Now()
	out := &FrozenTime{
		instant: t,
		sleep: func(d time.Duration) {
			time.Sleep(d)
		},
	}
	for _, v := range now {
		out.SetNow(v)
	}
	return out
}

func (f *FrozenTime) WithSleep(slp func(time.Duration)) *FrozenTime {
	f.sleep = slp
	return f
}

// ReferenceTime allows us to bind our time to a
// fixed reference.
type ReferenceTime struct {
	jonson.Shareable
	now   time.Time
	ref   time.Time
	sleep func(time.Duration)
}

func (m *ReferenceTime) SetReference(tm time.Time) {
	m.ref = tm
}

// SetNow sets the current timestamp
func (m *ReferenceTime) SetNow(tm time.Time) {
	m.now = tm
}

func (m *ReferenceTime) AddDate(years int, months int, days int) MockTime {
	m.now = m.now.AddDate(years, months, days)
	return m
}

func (m *ReferenceTime) Add(dur time.Duration) MockTime {
	m.now = m.now.Add(dur)
	return m
}

func (m *ReferenceTime) Now() time.Time {
	return m.now.Add(time.Since(m.ref))
}

func (m *ReferenceTime) Sleep(dur time.Duration) {
	m.sleep(dur)
}

func (m *ReferenceTime) WithSleep(slp func(time.Duration)) *ReferenceTime {
	m.sleep = slp
	return m
}

var _ = MockTime(&ReferenceTime{})

// NewReferenceTime returns a new time which is bound to a reference.
// Each call to Now() will add the time that passed since the reference time.
func NewReferenceTime(reference time.Time, now ...time.Time) *ReferenceTime {
	out := &ReferenceTime{
		now: reference,
		ref: reference,
		sleep: func(d time.Duration) {
			time.Sleep(d)
		},
	}
	for _, v := range now {
		out.SetNow(v)
	}
	return out
}
