package logging

import (
	"context"
	"log/slog"
	"slices"
)

// WarnRotations logs, at start-up, each secret that still has a previous value configured
// (ADR-0008) and what must happen before it is removed. Names and conditions only, never values.
// A previous secret left in place keeps accepting what it accepted, so the reminder repeats on
// every start until it is gone.
func WarnRotations(ctx context.Context, logger *slog.Logger, rotations map[string]string) {
	names := make([]string, 0, len(rotations))
	for name := range rotations {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		logger.WarnContext(ctx, "a previous secret is still configured (secret rotation in progress)",
			slog.String("variable", name), slog.String("remove_when", rotations[name]))
	}
}
