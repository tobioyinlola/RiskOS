package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// Configuration errors.
var (
	// ErrConflict means another change for the rule is pending, or the rule changed since the
	// change was proposed.
	ErrConflict = errors.New("conflict")
	// ErrOwnChange means a person tried to approve their own change.
	ErrOwnChange = errors.New("a change needs another person's approval")
)

// Rule modes in an environment.
const (
	ModeLive   = "live"
	ModeShadow = "shadow"
	ModeOff    = "off"
)

// modeRank orders modes by how much a rule acts: off, shadow, live.
func modeRank(m string) int { return slices.Index([]string{ModeOff, ModeShadow, ModeLive}, m) }

// defaultMode is how a rule starts for profile c: off when not enabled by default, and a risk
// signal off when the profile starts signals off (an automatic profile: a compliance person
// activates the signals they want).
func defaultMode(def *pack.Rule, c *store.Activation) string {
	if !def.Defaults.Enabled || (def.Kind == pack.KindSignal && c != nil && c.SignalDefault == SignalsOff) {
		return ModeOff
	}
	return string(def.Defaults.Mode)
}

// SignalsOff is the profile setting that starts every risk signal off.
const SignalsOff = "off"

// SystemProfileAuthor marks a compliance profile created automatically with its workspace.
const SystemProfileAuthor = "system:default"

// RuleConfig is a rule's configuration in the caller's environment.
type RuleConfig struct {
	RuleID string
	// Version is 0 while the rule runs with the library's defaults.
	Version int
	Mode    string
	// Settings are the values that differ from the library's defaults.
	Settings  map[string]float64
	Reason    string
	UpdatedAt *time.Time
	UpdatedBy string
	// Pending is the change awaiting a second person, if any.
	Pending *store.RuleChange
}

func configOf(def *pack.Rule, v *store.RuleVersion, c *store.Activation) RuleConfig {
	if v == nil {
		return RuleConfig{RuleID: def.ID, Mode: defaultMode(def, c), Settings: map[string]float64{}}
	}
	return RuleConfig{
		RuleID: def.ID, Version: v.Version, Mode: v.Mode, Settings: v.Settings, Reason: v.Reason,
		UpdatedAt: &v.CreatedAt, UpdatedBy: v.CreatedBy,
	}
}

// ConfigInput is a requested configuration.
type ConfigInput struct {
	Mode     string
	Settings map[string]float64
	Reason   string
	// RestoredFrom is set by a rollback.
	RestoredFrom *int
}

// ConfigResult is a configuration change: applied at once, or proposed for approval.
type ConfigResult struct {
	Applied  *RuleConfig
	Proposed *store.RuleChange
}

// integral reports whether a parameter type takes whole numbers only.
func integral(t pack.ParamType) bool {
	switch t {
	case pack.ParamMoney, pack.ParamCount, pack.ParamDuration, pack.ParamHour, pack.ParamPoints, pack.ParamFlag:
		return true
	}
	return false
}

