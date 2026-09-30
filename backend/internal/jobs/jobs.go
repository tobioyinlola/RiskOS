// Package jobs is the background-job boundary. Modules depend only on the types in this file;
// the queue implementation (River; see docs/adr/ADR-0003) is confined to this package so it
// can be replaced without touching callers. depguard forbids importing River anywhere else.
//
// Jobs must be idempotent: every queue in this class delivers at least once.
package jobs

import (
	"context"
	"time"
)

// Args is the payload of one job kind. Kind must be unique and stable across releases, because
// persisted jobs are matched to handlers by it. Args are JSON-serialized into the queue table:
// never put secrets in them.
type Args interface {
	Kind() string
}

// Enqueuer schedules jobs for asynchronous execution.
type Enqueuer interface {
	Enqueue(ctx context.Context, args Args) error
}

// Registry collects handlers and periodic jobs before the queue starts.
type Registry struct {
	registrations []func(*riverWorkers) error
	periodic      []periodicSpec
}

// periodicSpec schedules args every interval, and once when the worker starts.
type periodicSpec struct {
	interval time.Duration
	args     Args
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register binds handler to jobs of kind T.
func Register[T Args](r *Registry, handler func(context.Context, T) error) {
	r.registrations = append(r.registrations, func(w *riverWorkers) error {
		return addRiverWorker(w, handler)
	})
}

// RegisterPeriodic binds handler to jobs of kind T and schedules args every interval, plus once
// at worker start. With several workers, the queue's leader election ensures each run is
// scheduled once. Periodic handlers must be idempotent.
func RegisterPeriodic[T Args](r *Registry, interval time.Duration, args T, handler func(context.Context, T) error) {
	Register(r, handler)
	r.periodic = append(r.periodic, periodicSpec{interval: interval, args: args})
}
