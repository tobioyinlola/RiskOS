package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"golang.org/x/text/currency"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// maxUSDPerUnit bounds a rate (the database checks the same).
const maxUSDPerUnit = 1_000_000

// minorDigits returns how many minor-unit digits a currency has (2 for NGN and USD, 0 for JPY).
func minorDigits(code string) (int, bool) {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 0, false
	}
	scale, _ := currency.Standard.Rounding(unit)
	return scale, true
}

// ToUSD converts an amount with rates (US dollars per one unit of each currency): the result is
// in US cents, rounded to the nearest cent. ok is false without a rate for the currency.
func ToUSD(m interp.Money, rates map[string]float64) (interp.Money, bool) {
	if m.Currency == "USD" {
		return m, true
	}
	rate, ok := rates[m.Currency]
	if !ok {
		return interp.Money{}, false
	}
	digits, ok := minorDigits(m.Currency)
	if !ok {
		return interp.Money{}, false
	}
	major := m.Minor / math.Pow10(digits)
	return interp.Money{Minor: math.Round(major * rate * 100), Currency: "USD"}, true
}

// FXRates returns the tenant's exchange rates. Requires compliance:read.
func (s *Service) FXRates(ctx context.Context) ([]store.FXRate, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return nil, err
	}
	var out []store.FXRate
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.FXRates(ctx)
		return err
	})
	return out, err
}

func (s *Service) recordFX(ctx context.Context, action string, meta map[string]any) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, scope.TenantID),
		Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: meta,
	})
}

// SetFXRate sets US dollars per one unit of a currency. Audited. Requires compliance:write,
// held only by people.
func (s *Service) SetFXRate(ctx context.Context, code string, usdPerUnit float64, now time.Time) (store.FXRate, error) {
	scope, err := person(ctx, authz.ComplianceWrite)
	if err != nil {
		return store.FXRate{}, err
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if _, ok := minorDigits(code); !ok || code == "USD" {
		return store.FXRate{}, fmt.Errorf("%w: currency must be an ISO 4217 code other than USD", ErrInvalidInput)
	}
	if math.IsNaN(usdPerUnit) || usdPerUnit <= 0 || usdPerUnit > maxUSDPerUnit {
		return store.FXRate{}, fmt.Errorf("%w: usd_per_unit must be above 0 and at most %d", ErrInvalidInput, maxUSDPerUnit)
	}
	r := store.FXRate{Currency: code, USDPerUnit: usdPerUnit, UpdatedAt: now, UpdatedBy: scope.Principal.ID}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		before, err := s.store.FXRates(ctx)
		if err != nil {
			return err
		}
		meta := map[string]any{"currency": code, "usd_per_unit": usdPerUnit}
		for _, b := range before {
			if b.Currency == code {
				meta["previous_usd_per_unit"] = b.USDPerUnit
			}
		}
		if err := s.store.SetFXRate(ctx, r); err != nil {
			return err
		}
		return s.recordFX(ctx, "compliance.fx_rate_set", meta)
	})
	return r, err
}

// DeleteFXRate removes a currency's rate: rules on US dollar amounts are then not evaluated for
// it unless the event sends the amount. Audited. Requires compliance:write.
func (s *Service) DeleteFXRate(ctx context.Context, code string) error {
	if _, err := person(ctx, authz.ComplianceWrite); err != nil {
		return err
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	return s.runner.Run(ctx, func(ctx context.Context) error {
		if err := s.store.DeleteFXRate(ctx, code); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("%w: no rate for %s", ErrNotFound, code)
			}
			return err
		}
		return s.recordFX(ctx, "compliance.fx_rate_deleted", map[string]any{"currency": code})
	})
}
