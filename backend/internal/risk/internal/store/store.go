// Package store is the only code that reads or writes the evaluations table.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store/sqlcgen"
)

// ErrNotFound is returned when an evaluation does not exist in the current scope.
var ErrNotFound = errors.New("evaluation not found")

// Store operates inside the tenant transaction carried by the context.
type Store struct{}

func scoped(ctx context.Context) (*sqlcgen.Queries, tenancy.Scope, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	return sqlcgen.New(tx), scope, nil
}

// Insert stores an evaluation with everything needed to reproduce it: engine and rule versions,
// the features (normalized event and facts), signals, shadow-rule matches and policy reasons.
func (Store) Insert(ctx context.Context, r domain.Result, eventKey string, features []byte, duration time.Duration) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	signals, err := json.Marshal(nonNil(r.Signals))
	if err != nil {
		return fmt.Errorf("encode signals: %w", err)
	}
	shadow, err := json.Marshal(nonNil(r.ShadowSignals))
	if err != nil {
		return fmt.Errorf("encode shadow signals: %w", err)
	}
	us := min(duration.Microseconds(), int64(math.MaxInt32))
	if err := q.InsertEvaluation(ctx, sqlcgen.InsertEvaluationParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID,
		ID: r.ID, CreatedAt: r.CreatedAt, EventID: r.EventID, EventKey: eventKey, DryRun: r.EventID == nil,
		EngineVersion: r.EngineVersion, RuleVersions: nonNilStrings(r.RuleVersions), Features: features,
		RiskScore: int16(r.Score), RiskLevel: string(r.Level), Decision: string(r.Decision), //nolint:gosec // score is 0..100
		Signals: signals, DegradedSignals: nonNilStrings(r.DegradedSignals), RequestID: r.RequestID,
		DurationUs:    int32(us), //nolint:gosec // clamped above
		PolicyReasons: nonNilStrings(r.PolicyReasons), ShadowSignals: shadow,
	}); err != nil {
		return fmt.Errorf("insert evaluation: %w", err)
	}
	return nil
}

// Get returns an evaluation of the current environment. createdAt must be the time embedded in
// id (UUIDv7), which selects the partition.
func (Store) Get(ctx context.Context, id uuid.UUID, createdAt time.Time) (domain.Result, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Result{}, err
	}
	row, err := q.GetEvaluation(ctx, sqlcgen.GetEvaluationParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, CreatedAt: createdAt, ID: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Result{}, ErrNotFound
	}
	if err != nil {
		return domain.Result{}, fmt.Errorf("get evaluation: %w", err)
	}
	var signals []domain.Signal
	if err := json.Unmarshal(row.Signals, &signals); err != nil {
		return domain.Result{}, fmt.Errorf("decode signals: %w", err)
	}
	return domain.Result{
		ID: row.ID, CreatedAt: row.CreatedAt, EventID: row.EventID, EngineVersion: row.EngineVersion,
		Score: int(row.RiskScore), Level: domain.Level(row.RiskLevel), Decision: domain.Decision(row.Decision),
		Signals: nonNil(signals), DegradedSignals: nonNilStrings(row.DegradedSignals), RequestID: row.RequestID,
	}, nil
}

func nonNil(s []domain.Signal) []domain.Signal {
	if s == nil {
		return []domain.Signal{}
	}
	return s
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// TopSignals counts how often each signal fired in events since since, most frequent first.
func (Store) TopSignals(ctx context.Context, since time.Time) ([]domain.SignalCount, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.TopSignals(ctx, sqlcgen.TopSignalsParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since})
	if err != nil {
		return nil, fmt.Errorf("top signals: %w", err)
	}
	out := make([]domain.SignalCount, len(rows))
	for i, r := range rows {
		out[i] = domain.SignalCount{Code: r.Code, Label: r.Label, Hits: int(r.Hits)}
	}
	return out, nil
}
