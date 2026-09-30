package service

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`) //nolint:gochecknoglobals // compiled once, never mutated

// eventTypes are the event types a fail policy can be set for.
func eventTypes() []string {
	return []string{"login", "transaction", "beneficiary_added", "device_change", "session_change", "profile_change"}
}

// Settings returns the environment's risk settings (defaults when never set). Requires
// risk:configure.
func (s *Service) Settings(ctx context.Context) (domain.Settings, error) {
	if err := authz.Require(ctx, authz.RiskConfigure); err != nil {
		return domain.Settings{}, err
	}
	var out domain.Settings
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.GetSettings(ctx)
		return err
	})
	return out, err
}

// CatalogSettings returns the settings for displaying effective signal weights in the catalog.
// Requires rules:read (anyone who may read rules may see how signals are weighted).
func (s *Service) CatalogSettings(ctx context.Context) (domain.Settings, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return domain.Settings{}, err
	}
	var out domain.Settings
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.GetSettings(ctx)
		return err
	})
	return out, err
}

// Policy returns the environment's settings for displaying bands and decisions. Readable by
// anyone who can see scores (events, alerts, cases, rules or evaluations).
func (s *Service) Policy(ctx context.Context) (domain.Settings, error) {
	if err := authz.RequireAny(ctx, authz.EventsRead, authz.AlertsRead, authz.CasesRead, authz.RulesRead,
		authz.RiskEvaluate, authz.RiskConfigure); err != nil {
		return domain.Settings{}, err
	}
	var out domain.Settings
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		cfg, err := s.config.get(ctx, s.load)
		out = cfg.settings
		return err
	})
	return out, err
}

// PutSettings replaces the environment's risk settings. Weights are bounded (0 to
// domain.MaxSignalWeight) so a tenant can tune but never make a single signal decisive.
// Requires risk:configure; audited.
func (s *Service) PutSettings(ctx context.Context, in domain.Settings) (domain.Settings, error) {
	if err := authz.Require(ctx, authz.RiskConfigure); err != nil {
		return domain.Settings{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	if err := validateSettings(in); err != nil {
		return domain.Settings{}, err
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		before, err := s.store.GetSettings(ctx)
		if err != nil {
			return err
		}
		if err := s.store.PutSettings(ctx, in, scope.Principal.ID, s.clock.Now()); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "risk_settings.updated", ResourceType: "risk_settings", ResourceID: scope.EnvironmentID.String(),
			Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{
				"weights_before": before.Weights, "weights_after": in.Weights,
				"high_value_default_before": before.HighValueDefault, "high_value_default_after": in.HighValueDefault,
				"high_value_by_currency_after": in.HighValueByCurrency, "fail_policies_after": in.FailPolicies,
				"alert_min_decision_before": before.AlertMinDecision, "alert_min_decision_after": in.AlertMinDecision,
				"thresholds_before": before.Thresholds, "thresholds_after": in.Thresholds,
			},
		})
	})
	if err != nil {
		return domain.Settings{}, err
	}
	s.config.invalidate(ctx)
	return in, nil
}

func validateSettings(in domain.Settings) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", domain.ErrSettingsInvalid, fmt.Sprintf(format, args...))
	}
	for _, code := range slices.Sorted(maps.Keys(in.Weights)) {
		if _, ok := domain.BuiltinSignal(code); !ok {
			return invalid("unknown signal %q in weights", code)
		}
		if w := in.Weights[code]; w < 0 || w > domain.MaxSignalWeight {
			return invalid("weight of %s must be between 0 and %d", code, domain.MaxSignalWeight)
		}
	}
	if in.HighValueDefault <= 0 {
		return invalid("high_value_default must be positive")
	}
	for cur, v := range in.HighValueByCurrency {
		if !currencyPattern.MatchString(cur) {
			return invalid("%q is not an ISO 4217 currency code", cur)
		}
		if v <= 0 {
			return invalid("high-value threshold for %s must be positive", cur)
		}
	}
	if !in.Thresholds.Valid() {
		return invalid("thresholds must increase strictly: 1 <= low < elevated < high < critical <= 100")
	}
	if in.AlertMinDecision != domain.DecisionReview && in.AlertMinDecision != domain.DecisionStepUp {
		return invalid("alert_min_decision must be step_up or review")
	}
	for typ, p := range in.FailPolicies {
		if !slices.Contains(eventTypes(), typ) {
			return invalid("unknown event type %q in fail_policies", typ)
		}
		if p != domain.FailOpen && p != domain.FailClosed {
			return invalid("fail policy for %s must be open or closed", typ)
		}
	}
	return nil
}
