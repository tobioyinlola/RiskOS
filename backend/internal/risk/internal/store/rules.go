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

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store/sqlcgen"
)

// CountRules returns how many rules the environment has (enabled or not).
func (Store) CountRules(ctx context.Context) (int64, error) {
	q, _, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountRules(ctx)
	if err != nil {
		return 0, fmt.Errorf("count rules: %w", err)
	}
	return n, nil
}

// InsertRule creates a rule with its first version.
func (Store) InsertRule(ctx context.Context, id uuid.UUID, code string, v domain.RuleVersion) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertRule(ctx, sqlcgen.InsertRuleParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, Code: code, CreatedBy: v.CreatedBy,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrRuleCodeTaken
		}
		return fmt.Errorf("insert rule: %w", err)
	}
	return insertVersion(ctx, q, id, v)
}

// AddVersion appends v to a rule and makes it current, if the rule is still at expected.
func (Store) AddVersion(ctx context.Context, id uuid.UUID, expected int, v domain.RuleVersion) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := insertVersion(ctx, q, id, v); err != nil {
		return err
	}
	n, err := q.SetRuleCurrentVersion(ctx, sqlcgen.SetRuleCurrentVersionParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id,
		NewVersion: int32(v.Version), ExpectedVersion: int32(expected), //nolint:gosec // versions are small
	})
	if err != nil {
		return fmt.Errorf("move rule version: %w", err)
	}
	if n != 1 {
		return domain.ErrRuleConflict
	}
	return nil
}

func insertVersion(ctx context.Context, q *sqlcgen.Queries, id uuid.UUID, v domain.RuleVersion) error {
	_, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	var restored *int32
	if v.RestoredFrom != nil {
		r := int32(*v.RestoredFrom) //nolint:gosec // versions are small
		restored = &r
	}
	if err := q.InsertRuleVersion(ctx, sqlcgen.InsertRuleVersionParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RuleID: id,
		Version: int32(v.Version), Name: v.Name, Description: v.Description, Severity: v.Severity, //nolint:gosec // small
		Weight: int16(v.Weight), Mode: string(v.Mode), Enabled: v.Enabled, Condition: v.Condition, //nolint:gosec // 0..100
		CreatedBy: v.CreatedBy, RestoredFrom: restored,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrRuleConflict
		}
		return fmt.Errorf("insert rule version: %w", err)
	}
	return nil
}

// GetRule returns a rule with its current version.
func (Store) GetRule(ctx context.Context, id uuid.UUID) (domain.Rule, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Rule{}, err
	}
	row, err := q.GetCurrentRule(ctx, sqlcgen.GetCurrentRuleParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Rule{}, domain.ErrRuleNotFound
	}
	if err != nil {
		return domain.Rule{}, fmt.Errorf("get rule: %w", err)
	}
	return toRule(sqlcgen.ListCurrentRulesRow(row)), nil
}

// ListRules returns every rule of the environment with its current version, by code.
func (Store) ListRules(ctx context.Context) ([]domain.Rule, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCurrentRules(ctx, sqlcgen.ListCurrentRulesParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID})
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	out := make([]domain.Rule, len(rows))
	for i, r := range rows {
		out[i] = toRule(r)
	}
	return out, nil
}

// ListVersions returns a rule's versions, newest first.
func (Store) ListVersions(ctx context.Context, id uuid.UUID) ([]domain.RuleVersion, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRuleVersions(ctx, sqlcgen.ListRuleVersionsParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RuleID: id})
	if err != nil {
		return nil, fmt.Errorf("list rule versions: %w", err)
	}
	out := make([]domain.RuleVersion, len(rows))
	for i, r := range rows {
		out[i] = toVersion(sqlcgen.GetRuleVersionRow(r))
	}
	return out, nil
}

