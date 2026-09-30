// Package engine holds signal-generating engines.
package engine

import (
	"context"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

// StubVersion identifies the stub engine in every evaluation it produces, so stub decisions can
// always be told apart from real ones.
const StubVersion = "risk-engine-stub-0.1"

// Stub is the placeholder engine of build step 3: it produces no signals, so every event scores 0
// (LOW, ALLOW). It exists to prove the evaluation path end to end (persist, respond, reproduce)
// and is replaced by rules, velocity and behavioral generators in build step 4.
type Stub struct{}

// Version implements domain.Engine.
func (Stub) Version() string { return StubVersion }

// Signals implements domain.Engine.
func (Stub) Signals(context.Context, domain.Features) ([]domain.Signal, []string) {
	return nil, nil
}
