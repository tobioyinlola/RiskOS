package service

import (
	"context"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// JurisdictionView is a jurisdiction and whether the tenant has activated it (ADR-0007).
type JurisdictionView struct {
	store.Jurisdiction
	Activated bool
}

// Jurisdictions lists the jurisdictions RiskOS has a pack for. Requires compliance:read.
func (s *Service) Jurisdictions(ctx context.Context) ([]JurisdictionView, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return nil, err
	}
	var out []JurisdictionView
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		list, err := s.store.Jurisdictions(ctx)
		if err != nil {
			return err
		}
		acts, err := s.store.Activations(ctx, scope.TenantID)
		if err != nil {
			return err
		}
		for _, j := range list {
			active := slices.ContainsFunc(acts, func(a store.Activation) bool { return a.Jurisdiction == j.Code })
			out = append(out, JurisdictionView{Jurisdiction: j, Activated: active})
		}
		return nil
	})
	return out, err
}

// Activations lists the tenant's activations, the fallback first. Requires compliance:read.
func (s *Service) Activations(ctx context.Context) ([]store.Activation, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.Activation
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		out, err = s.store.Activations(ctx, scope.TenantID)
		return err
	})
	return out, err
}

// ActiveRule is a pack rule running for one of the tenant's activations in the caller's
// environment, with what an integration must send for it to be evaluated.
type ActiveRule struct {
	Rule          *pack.Rule
	Jurisdiction  string
	Pack          string
	BookingEntity string
	// Mode is live or shadow.
	Mode string
	// Facts and Refs are the compliance facts and references it reads; UnsetSettings the
	// settings without a value, which leave it not evaluated.
	Facts, Refs, UnsetSettings []string
}

// ActiveRules lists every rule of every activation in shadow or live that applies to it and is
// not off in the caller's environment, optionally of one jurisdiction. Governance presets are
// processes, not event rules, and are left out. Requires compliance:read.
func (s *Service) ActiveRules(ctx context.Context, jurisdiction string) ([]ActiveRule, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return nil, err
	}
	out := []ActiveRule{}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		acts, err := s.store.Activations(ctx, scope.TenantID)
		if err != nil {
			return err
		}
		for i := range acts {
			a := &acts[i]
			if (a.Status != "live" && a.Status != "shadow") || (jurisdiction != "" && a.Jurisdiction != jurisdiction) {
				continue
			}
			rec, err := s.store.Pack(ctx, a.PackID, a.PackVersion)
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
			for j := range p.Rules {
				def := &p.Rules[j]
				if ok := applies(def, a); def.Trigger == nil || !*ok {
					continue
				}
				var v *store.RuleVersion
				if c, ok := configured[def.ID]; ok {
					v = &c
				}
				cfg := configOf(def, v, a)
				if cfg.Mode == ModeOff {
					continue
				}
				if a.Status == "shadow" {
					cfg.Mode = ModeShadow
				}
				facts, entities := def.Uses()
				var refs, unset []string
				for _, e := range entities {
					if slices.Contains(pack.RefEntities(), e) {
						refs = append(refs, e)
					}
				}
				for _, prm := range def.Parameters {
					if _, set := cfg.Settings[prm.Name]; !set && prm.Default == nil {
						unset = append(unset, prm.Name)
					}
				}
				out = append(out, ActiveRule{
					Rule: def, Jurisdiction: a.Jurisdiction, Pack: p.ID + "@" + p.Version, BookingEntity: a.BookingEntity,
					Mode: cfg.Mode, Facts: facts, Refs: refs, UnsetSettings: unset,
				})
			}
		}
		return nil
	})
	return out, err
}

func readScope(ctx context.Context) (tenancy.Scope, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return tenancy.Scope{}, err
	}
	return tenancy.FromContext(ctx)
}
