// Package jobs is the background-job boundary. Modules depend only on the types in this file;
// the queue implementation (River, provisional; see docs/adr) is confined to this package so it
// can be replaced without touching callers. depguard forbids importing River anywhere else.
//
// Jobs must be idempotent: every queue in this class delivers at least once.
package jobs

import (
	"context"
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

// Registry collects handlers before the queue starts. Handlers are registered with Register.
type Registry struct {
	registrations []func(*riverWorkers) error
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register binds handler to jobs of kind T.
func Register[T Args](r *Registry, handler func(context.Context, T) error) {
	r.registrations = append(r.registrations, func(w *riverWorkers) error {
		return addRiverWorker(w, handler)
	})
}
