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

// CreateRule creates a rule at version 1. Requires rules:write.
func (s *Service) CreateRule(ctx context.Context, in RuleInput) (domain.Rule, error) {
	if err := authz.Require(ctx, authz.RulesWrite); err != nil {
		return domain.Rule{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Rule{}, err
	}
	if !codePattern.MatchString(in.Code) {
		return domain.Rule{}, fmt.Errorf("%w: code must match %s", domain.ErrRuleInvalid, codePattern)
	}
	if _, builtin := domain.BuiltinSignal(in.Code); builtin {
		return domain.Rule{}, fmt.Errorf("%w: code %q is a built-in signal", domain.ErrRuleInvalid, in.Code)
	}
	v := domain.RuleVersion{
		Version: 1, Name: strings.TrimSpace(in.Name), Description: in.Description, Severity: in.Severity,
		Weight: in.Weight, Mode: in.Mode, Enabled: in.Enabled, Condition: in.Condition,
		CreatedBy: scope.Principal.ID, CreatedAt: s.clock.Now(),
	}
	if v.Condition, err = validateVersion(v); err != nil {
		return domain.Rule{}, err
	}
	r := domain.Rule{ID: ids.NewV7(), Code: in.Code, CreatedAt: v.CreatedAt, Current: v}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		n, err := s.store.CountRules(ctx)
		if err != nil {
			return err
		}
		if n >= domain.MaxRulesPerEnvironment {
			return fmt.Errorf("%w: an environment can have at most %d rules; disable or edit existing ones",
				domain.ErrRuleLimit, domain.MaxRulesPerEnvironment)
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
		return domain.Rule{}, err
	}
	s.config.invalidate(ctx)
	return r, nil
}

// UpdateRule applies patch as a new version. A patch that changes nothing returns the rule
// unchanged and records nothing. Requires rules:write.
func (s *Service) UpdateRule(ctx context.Context, id uuid.UUID, patch RulePatch) (domain.Rule, error) {
	return s.newVersion(ctx, id, "rule.updated", func(_ context.Context, cur domain.RuleVersion) (domain.RuleVersion, error) {
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

// RollbackRule restores an earlier version as a new version (history is never rewritten).
// Requires rules:write.
func (s *Service) RollbackRule(ctx context.Context, id uuid.UUID, toVersion int) (domain.Rule, error) {
	return s.newVersion(ctx, id, "rule.rolled_back", func(ctx context.Context, cur domain.RuleVersion) (domain.RuleVersion, error) {
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

func (s *Service) newVersion(ctx context.Context, id uuid.UUID, action string,
	next func(context.Context, domain.RuleVersion) (domain.RuleVersion, error),
) (domain.Rule, error) {
	if err := authz.Require(ctx, authz.RulesWrite); err != nil {
		return domain.Rule{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Rule{}, err
	}
	var r domain.Rule
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
		v.Version, v.CreatedBy, v.CreatedAt = r.Current.Version+1, scope.Principal.ID, s.clock.Now()
		if err := s.store.AddVersion(ctx, id, r.Current.Version, v); err != nil {
			return err
		}
		meta := versionMetadata(r.Code, v)
		meta["previous_version"] = r.Current.Version
		if v.RestoredFrom != nil {
			meta["restored_from"] = *v.RestoredFrom
		}
		r.Current = v
		return s.audit.Record(ctx, audit.Entry{
			Action: action, ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, id),
			Outcome: audit.OutcomeSuccess, Metadata: meta,
		})
	})
	if err != nil {
		return domain.Rule{}, err
	}
	s.config.invalidate(ctx)
	return r, nil
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
		out, err = s.store.ListRules(ctx)
		return err
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
		r, err = s.store.GetRule(ctx, id)
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
