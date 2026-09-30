// Package health implements liveness and readiness.
//
// Liveness answers "is the process able to serve HTTP" and never touches dependencies, so a
// database outage cannot make an orchestrator restart healthy API pods. Readiness answers "should
// this instance receive traffic": every dependency check must pass within its timeout, and the
// instance must not be draining for shutdown.
package health

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// Check probes one dependency.
type Check struct {
	Name    string
	Timeout time.Duration
	Probe   func(context.Context) error
}

// Service evaluates readiness.
type Service struct {
	logger   *slog.Logger
	checks   []Check
	draining atomic.Bool
}

// NewService returns a Service running checks on every readiness request.
func NewService(logger *slog.Logger, checks ...Check) *Service {
	return &Service{logger: logger, checks: checks}
}

// StartDraining makes readiness fail from now on. It is called when shutdown begins.
func (s *Service) StartDraining() { s.draining.Store(true) }

// Readiness runs all checks concurrently and reports per-check results and overall readiness.
func (s *Service) Readiness(ctx context.Context) (ready bool, results map[string]bool) {
	results = make(map[string]bool, len(s.checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range s.checks {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, c.Timeout)
			defer cancel()
			err := c.Probe(cctx)
			if err != nil {
				// The cause is logged for operators and never returned to callers.
				s.logger.WarnContext(ctx, "readiness check failed", slog.String("check", c.Name), slog.Any("error", err))
			}
			mu.Lock()
			results[c.Name] = err == nil
			mu.Unlock()
		})
	}
	wg.Wait()

	ready = !s.draining.Load()
	for _, ok := range results {
		ready = ready && ok
	}
	return ready, results
}

// Handlers implements the health operations of apigen.StrictServerInterface.
type Handlers struct {
	svc *Service
}

// NewHandlers returns the health HTTP handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// GetLiveness implements GET /health/live.
func (h *Handlers) GetLiveness(context.Context, apigen.GetLivenessRequestObject) (apigen.GetLivenessResponseObject, error) {
	return apigen.GetLiveness200JSONResponse{Body: apigen.LivenessResponse{Status: apigen.LivenessResponseStatusOk}}, nil
}

// GetReadiness implements GET /health/ready.
func (h *Handlers) GetReadiness(ctx context.Context, _ apigen.GetReadinessRequestObject) (apigen.GetReadinessResponseObject, error) {
	ready, results := h.svc.Readiness(ctx)
	body := apigen.ReadinessResponse{
		Status: apigen.ReadinessResponseStatusOk,
		Checks: make(map[string]apigen.ReadinessResponseChecks, len(results)),
	}
	for name, ok := range results {
		if ok {
			body.Checks[name] = apigen.ReadinessResponseChecksOk
		} else {
			body.Checks[name] = apigen.ReadinessResponseChecksUnavailable
		}
	}
	if !ready {
		body.Status = apigen.ReadinessResponseStatusUnavailable
		return apigen.GetReadiness503JSONResponse{Body: body}, nil
	}
	return apigen.GetReadiness200JSONResponse{Body: body}, nil
}
