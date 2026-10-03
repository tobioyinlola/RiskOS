// Package engine produces an event's signals: built-in behavioral signals from the feature
// store plus the tenant's rules, all evaluated against the same facts.
package engine

import (
	"context"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
)

// Version identifies the engine (signal catalog, thresholds, score formula and policy). It is
// stored with every evaluation; changing any of those is a new version.
const Version = "risk-engine-2.0.0"

// FeatureSource reads, and for ingested events updates, behavioral features.
type FeatureSource interface {
	Observe(ctx context.Context, e features.Event, write bool) features.Values
}

// CompiledRule is a rule's current version, ready to evaluate.
type CompiledRule struct {
	Rule      domain.Rule
	Condition rules.Compiled
}

// Output is everything an evaluation derives, stored with it for reproducibility.
type Output struct {
	Signals      []domain.Signal
	Shadow       []domain.Signal
	Degraded     []string
	Warming      []string
	Facts        rules.Facts
	RuleVersions []string
	Values       features.Values
}

// Engine evaluates events.
type Engine struct {
	features FeatureSource
	windows  WindowSource
}

// New returns an Engine reading features from src and rule pack windows from ws.
func New(src FeatureSource, ws WindowSource) Engine { return Engine{features: src, windows: ws} }

// Evaluate derives signals for e. write is false for dry runs and rule tests, which must never
// change feature state. Rules are evaluated in the order given; disabled rules are skipped.
func (en Engine) Evaluate(ctx context.Context, e features.Event, write bool, s domain.Settings, rs []CompiledRule) Output {
	v := en.features.Observe(ctx, e, write)
	signals, degraded, warming := features.Signals(e, v, s)
	out := Output{Signals: signals, Degraded: degraded, Warming: warming, Facts: features.Facts(e, v), Values: v}
	for _, r := range rs {
		cur := r.Rule.Current
		if !cur.Enabled {
			continue
		}
		out.RuleVersions = append(out.RuleVersions, r.Rule.VersionRef())
		if !r.Condition.Match(out.Facts) {
			continue
		}
		sig := domain.Signal{Code: r.Rule.Code, Label: cur.Name, Weight: cur.Weight}
		if cur.Mode == domain.ModeShadow {
			out.Shadow = append(out.Shadow, sig)
			continue
		}
		if sig.Weight > 0 {
			out.Signals = append(out.Signals, sig)
		}
	}
	return out
}
