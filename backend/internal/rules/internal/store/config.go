package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store/sqlcgen"
)

// ErrConflict means a unique rule was violated: another change for the rule is pending, or the
// version was written concurrently.
var ErrConflict = errors.New("conflict")

// RuleVersion is one version of a rule's configuration in an environment.
type RuleVersion struct {
	RuleID       string
	Version      int
	Mode         string
	Settings     map[string]float64
	Reason       string
	CreatedAt    time.Time
	CreatedBy    string
	ChangeID     *uuid.UUID
	RestoredFrom *int
}

// RuleChange is a configuration change awaiting, or past, a second person's decision.
type RuleChange struct {
	ID             uuid.UUID
	PackID, RuleID string
	BaseVersion    int
	Mode           string
	Settings       map[string]float64
	Reason         string
	RestoredFrom   *int
	Status         string
	ProposedBy     string
	ProposedAt     time.Time
	DecidedBy      *string
	DecidedAt      *time.Time
	DecisionNote   *string
	AppliedVersion *int
}

func scoped(ctx context.Context) (*sqlcgen.Queries, tenancy.Scope, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	return q, scope, err
}

func settingsOf(raw []byte) (map[string]float64, error) {
	out := map[string]float64{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	return out, nil
}

func intPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func int32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v) //nolint:gosec // versions are small
	return &n
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CurrentVersions returns each configured rule's latest version in the scope's environment.
func (Store) CurrentVersions(ctx context.Context, packID string) (map[string]RuleVersion, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.CurrentRuleVersions(ctx, sqlcgen.CurrentRuleVersionsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, PackID: packID,
	})
	if err != nil {
		return nil, fmt.Errorf("current rule versions: %w", err)
	}
	out := make(map[string]RuleVersion, len(rows))
	for _, r := range rows {
		s, err := settingsOf(r.Settings)
		if err != nil {
			return nil, err
		}
		out[r.RuleID] = RuleVersion{
			RuleID: r.RuleID, Version: int(r.Version), Mode: r.Mode, Settings: s, Reason: r.Reason,
			CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, ChangeID: r.ChangeID, RestoredFrom: intPtr(r.RestoredFrom),
		}
	}
	return out, nil
}

// Versions returns a rule's versions, newest first.
func (Store) Versions(ctx context.Context, packID, ruleID string) ([]RuleVersion, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.RuleVersions(ctx, sqlcgen.RuleVersionsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, PackID: packID, RuleID: ruleID,
	})
	if err != nil {
		return nil, fmt.Errorf("rule versions: %w", err)
	}
	out := make([]RuleVersion, len(rows))
	for i, r := range rows {
		s, err := settingsOf(r.Settings)
		if err != nil {
			return nil, err
		}
		out[i] = RuleVersion{
			RuleID: ruleID, Version: int(r.Version), Mode: r.Mode, Settings: s, Reason: r.Reason,
			CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, ChangeID: r.ChangeID, RestoredFrom: intPtr(r.RestoredFrom),
		}
	}
	return out, nil
}

// InsertVersion stores a new version; ErrConflict when the version number is taken.
func (Store) InsertVersion(ctx context.Context, packID string, v RuleVersion) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	settings, err := json.Marshal(v.Settings)
	if err != nil {
		return err
	}
	err = q.InsertRuleVersion(ctx, sqlcgen.InsertRuleVersionParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, PackID: packID, RuleID: v.RuleID,
		Version: int32(v.Version), Mode: v.Mode, Settings: settings, Reason: v.Reason, //nolint:gosec // versions are small
		CreatedAt: v.CreatedAt, CreatedBy: v.CreatedBy, ChangeID: v.ChangeID, RestoredFrom: int32Ptr(v.RestoredFrom),
	})
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("insert rule version: %w", err)
	}
	return nil
}

// InsertChange stores a pending change; ErrConflict when the rule already has one.
func (Store) InsertChange(ctx context.Context, c RuleChange) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	settings, err := json.Marshal(c.Settings)
	if err != nil {
		return err
	}
	err = q.InsertRuleChange(ctx, sqlcgen.InsertRuleChangeParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: c.ID, PackID: c.PackID, RuleID: c.RuleID,
		BaseVersion: int32(c.BaseVersion), Mode: c.Mode, Settings: settings, Reason: c.Reason, //nolint:gosec // versions are small
		RestoredFrom: int32Ptr(c.RestoredFrom), ProposedBy: c.ProposedBy, ProposedAt: c.ProposedAt,
	})
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("insert rule change: %w", err)
	}
	return nil
}

func changeOf(id uuid.UUID, packID, ruleID string, base int32, mode string, settings []byte, reason string,
	restored *int32, status, by string, at time.Time, decidedBy *string, decidedAt *time.Time, note *string, applied *int32,
) (RuleChange, error) {
	s, err := settingsOf(settings)
	if err != nil {
		return RuleChange{}, err
	}
	return RuleChange{
		ID: id, PackID: packID, RuleID: ruleID, BaseVersion: int(base), Mode: mode, Settings: s, Reason: reason,
		RestoredFrom: intPtr(restored), Status: status, ProposedBy: by, ProposedAt: at, DecidedBy: decidedBy,
		DecidedAt: decidedAt, DecisionNote: note, AppliedVersion: intPtr(applied),
	}, nil
}

// Change returns a change, locked for update until the transaction ends.
func (Store) Change(ctx context.Context, id uuid.UUID) (RuleChange, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return RuleChange{}, err
	}
	r, err := q.GetRuleChange(ctx, sqlcgen.GetRuleChangeParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return RuleChange{}, ErrNotFound
	}
	if err != nil {
		return RuleChange{}, fmt.Errorf("get rule change: %w", err)
	}
	return changeOf(r.ID, r.PackID, r.RuleID, r.BaseVersion, r.Mode, r.Settings, r.Reason, r.RestoredFrom, r.Status,
		r.ProposedBy, r.ProposedAt, r.DecidedBy, r.DecidedAt, r.DecisionNote, r.AppliedVersion)
}

// Changes lists changes, newest first; status and ruleID filter when not empty.
func (Store) Changes(ctx context.Context, status, ruleID string) ([]RuleChange, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	p := sqlcgen.ListRuleChangesParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID}
	if status != "" {
		p.Status = &status
	}
	if ruleID != "" {
		p.RuleID = &ruleID
	}
	rows, err := q.ListRuleChanges(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("list rule changes: %w", err)
	}
	out := make([]RuleChange, len(rows))
	for i, r := range rows {
		if out[i], err = changeOf(r.ID, r.PackID, r.RuleID, r.BaseVersion, r.Mode, r.Settings, r.Reason, r.RestoredFrom,
			r.Status, r.ProposedBy, r.ProposedAt, r.DecidedBy, r.DecidedAt, r.DecisionNote, r.AppliedVersion); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Decide records a decision on a pending change.
func (Store) Decide(ctx context.Context, id uuid.UUID, status, by string, at time.Time, note *string, applied *int) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.DecideRuleChange(ctx, sqlcgen.DecideRuleChangeParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, Status: status, DecidedBy: &by,
		DecidedAt: &at, DecisionNote: note, AppliedVersion: int32Ptr(applied),
	}); err != nil {
		return fmt.Errorf("decide rule change: %w", err)
	}
	return nil
}