// normalize checks a requested configuration against the rule's parameters and their bounds,
// and drops settings equal to the library's default, so "back to the default" is one state.
func normalize(def *pack.Rule, in ConfigInput) (ConfigInput, error) {
	if modeRank(in.Mode) < 0 {
		return in, fmt.Errorf("%w: mode must be live, shadow or off", ErrInvalidInput)
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len([]rune(in.Reason)) > 500 {
		return in, fmt.Errorf("%w: a reason of 1 to 500 characters is required", ErrInvalidInput)
	}
	out := map[string]float64{}
	for _, name := range slices.Sorted(maps.Keys(in.Settings)) {
		v := in.Settings[name]
		i := slices.IndexFunc(def.Parameters, func(p pack.Parameter) bool { return p.Name == name })
		if i < 0 {
			return in, fmt.Errorf("%w: %s has no setting %q", ErrInvalidInput, def.ID, name)
		}
		p := def.Parameters[i]
		switch {
		case math.IsNaN(v) || math.IsInf(v, 0):
			return in, fmt.Errorf("%w: %s must be a number", ErrInvalidInput, name)
		case v < p.Min || v > p.Max:
			return in, fmt.Errorf("%w: %s must be between %v and %v", ErrInvalidInput, name, p.Min, p.Max)
		case integral(p.Type) && v != math.Trunc(v):
			return in, fmt.Errorf("%w: %s must be a whole number", ErrInvalidInput, name)
		}
		if p.Default != nil && *p.Default == v {
			continue
		}
		out[name] = v
	}
	in.Settings = out
	return in, nil
}

// needsApproval reports whether a change needs a second person: in production, any settings
// change to a regulatory control (whether a setting loosens depends on how the rule uses it, so
// every change is checked) and any move of one away from live. Tightening the mode applies at
// once.
func needsApproval(scope tenancy.Scope, def *pack.Rule, cur RuleConfig, in ConfigInput) bool {
	if scope.EnvironmentKind != tenancy.EnvironmentProduction || def.Kind != pack.KindControl {
		return false
	}
	return !maps.Equal(cur.Settings, in.Settings) || modeRank(in.Mode) < modeRank(cur.Mode)
}

// person checks that the caller is a person holding p.
func person(ctx context.Context, p authz.Permission) (tenancy.Scope, error) {
	if err := authz.Require(ctx, p); err != nil {
		return tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return tenancy.Scope{}, err
	}
	if scope.Principal.Type != tenancy.PrincipalUser {
		return tenancy.Scope{}, authz.ErrForbidden
	}
	return scope, nil
}

// rule loads one of the tenant's rules, with its pack and configuration: in the jurisdiction
// the rule ID names (GH-REG-01), else in the fallback activation's pack.
func (s *Service) rule(ctx context.Context, scope tenancy.Scope, ruleID string) (library, *pack.Rule, RuleConfig, error) {
	return s.ruleIn(ctx, scope, JurisdictionOf(ruleID), ruleID)
}

// ruleFor is rule in a named jurisdiction, when one is given: an inherited rule (ATO-04) is in
// Nigeria's pack and in every pack that inherits it, each configured on its own.
func (s *Service) ruleFor(ctx context.Context, scope tenancy.Scope, jurisdiction, ruleID string) (library, *pack.Rule, RuleConfig, error) {
	if jurisdiction == "" {
		return s.rule(ctx, scope, ruleID)
	}
	return s.ruleIn(ctx, scope, jurisdiction, ruleID)
}

// placeholdersSet checks that a rule off until confirmed (off_reason, or settings the
// institution must enter) is switched on only with every such setting entered.
func placeholdersSet(def *pack.Rule, cur RuleConfig, in ConfigInput) error {
	if in.Mode == ModeOff || cur.Mode != ModeOff {
		return nil
	}
	var missing []string
	for _, p := range def.Parameters {
		if _, set := in.Settings[p.Name]; !set && p.Default == nil {
			missing = append(missing, p.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s is off until its settings are entered: set %s first", ErrInvalidInput, def.ID, strings.Join(missing, ", "))
	}
	return nil
}

// ruleIn loads one of the tenant's rules in a jurisdiction ("" is the fallback activation's),
// with its pack and configuration.
func (s *Service) ruleIn(ctx context.Context, scope tenancy.Scope, jurisdiction, ruleID string) (library, *pack.Rule, RuleConfig, error) {
	lib, err := s.load(ctx, scope, jurisdiction)
	if err != nil {
		return library{}, nil, RuleConfig{}, err
	}
	i := slices.IndexFunc(lib.pack.Rules, func(r pack.Rule) bool { return r.ID == ruleID })
	if i < 0 {
		return library{}, nil, RuleConfig{}, fmt.Errorf("%w: rule %s", ErrNotFound, ruleID)
	}
	def := &lib.pack.Rules[i]
	cur, err := s.store.CurrentVersions(ctx, lib.pack.ID)
	if err != nil {
		return library{}, nil, RuleConfig{}, err
	}
	var v *store.RuleVersion
	if c, ok := cur[ruleID]; ok {
		v = &c
	}
	cfg := configOf(def, v, lib.profile)
	pending, err := s.store.Changes(ctx, "pending", lib.pack.ID, ruleID)
	if err != nil {
		return library{}, nil, RuleConfig{}, err
	}
	if len(pending) > 0 {
		cfg.Pending = &pending[0]
	}
	return lib, def, cfg, nil
}

func (s *Service) recordRule(ctx context.Context, action, ruleID string, meta map[string]any) error {
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "pack_rule", ResourceID: ruleID, Outcome: audit.OutcomeSuccess, Metadata: meta,
	})
}

// Config returns a rule's configuration in the caller's environment. Requires compliance:read.
func (s *Service) Config(ctx context.Context, jurisdiction, ruleID string) (RuleConfig, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return RuleConfig{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return RuleConfig{}, err
	}
	var out RuleConfig
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		_, _, out, err = s.ruleFor(ctx, scope, jurisdiction, ruleID)
		return err
	})
	return out, err
}

