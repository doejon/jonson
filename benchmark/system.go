// Package benchmark provides minimal, exportable Jonson systems and payloads
// for use in external benchmarks and integration tests.
package benchmark

import "github.com/doejon/jonson"

// --- Test System for Benchmarking ---

// System BenchmarkSystem is a mock Jonson system with a single method
// designed to return a complex payload for performance testing.
type System struct{}

// NewBenchmarkSystem provides a pre-configured Jonson system for use in benchmarks.
func NewBenchmarkSystem() *System {
	return &System{}
}

// ComplexResponseV1Result defines the result structure for the benchmark method.
type ComplexResponseV1Result struct {
	Payload *Payload
}

// ComplexResponseV1 is the RPC method that returns a complex data structure.
func (s *System) ComplexResponseV1(_ *jonson.Context) (*ComplexResponseV1Result, error) {
	return &ComplexResponseV1Result{Payload: NewPayload()}, nil
}

// --- Test Payload for Benchmarking ---

// Payload defines a complex data structure used for benchmarking normalization logic.
type Payload struct {
	ID          string
	Users       []User
	Permissions map[string][]string
	Metadata    *Metadata
}

// User is a sub-structure for the benchmark payload.
type User struct {
	Name          string
	Emails, Roles []string
}

// Metadata is a sub-structure for the benchmark payload.
type Metadata struct {
	Logs, ParentIDs []string
}

// NewPayload creates a fresh instance of the complex payload with several nil slices.
func NewPayload() *Payload {
	return &Payload{
		ID:          "payload-123",
		Users:       []User{{Name: "Alice", Roles: nil}, {Name: "Bob", Emails: nil}},
		Permissions: map[string][]string{"admin": {"read", "write"}, "user": nil},
		Metadata:    &Metadata{Logs: nil},
	}
}
