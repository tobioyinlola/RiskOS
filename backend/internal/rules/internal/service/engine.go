package service

import (
	"context"
	"errors"
	"slices"

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
}

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
		out = &TenantPack{Ref: p.ID + "@" + p.Version}
		for i := range p.Rules {
			def := &p.Rules[i]
			applies := applies(def, &c)
			// Settings overrides and per-tenant modes arrive with rule configuration (phase 4);
			// until then every rule runs with the library's defaults.
			if def.Kind == pack.KindGovernance || def.Trigger == nil || !def.Defaults.Enabled || applies == nil || !*applies {
				continue
			}
			out.rules = append(out.rules, tenantRule{r: interp.Compile(def, nil), mode: def.Defaults.Mode})
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
