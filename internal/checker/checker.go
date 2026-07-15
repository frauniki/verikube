/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package checker implements the network probes executed by runner pods.
// Checkers are pure: they observe an outcome and never apply expect
// semantics or retries — that is the runner's job.
package checker

import (
	"context"
	"time"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

// Result is the raw outcome of a single probe attempt.
type Result struct {
	// Observed is true when the probe succeeded (connection established,
	// expected HTTP status seen). Expect inversion is applied later.
	Observed bool
	// Message explains the failure when Observed is false.
	Message string
	// Duration of the probe attempt.
	Duration time.Duration
}

// Probe type names, matching the CheckSpec oneof field names.
const (
	TypeTCP  = "tcp"
	TypeHTTP = "http"
	TypeGRPC = "grpc"
)

// Checker executes one type of probe.
type Checker interface {
	// Type matches the CheckSpec oneof field name, e.g. TypeTCP.
	Type() string
	Check(ctx context.Context, spec verikubev1alpha1.CheckSpec) Result
}

// Registry maps probe types to checkers.
type Registry struct {
	checkers map[string]Checker
}

// NewRegistry returns a registry with the given checkers registered.
func NewRegistry(checkers ...Checker) *Registry {
	r := &Registry{checkers: map[string]Checker{}}
	for _, c := range checkers {
		r.checkers[c.Type()] = c
	}
	return r
}

// ForSpec picks the checker for whichever probe field is set on the spec.
// It returns false when the spec's probe type has no registered checker,
// e.g. a runner image older than the CRD schema.
func (r *Registry) ForSpec(spec verikubev1alpha1.CheckSpec) (Checker, bool) {
	var typ string
	switch {
	case spec.TCP != nil:
		typ = TypeTCP
	case spec.HTTP != nil:
		typ = TypeHTTP
	case spec.GRPC != nil:
		typ = TypeGRPC
	default:
		return nil, false
	}
	c, ok := r.checkers[typ]
	return c, ok
}