// Configure sets a rule's mode and settings in the caller's environment: as a new version at
// once, or, when a second person must agree (needsApproval), as a pending change. Audited.
// Requires compliance:configure, held only by people.
func (s *Service) Configure(ctx context.Context, jurisdiction, ruleID string, in ConfigInput, now time.Time) (ConfigResult, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return ConfigResult{}, err
	}
	var out ConfigResult
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		lib, def, cur, err := s.ruleFor(ctx, scope, jurisdiction, ruleID)
		if err != nil {
			return err
		}
		if in, err = normalize(def, in); err != nil {
			return err
		}
		if err := placeholdersSet(def, cur, in); err != nil {
			return err
		}
		if in.Mode == cur.Mode && maps.Equal(in.Settings, cur.Settings) {
			// Nothing changes: no version, no proposal.
			out.Applied = &cur
			return nil
		}
		meta := map[string]any{
			"pack": lib.pack.ID + "@" + lib.pack.Version, "base_version": cur.Version, "reason": in.Reason,
			"mode_before": cur.Mode, "mode_after": in.Mode, "settings_before": cur.Settings, "settings_after": in.Settings,
		}
		if in.RestoredFrom != nil {
			meta["restored_from"] = *in.RestoredFrom
		}
		if needsApproval(scope, def, cur, in) {
			if cur.Pending != nil {
				return fmt.Errorf("%w: %s already has a change awaiting approval", ErrConflict, ruleID)
			}
			c := store.RuleChange{
				ID: ids.NewV7(), PackID: lib.pack.ID, RuleID: ruleID, BaseVersion: cur.Version, Mode: in.Mode,
				Settings: in.Settings, Reason: in.Reason, RestoredFrom: in.RestoredFrom, Status: "pending",
				ProposedBy: scope.Principal.ID, ProposedAt: now,
			}
			if err := s.store.InsertChange(ctx, c); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return fmt.Errorf("%w: %s already has a change awaiting approval", ErrConflict, ruleID)
				}
				return err
			}
			meta["change_id"] = ids.Format(ids.PrefixPackChange, c.ID)
			out.Proposed = &c
			return s.recordRule(ctx, "pack_rule.change_proposed", ruleID, meta)
		}
		v := store.RuleVersion{
			RuleID: ruleID, Version: cur.Version + 1, Mode: in.Mode, Settings: in.Settings, Reason: in.Reason,
			CreatedAt: now, CreatedBy: scope.Principal.ID, RestoredFrom: in.RestoredFrom,
		}
		if err := s.store.InsertVersion(ctx, lib.pack.ID, v); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return fmt.Errorf("%w: %s changed at the same time; try again", ErrConflict, ruleID)
			}
			return err
		}
		cfg := configOf(def, &v, nil)
		out.Applied = &cfg
		meta["version"] = v.Version
		return s.recordRule(ctx, "pack_rule.configured", ruleID, meta)
	})
	return out, err
}

// History returns a rule's configuration versions in the caller's environment, newest first.
// Requires compliance:read.
func (s *Service) History(ctx context.Context, jurisdiction, ruleID string) ([]store.RuleVersion, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.RuleVersion
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		lib, _, _, err := s.ruleFor(ctx, scope, jurisdiction, ruleID)
		if err != nil {
			return err
		}
		out, err = s.store.Versions(ctx, lib.pack.ID, ruleID)
		return err
	})
	return out, err
}

// Rollback restores a rule's earlier version as its next one, through Configure (so a
// rollback that could loosen a control in production also needs approval).
func (s *Service) Rollback(ctx context.Context, jurisdiction, ruleID string, version int, reason string, now time.Time) (ConfigResult, error) {
	versions, err := s.History(ctx, jurisdiction, ruleID)
	if err != nil {
		return ConfigResult{}, err
	}
	i := slices.IndexFunc(versions, func(v store.RuleVersion) bool { return v.Version == version })
	if i < 0 {
		return ConfigResult{}, fmt.Errorf("%w: %s has no version %d", ErrNotFound, ruleID, version)
	}
	v := versions[i]
	return s.Configure(ctx, jurisdiction, ruleID, ConfigInput{Mode: v.Mode, Settings: v.Settings, Reason: reason, RestoredFrom: &version}, now)
}

// Changes lists configuration changes in the caller's environment; status filters when set.
// Requires compliance:read.
func (s *Service) Changes(ctx context.Context, status string) ([]store.RuleChange, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return nil, err
	}
	var out []store.RuleChange
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.Changes(ctx, status, "", "")
		return err
	})
	return out, err
}