// GetVersion returns one version of a rule.
func (Store) GetVersion(ctx context.Context, id uuid.UUID, version int) (domain.RuleVersion, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.RuleVersion{}, err
	}
	row, err := q.GetRuleVersion(ctx, sqlcgen.GetRuleVersionParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, RuleID: id, Version: int32(version), //nolint:gosec // small
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RuleVersion{}, domain.ErrRuleNotFound
	}
	if err != nil {
		return domain.RuleVersion{}, fmt.Errorf("get rule version: %w", err)
	}
	return toVersion(row), nil
}

func toRule(r sqlcgen.ListCurrentRulesRow) domain.Rule {
	return domain.Rule{
		ID: r.ID, Code: r.Code, CreatedAt: r.CreatedAt,
		Current: toVersion(sqlcgen.GetRuleVersionRow{
			Version: r.Version, Name: r.Name, Description: r.Description, Severity: r.Severity, Weight: r.Weight,
			Mode: r.Mode, Enabled: r.Enabled, Condition: r.Condition, CreatedAt: r.VersionCreatedAt,
			CreatedBy: r.VersionCreatedBy, RestoredFrom: r.RestoredFrom,
		}),
	}
}

func toVersion(r sqlcgen.GetRuleVersionRow) domain.RuleVersion {
	v := domain.RuleVersion{
		Version: int(r.Version), Name: r.Name, Description: r.Description, Severity: r.Severity,
		Weight: int(r.Weight), Mode: domain.RuleMode(r.Mode), Enabled: r.Enabled, Condition: r.Condition,
		CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy,
	}
	if r.RestoredFrom != nil {
		n := int(*r.RestoredFrom)
		v.RestoredFrom = &n
	}
	return v
}

// GetSettings returns the environment's risk settings, or the defaults when none are stored.
func (Store) GetSettings(ctx context.Context) (domain.Settings, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	row, err := q.GetRiskSettings(ctx, sqlcgen.GetRiskSettingsParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultSettings(), nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("get risk settings: %w", err)
	}
	s := domain.Settings{
		HighValueDefault: row.HighValueDefault, AlertMinDecision: domain.Decision(row.AlertMinDecision),
		AutoBlock: domain.AutoBlockMode(row.AutoBlockMode), AutoBlockSince: row.AutoBlockShadowSince,
	}
	for dst, raw := range map[any][]byte{
		&s.Weights: row.Weights, &s.HighValueByCurrency: row.HighValueByCurrency, &s.FailPolicies: row.FailPolicies,
		&s.Thresholds: row.Thresholds,
	} {
		if err := json.Unmarshal(raw, dst); err != nil {
			return domain.Settings{}, fmt.Errorf("decode risk settings: %w", err)
		}
	}
	return s, nil
}

// PutSettings stores the environment's risk settings.
func (Store) PutSettings(ctx context.Context, s domain.Settings, by string, at time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	enc := func(v any) []byte {
		b, _ := json.Marshal(v) //nolint:errchkjson // maps of strings and numbers always encode
		return b
	}
	if err := q.UpsertRiskSettings(ctx, sqlcgen.UpsertRiskSettingsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID,
		Weights: enc(s.Weights), HighValueDefault: s.HighValueDefault,
		HighValueByCurrency: enc(s.HighValueByCurrency), FailPolicies: enc(s.FailPolicies),
		AlertMinDecision: string(s.AlertMinDecision), Thresholds: enc(s.Thresholds),
		AutoBlockMode: string(s.AutoBlock), AutoBlockShadowSince: s.AutoBlockSince, UpdatedAt: at, UpdatedBy: by,
	}); err != nil {
		return fmt.Errorf("store risk settings: %w", err)
	}
	return nil
}

// CountWouldBlock counts evaluations since since that automatic blocking would have blocked.
func (Store) CountWouldBlock(ctx context.Context, since time.Time) (int64, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountWouldBlock(ctx, sqlcgen.CountWouldBlockParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Since: since})
	if err != nil {
		return 0, fmt.Errorf("count would-block evaluations: %w", err)
	}
	return n, nil
}
