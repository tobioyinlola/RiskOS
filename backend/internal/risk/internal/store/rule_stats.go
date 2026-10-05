package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store/sqlcgen"
)

// RuleDay is how many evaluations matched a signal code on one UTC day, and the last of them.
type RuleDay struct {
	Code    string
	Day     time.Time
	Matches int64
	LastAt  time.Time
}

// RuleMatchesByDay counts, per UTC day since since, the evaluations each of codes matched
// (active or monitoring). One aggregate query, never a query per event.
func (Store) RuleMatchesByDay(ctx context.Context, since time.Time, codes []string) ([]RuleDay, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.RuleMatchesByDay(ctx, sqlcgen.RuleMatchesByDayParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since, Codes: codes,
	})
	if err != nil {
		return nil, fmt.Errorf("rule matches: %w", err)
	}
	out := make([]RuleDay, len(rows))
	for i, r := range rows {
		out[i] = RuleDay{Code: r.Code, Day: r.Day.Time, Matches: r.Matches, LastAt: r.LastAt}
	}
	return out, nil
}

// CountEvaluationsSince counts the evaluations (not dry runs) since a time.
func (Store) CountEvaluationsSince(ctx context.Context, since time.Time) (int64, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountEvaluationsSince(ctx, sqlcgen.CountEvaluationsSinceParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since,
	})
	if err != nil {
		return 0, fmt.Errorf("count evaluations: %w", err)
	}
	return n, nil
}

// RecentFacts is an evaluation's recorded facts and outcome.
type RecentFacts struct {
	ID       uuid.UUID
	EventID  *uuid.UUID
	At       time.Time
	Facts    []byte
	Decision string
	Score    int
}

// RecentFacts returns the newest evaluations (not dry runs) since a time, at most limit, in
// one read.
func (Store) RecentFacts(ctx context.Context, since time.Time, limit int) ([]RecentFacts, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.RecentFacts(ctx, sqlcgen.RecentFactsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since, MaxRows: int32(limit), //nolint:gosec // bounded by the caller
	})
	if err != nil {
		return nil, fmt.Errorf("recent facts: %w", err)
	}
	out := make([]RecentFacts, len(rows))
	for i, r := range rows {
		out[i] = RecentFacts{ID: r.ID, EventID: r.EventID, At: r.CreatedAt, Facts: r.Facts, Decision: r.Decision, Score: int(r.RiskScore)}
	}
	return out, nil
}