// Approve applies a pending change as the rule's next version, credited to its proposer. Only
// another person holding rules:approve may approve, and only while the rule is still at the
// version the change was proposed against. Audited.
func (s *Service) Approve(ctx context.Context, id uuid.UUID, note string, now time.Time) (store.RuleChange, error) {
	scope, err := person(ctx, authz.RulesApprove)
	if err != nil {
		return store.RuleChange{}, err
	}
	var out store.RuleChange
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		c, err := s.pending(ctx, id)
		if err != nil {
			return err
		}
		if c.ProposedBy == scope.Principal.ID {
			return ErrOwnChange
		}
		jurisdiction, err := s.jurisdictionOfPack(ctx, c.PackID)
		if err != nil {
			return err
		}
		_, _, cur, err := s.ruleIn(ctx, scope, jurisdiction, c.RuleID)
		if err != nil {
			return err
		}
		if cur.Version != c.BaseVersion {
			return fmt.Errorf("%w: %s changed since this change was proposed; reject it and propose again", ErrConflict, c.RuleID)
		}
		applied := c.BaseVersion + 1
		v := store.RuleVersion{
			RuleID: c.RuleID, Version: applied, Mode: c.Mode, Settings: c.Settings, Reason: c.Reason,
			CreatedAt: now, CreatedBy: c.ProposedBy, ChangeID: &c.ID, RestoredFrom: c.RestoredFrom,
		}
		if err := s.store.InsertVersion(ctx, c.PackID, v); err != nil {
			return err
		}
		n := noteOf(note)
		if err := s.store.Decide(ctx, c.ID, "approved", scope.Principal.ID, now, n, &applied); err != nil {
			return err
		}
		c.Status, c.DecidedBy, c.DecidedAt, c.DecisionNote, c.AppliedVersion = "approved", &scope.Principal.ID, &now, n, &applied
		out = c
		return s.recordRule(ctx, "pack_rule.change_approved", c.RuleID, map[string]any{
			"change_id": ids.Format(ids.PrefixPackChange, c.ID), "version": applied, "proposed_by": c.ProposedBy,
			"mode_after": c.Mode, "settings_after": c.Settings,
		})
	})
	return out, err
}

// Reject rejects a pending change: a person holding rules:approve, or its proposer withdrawing
// it. Audited.
func (s *Service) Reject(ctx context.Context, id uuid.UUID, note string, now time.Time) (store.RuleChange, error) {
	scope, err := person(ctx, authz.ComplianceRead)
	if err != nil {
		return store.RuleChange{}, err
	}
	var out store.RuleChange
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		c, err := s.pending(ctx, id)
		if err != nil {
			return err
		}
		if c.ProposedBy != scope.Principal.ID && authz.Require(ctx, authz.RulesApprove) != nil {
			return authz.ErrForbidden
		}
		n := noteOf(note)
		if err := s.store.Decide(ctx, c.ID, "rejected", scope.Principal.ID, now, n, nil); err != nil {
			return err
		}
		c.Status, c.DecidedBy, c.DecidedAt, c.DecisionNote = "rejected", &scope.Principal.ID, &now, n
		out = c
		return s.recordRule(ctx, "pack_rule.change_rejected", c.RuleID, map[string]any{
			"change_id": ids.Format(ids.PrefixPackChange, c.ID), "proposed_by": c.ProposedBy,
			"withdrawn": c.ProposedBy == scope.Principal.ID,
		})
	})
	return out, err
}

// jurisdictionOfPack returns the jurisdiction whose pack a change was made in.
func (s *Service) jurisdictionOfPack(ctx context.Context, packID string) (string, error) {
	list, err := s.store.Jurisdictions(ctx)
	if err != nil {
		return "", err
	}
	for _, j := range list {
		if j.PackID == packID {
			return j.Code, nil
		}
	}
	return "", fmt.Errorf("%w: pack %s", ErrNotFound, packID)
}

func (s *Service) pending(ctx context.Context, id uuid.UUID) (store.RuleChange, error) {
	c, err := s.store.Change(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.RuleChange{}, fmt.Errorf("%w: change", ErrNotFound)
	}
	if err != nil {
		return store.RuleChange{}, err
	}
	if c.Status != "pending" {
		return store.RuleChange{}, fmt.Errorf("%w: the change is already %s", ErrConflict, c.Status)
	}
	return c, nil
}

func noteOf(note string) *string {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	if r := []rune(note); len(r) > 500 {
		note = string(r[:500])
	}
	return &note
}
