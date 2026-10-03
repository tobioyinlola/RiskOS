package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`) //nolint:gochecknoglobals // compiled once, never mutated

// RuleInput describes a new rule.
type RuleInput struct {
	Code        string
	Name        string
	Description string
	Severity    string
	Weight      int
	Mode        domain.RuleMode
	Enabled     bool
	Condition   json.RawMessage
}

// RulePatch changes a rule; nil fields keep their current value.
type RulePatch struct {
	Name        *string
	Description *string
	Severity    *string
	Weight      *int
	Mode        *domain.RuleMode
	Enabled     *bool
	Condition   json.RawMessage
}

func compileCondition(raw json.RawMessage) (rules.Compiled, error) { return rules.Compile(raw) }

// validateVersion checks a rule version and returns its condition in canonical (compact) form.
func validateVersion(v domain.RuleVersion) (json.RawMessage, error) {
	invalid := func(msg string) error { return fmt.Errorf("%w: %s", domain.ErrRuleInvalid, msg) }
	if n := utf8.RuneCountInString(strings.TrimSpace(v.Name)); n < 1 || n > 100 {
		return nil, invalid("name must be 1-100 characters")
	}
	if utf8.RuneCountInString(v.Description) > 1000 {
		return nil, invalid("description must be at most 1000 characters")
	}
	if !slices.Contains([]string{"low", "medium", "high", "critical"}, v.Severity) {
		return nil, invalid("severity must be low, medium, high or critical")
	}
	if v.Weight < 0 || v.Weight > domain.MaxScore {
		return nil, invalid("weight must be between 0 and 100")
	}
	if v.Mode != domain.ModeActive && v.Mode != domain.ModeShadow {
		return nil, invalid("mode must be active or shadow")
	}
	if _, err := rules.Compile(v.Condition); err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrRuleInvalid, strings.TrimPrefix(err.Error(), rules.ErrInvalid.Error()+": "))
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, v.Condition); err != nil {
		return nil, invalid("condition must be JSON")
	}
	return compact.Bytes(), nil
}

// RuleResult is the outcome of a rule change: the rule as it now is and, in a production
// environment, the proposed change awaiting approval instead (nothing applied yet).
type RuleResult struct {
	Rule   domain.Rule
	Change *domain.RuleChange
}

// needsApproval reports whether rule changes in the scope's environment wait for a second
// person: production does, test applies them at once.
func needsApproval(scope tenancy.Scope) bool {
	return scope.EnvironmentKind == tenancy.EnvironmentProduction
}

func checkRuleLimit(ctx context.Context, st interface {
	CountRules(context.Context) (int64, error)
},
) error {
	n, err := st.CountRules(ctx)
	if err != nil {
		return err
	}
	if n >= domain.MaxRulesPerEnvironment {
		return fmt.Errorf("%w: an environment can have at most %d rules; disable or edit existing ones",
			domain.ErrRuleLimit, domain.MaxRulesPerEnvironment)
	}
	return nil
}

// CreateRule creates a rule at version 1, or in production proposes it for approval. Requires
// rules:write.
func (s *Service) CreateRule(ctx context.Context, in RuleInput) (RuleResult, error) {
	if err := authz.Require(ctx, authz.RulesWrite); err != nil {
		return RuleResult{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return RuleResult{}, err
	}
	if !codePattern.MatchString(in.Code) {
		return RuleResult{}, fmt.Errorf("%w: code must match %s", domain.ErrRuleInvalid, codePattern)
	}
	if in.Code == domain.SignalComplianceReview {
		return RuleResult{}, fmt.Errorf("%w: code %q is reserved for compliance-only signals", domain.ErrRuleInvalid, in.Code)
	}
	if in.Code == domain.SignalDenyList {
		return RuleResult{}, fmt.Errorf("%w: code %q is reserved for deny lists", domain.ErrRuleInvalid, in.Code)
	}
	if _, builtin := domain.BuiltinSignal(in.Code); builtin {
		return RuleResult{}, fmt.Errorf("%w: code %q is a built-in signal", domain.ErrRuleInvalid, in.Code)
	}
	v := domain.RuleVersion{
		Version: 1, Name: strings.TrimSpace(in.Name), Description: in.Description, Severity: in.Severity,
		Weight: in.Weight, Mode: in.Mode, Enabled: in.Enabled, Condition: in.Condition,
		CreatedBy: scope.Principal.ID, CreatedAt: s.clock.Now(),
	}
	if v.Condition, err = validateVersion(v); err != nil {
		return RuleResult{}, err
	}
	r := domain.Rule{ID: ids.NewV7(), Code: in.Code, CreatedAt: v.CreatedAt, Current: v}
	if needsApproval(scope) {
		c := domain.RuleChange{ID: ids.NewV7(), RuleID: r.ID, Code: in.Code, Kind: domain.ChangeCreate, Proposed: v}
		return s.propose(ctx, scope, domain.Rule{}, c)
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if err := checkRuleLimit(ctx, s.store); err != nil {
			return err
		}
		if err := s.store.InsertRule(ctx, r.ID, r.Code, v); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "rule.created", ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, r.ID),
			Outcome: audit.OutcomeSuccess, Metadata: versionMetadata(r.Code, v),
		})
	})
	if err != nil {
		return RuleResult{}, err
	}
	s.config.invalidate(ctx)
	return RuleResult{Rule: r}, nil
}

// UpdateRule applies patch as a new version (in production: proposes it). A patch that changes
// nothing returns the rule unchanged and records nothing. Requires rules:write.
func (s *Service) UpdateRule(ctx context.Context, id uuid.UUID, patch RulePatch) (RuleResult, error) {
	return s.newVersion(ctx, id, domain.ChangeUpdate, func(_ context.Context, cur domain.RuleVersion) (domain.RuleVersion, error) {
		next := cur
		next.RestoredFrom = nil
		if patch.Name != nil {
			next.Name = strings.TrimSpace(*patch.Name)
		}
		if patch.Description != nil {
			next.Description = *patch.Description
		}
		if patch.Severity != nil {
			next.Severity = *patch.Severity
		}
		if patch.Weight != nil {
			next.Weight = *patch.Weight
		}
		if patch.Mode != nil {
			next.Mode = *patch.Mode
		}
		if patch.Enabled != nil {
			next.Enabled = *patch.Enabled
		}
		if patch.Condition != nil {
			next.Condition = patch.Condition
		}
		return next, nil
	})
}

// RollbackRule restores an earlier version as a new version (history is never rewritten); in
// production it proposes the rollback. Requires rules:write.
func (s *Service) RollbackRule(ctx context.Context, id uuid.UUID, toVersion int) (RuleResult, error) {
	return s.newVersion(ctx, id, domain.ChangeRollback, func(ctx context.Context, cur domain.RuleVersion) (domain.RuleVersion, error) {
		if toVersion == cur.Version {
			return domain.RuleVersion{}, fmt.Errorf("%w: version %d is already current", domain.ErrRuleInvalid, toVersion)
		}
		old, err := s.store.GetVersion(ctx, id, toVersion)
		if errors.Is(err, domain.ErrRuleNotFound) {
			return domain.RuleVersion{}, fmt.Errorf("%w: version %d does not exist", domain.ErrRuleInvalid, toVersion)
		}
		if err != nil {
			return domain.RuleVersion{}, err
		}
		old.RestoredFrom = &toVersion
		return old, nil
	})
}

// actions are the audit actions of applied rule changes.
var actions = map[domain.ChangeKind]string{ //nolint:gochecknoglobals // constant lookup table
	domain.ChangeCreate: "rule.created", domain.ChangeUpdate: "rule.updated", domain.ChangeRollback: "rule.rolled_back",
}

func (s *Service) newVersion(ctx context.Context, id uuid.UUID, kind domain.ChangeKind,
	next func(context.Context, domain.RuleVersion) (domain.RuleVersion, error),
) (RuleResult, error) {
	if err := authz.Require(ctx, authz.RulesWrite); err != nil {
		return RuleResult{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return RuleResult{}, err
	}
	var r domain.Rule
	var proposal *domain.RuleChange
	changed := false
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if r, err = s.store.GetRule(ctx, id); err != nil {
			return err
		}
		v, err := next(ctx, r.Current)
		if err != nil {
			return err
		}
		if v.Condition, err = validateVersion(v); err != nil {
			return err
		}
		if sameContent(v, r.Current) && v.RestoredFrom == nil {
			return nil // nothing changed
		}
		if needsApproval(scope) {
			base := r.Current.Version
			proposal = &domain.RuleChange{ID: ids.NewV7(), RuleID: id, Code: r.Code, Kind: kind, BaseVersion: &base, Proposed: v}
			return nil
		}
		v.Version, v.CreatedBy, v.CreatedAt = r.Current.Version+1, scope.Principal.ID, s.clock.Now()
		if err := s.applyVersion(ctx, r, v, kind, nil); err != nil {
			return err
		}
		r.Current, changed = v, true
		return nil
	})
	if err != nil {
		return RuleResult{}, err
	}
	if proposal != nil {
		return s.propose(ctx, scope, r, *proposal)
	}
	if changed {
		s.config.invalidate(ctx)
	}
	return RuleResult{Rule: r}, nil
}

// applyVersion appends v as the rule's next version and audits it. approval is the change it
// applies after a second person approved it, if any.
func (s *Service) applyVersion(ctx context.Context, r domain.Rule, v domain.RuleVersion, kind domain.ChangeKind, approval *domain.RuleChange) error {
	if err := s.store.AddVersion(ctx, r.ID, r.Current.Version, v); err != nil {
		return err
	}
	meta := versionMetadata(r.Code, v)
	meta["previous_version"] = r.Current.Version
	if v.RestoredFrom != nil {
		meta["restored_from"] = *v.RestoredFrom
	}
	approvalMetadata(meta, approval)
	return s.audit.Record(ctx, audit.Entry{
		Action: actions[kind], ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, r.ID),
		Outcome: audit.OutcomeSuccess, Metadata: meta,
	})
}

func sameContent(a, b domain.RuleVersion) bool {
	return a.Name == b.Name && a.Description == b.Description && a.Severity == b.Severity &&
		a.Weight == b.Weight && a.Mode == b.Mode && a.Enabled == b.Enabled && bytes.Equal(a.Condition, b.Condition)
}

func versionMetadata(code string, v domain.RuleVersion) map[string]any {
	return map[string]any{
		"code": code, "version": v.Version, "mode": string(v.Mode), "enabled": v.Enabled,
		"weight": v.Weight, "severity": v.Severity,
	}
}

// ListRules returns the environment's rules with their current versions. Requires rules:read.
func (s *Service) ListRules(ctx context.Context) ([]domain.Rule, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return nil, err
	}
	var out []domain.Rule
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if out, err = s.store.ListRules(ctx); err != nil {
			return err
		}
		pending, err := s.store.PendingChanges(ctx)
		if err != nil {
			return err
		}
		for i := range out {
			if c, ok := pending[out[i].ID]; ok {
				out[i].PendingChange = &c
			}
		}
		return nil
	})
	return out, err
}

// GetRule returns a rule. Requires rules:read.
func (s *Service) GetRule(ctx context.Context, id uuid.UUID) (domain.Rule, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return domain.Rule{}, err
	}
	var r domain.Rule
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if r, err = s.store.GetRule(ctx, id); err != nil {
			return err
		}
		pending, err := s.store.PendingChanges(ctx)
		if c, ok := pending[id]; ok {
			r.PendingChange = &c
		}
		return err
	})
	return r, err
}

// RuleVersions returns a rule's history, newest first. Requires rules:read.
func (s *Service) RuleVersions(ctx context.Context, id uuid.UUID) (domain.Rule, []domain.RuleVersion, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return domain.Rule{}, nil, err
	}
	var r domain.Rule
	var vs []domain.RuleVersion
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if r, err = s.store.GetRule(ctx, id); err != nil {
			return err
		}
		vs, err = s.store.ListVersions(ctx, id)
		return err
	})
	return r, vs, err
}

// TestResult is the outcome of testing a rule against an event.
type TestResult struct {
	Rule    domain.Rule
	Matched bool
	// Facts are the values of the fields the condition references (absent ones are omitted).
	Facts map[string]any
	// Draft reports whether a draft condition was tested instead of the current version.
	Draft bool
}

// TestRule evaluates a rule's current condition, or a draft condition, against an event using
// read-only features: nothing is stored and no feature state changes. Requires rules:read.
func (s *Service) TestRule(ctx context.Context, id uuid.UUID, normalized []byte, draft json.RawMessage) (TestResult, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return TestResult{}, err
	}
	ev, err := features.ParseEvent(normalized)
	if err != nil {
		return TestResult{}, err
	}
	var res TestResult
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if res.Rule, err = s.store.GetRule(ctx, id); err != nil {
			return err
		}
		condition := res.Rule.Current.Condition
		if draft != nil {
			condition, res.Draft = draft, true
		}
		compiled, err := rules.Compile(condition)
		if err != nil {
			return fmt.Errorf("%w: %s", domain.ErrRuleInvalid, strings.TrimPrefix(err.Error(), rules.ErrInvalid.Error()+": "))
		}
		facts := features.Facts(ev, s.features.Observe(ctx, ev, false))
		res.Matched = compiled.Match(facts)
		res.Facts = map[string]any{}
		for _, f := range compiled.Fields() {
			if v, ok := facts[f]; ok {
				res.Facts[f] = v
			}
		}
		return nil
	})
	return res, err
}
