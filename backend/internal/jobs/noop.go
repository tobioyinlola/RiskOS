package jobs

import (
	"context"
	"log/slog"
)

// NoopArgs is a job that does nothing but log. The worker enqueues one at startup as a
// self-check proving the enqueue → fetch → execute path works end to end.
type NoopArgs struct {
	// Reason records why the job was enqueued, for operators reading logs.
	Reason string `json:"reason"`
}

// Kind implements Args.
func (NoopArgs) Kind() string { return "noop" }

// NoopHandler returns a handler that logs each executed noop job.
func NoopHandler(logger *slog.Logger) func(context.Context, NoopArgs) error {
	return func(ctx context.Context, args NoopArgs) error {
		logger.InfoContext(ctx, "noop job executed", slog.String("reason", args.Reason))
		return nil
	}
}
