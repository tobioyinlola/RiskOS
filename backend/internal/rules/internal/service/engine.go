package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// TenantPack is the rules of a tenant's pack that apply to its profile and are switched on,
// compiled with their settings, ready to evaluate (ADR-0006).
type TenantPack struct {
	// Ref is "<pack>@<version>", recorded with every evaluation.
	Ref   string
	rules []tenantRule
	// rates are the institution's exchange rates: US dollars per one unit of a currency.
	rates map[string]float64
}

// AmountUSD converts an amount to US dollars with the institution's rates; ok is false without
// a rate for its currency.
func (p *TenantPack) AmountUSD(m interp.Money) (interp.Money, bool) { return ToUSD(m, p.rates) }

type tenantRule struct {
	r    *interp.Rule
	mode pack.Mode
}

// TenantPack returns the tenant's compiled pack, or nil when the tenant has no compliance
// profile (without one, RiskOS cannot know which rules apply). It must run inside the caller's
// transaction; authorization is the caller's (the risk engine evaluating an event).
func (s *Service) TenantPack(ctx context.Context) (*TenantPack, error) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out *TenantPack
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		c, err := s.store.Compliance(ctx, scope.TenantID)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		rec, err := s.store.Pack(ctx, c.PackID, c.PackVersion)
		if err != nil {
			return err
		}
		p, err := s.decoded(ctx, rec)
		if err != nil {
			return err
		}
		configured, err := s.store.CurrentVersions(ctx, p.ID)
		if err != nil {
			return err
		}
		rates, err := s.store.FXRates(ctx)
		if err != nil {
			return err
		}
		out = &TenantPack{Ref: p.ID + "@" + p.Version, rates: map[string]float64{}}
		for _, r := range rates {
			out.rates[r.Currency] = r.USDPerUnit
		}
		for i := range p.Rules {
			def := &p.Rules[i]
			applies := applies(def, &c)
			if def.Kind == pack.KindGovernance || def.Trigger == nil || applies == nil || !*applies {
				continue
			}
			// The environment's configuration, or the library's defaults.
			mode, settings := defaultMode(def), map[string]float64(nil)
			if v, ok := configured[def.ID]; ok {
				mode, settings = v.Mode, v.Settings
			}
			if mode == ModeOff {
				continue
			}
			out.rules = append(out.rules, tenantRule{r: interp.Compile(def, settings), mode: pack.Mode(mode)})
		}
		return nil
	})
	return out, err
}

// Windows returns the windows an event of these kinds is recorded in (every window of the pack
// over one of the kinds, whichever rule reads it) and the windows the rules it triggers read.
func (p *TenantPack) Windows(kinds []string) (record, read []interp.AggRef) {
	for _, t := range p.rules {
		triggered := t.r.Triggered(kinds)
		for _, a := range t.r.Aggregates() {
			if slices.Contains(kinds, a.Event) {
				record = append(record, a)
			}
			if triggered {
				read = append(read, a)
			}
		}
	}
	return record, read
}

// Outcome is one triggered rule's evaluation, with what the definition says it does.
type Outcome struct {
	interp.Result
	Name string
	Kind pack.Kind
	Mode pack.Mode
	// Decision is a control's action; MinDecision the least decision a signal requires.
	Decision, MinDecision string
	Reason                string
	Tasks                 []string
	Severity              string
}

// Evaluate evaluates every rule the event triggers, in rule order.
func (p *TenantPack) Evaluate(env interp.Env) []Outcome {
	var out []Outcome
	for _, t := range p.rules {
		if !t.r.Triggered(env.Kinds) {
			continue
		}
		def := t.r.Def
		o := Outcome{
			Result: interp.Evaluate(t.r, env), Name: def.Text.Name, Kind: def.Kind, Mode: t.mode,
			Decision: def.Outcome.Decision, MinDecision: def.Outcome.MinDecision, Reason: def.Outcome.Reason,
			Severity: def.Outcome.Severity,
		}
		for _, task := range def.Outcome.Tasks {
			// A task tied to another event (REG-14's escalation) is not this event's.
			if task.On == "" || slices.Contains(env.Kinds, task.On) {
				o.Tasks = append(o.Tasks, task.Type)
			}
		}
		out = append(out, o)
	}
	return out
}

// Candidates compiles one rule of the tenant's pack for a backtest, whatever its mode: with the
// given settings (checked against the pack's bounds) and with the environment's current
// configuration. nil settings mean the current configuration for both. Requires
// compliance:read.
func (s *Service) Candidates(ctx context.Context, ruleID string, settings map[string]float64) (candidate, current *TenantPack, err error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return nil, nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		lib, def, cur, err := s.rule(ctx, scope, ruleID)
		if err != nil {
			return err
		}
		if def.Kind == pack.KindGovernance || def.Trigger == nil {
			return fmt.Errorf("%w: %s is a workflow, not an event rule; it cannot be backtested", ErrInvalidInput, ruleID)
		}
		ref := lib.pack.ID + "@" + lib.pack.Version
		one := func(settings map[string]float64) *TenantPack {
			return &TenantPack{Ref: ref, rules: []tenantRule{{r: interp.Compile(def, settings), mode: pack.ModeLive}}}
		}
		current, candidate = one(cur.Settings), one(cur.Settings)
		if settings != nil {
			in, err := normalize(def, ConfigInput{Mode: ModeLive, Settings: settings, Reason: "backtest"})
			if err != nil {
				return err
			}
			candidate = one(in.Settings)
		}
		return nil
	})
	return candidate, current, err
}
