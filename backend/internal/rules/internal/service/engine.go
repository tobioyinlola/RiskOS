package service

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// TenantPack is the rules of one of a tenant's activations that apply to it and are switched on,
// compiled with their settings, ready to evaluate (ADR-0006, ADR-0007).
type TenantPack struct {
	// Ref is "<pack>@<version>", recorded with every evaluation.
	Ref string
	// Jurisdiction is the pack's country; Location its time zone (windows, hours of the day).
	Jurisdiction string
	Location     *time.Location
	// LicenceType is the activation's institution type (the institution.licence_type fact).
	LicenceType string
	// DayStart is the hour a business day starts, for calendar windows.
	DayStart int
	rules    []tenantRule
	// rates are the institution's exchange rates: US dollars per one unit of a currency; ratesAt
	// when each was set.
	rates   map[string]float64
	ratesAt map[string]time.Time
}

// AmountUSD converts an amount to US dollars with the institution's rates; ok is false without
// a rate for its currency.
func (p *TenantPack) AmountUSD(m interp.Money) (interp.Money, bool) { return ToUSD(m, p.rates) }

// USDRate is the rate a conversion used, recorded with the evaluation (ADR-0007): US dollars per
// one unit of Currency, as set at AsOf.
type USDRate struct {
	Currency   string    `json:"currency"`
	USDPerUnit float64   `json:"usd_per_unit"`
	AsOf       time.Time `json:"as_of"`
}

// RateFor returns the rate for a currency; ok is false without one.
func (p *TenantPack) RateFor(currency string) (USDRate, bool) {
	r, ok := p.rates[currency]
	if !ok {
		return USDRate{}, false
	}
	return USDRate{Currency: currency, USDPerUnit: r, AsOf: p.ratesAt[currency]}, true
}

// Query returns the aggregation and window to ask a window store for, for an event at at. A
// calendar window runs from the start of the current local day or month (from the day
// boundary) to the event; days_over carries its threshold and the offset of the local day, as
// "days_over:<threshold>:<offset ms>".
func (p *TenantPack) Query(ref interp.AggRef, at time.Time) (fn string, window time.Duration) {
	fn, window = ref.Fn, time.Duration(ref.Window)*time.Second
	loc := p.Location
	if loc == nil {
		loc = time.UTC
	}
	local := at.In(loc)
	switch ref.Calendar {
	case pack.CalendarDay:
		start := time.Date(local.Year(), local.Month(), local.Day(), p.DayStart, 0, 0, 0, loc)
		if local.Before(start) {
			start = start.AddDate(0, 0, -1)
		}
		window = at.Sub(start)
	case pack.CalendarMonth:
		start := time.Date(local.Year(), local.Month(), 1, p.DayStart, 0, 0, 0, loc)
		if local.Before(start) {
			start = start.AddDate(0, -1, 0)
		}
		window = at.Sub(start)
	}
	if ref.Fn == "days_over" {
		_, offset := local.Zone()
		ms := int64(offset)*1000 - int64(p.DayStart)*3_600_000
		fn = fmt.Sprintf("days_over:%.0f:%d", ref.Over, ms)
	}
	return fn, window
}

type tenantRule struct {
	r    *interp.Rule
	mode pack.Mode
}

// How an event's governing jurisdiction was found (ADR-0007).
const (
	// ResolvedBookingEntity: the event named a booking entity the tenant has activated.
	ResolvedBookingEntity = "booking_entity"
	// ResolvedFallback: the event named none (or one the tenant has not activated) and the
	// tenant's stated fallback activation applies.
	ResolvedFallback = "fallback"
	// ResolvedNone: no jurisdiction could be resolved, so no pack judged the event.
	ResolvedNone = "unresolved"
	// ResolvedInactive: the event's booking entity is activated but in draft or suspended; no
	// other country's pack is used in its place.
	ResolvedInactive = "inactive"
)

// Resolution is which pack governs an event and why.
type Resolution struct {
	Pack *TenantPack
	// By is one of the Resolved constants.
	By            string
	BookingEntity string
}

// TenantPacks are a tenant's active activations compiled for evaluation.
type TenantPacks struct {
	byEntity map[string]*TenantPack
	// inactive are booking entities activated in draft or suspended.
	inactive map[string]bool
	fallback *TenantPack
}

// Resolve returns the pack that governs an event naming bookingEntity ("" when it names none).
// It never guesses: an entity the tenant has not activated, or no entity, falls back only to the
// tenant's stated fallback activation, and an entity in draft or suspended is not evaluated.
func (t *TenantPacks) Resolve(bookingEntity string) Resolution {
	if p, ok := t.byEntity[bookingEntity]; ok && bookingEntity != "" {
		return Resolution{Pack: p, By: ResolvedBookingEntity, BookingEntity: bookingEntity}
	}
	if t.inactive[bookingEntity] && bookingEntity != "" {
		return Resolution{By: ResolvedInactive, BookingEntity: bookingEntity}
	}
	if t.fallback != nil {
		return Resolution{Pack: t.fallback, By: ResolvedFallback, BookingEntity: bookingEntity}
	}
	return Resolution{By: ResolvedNone, BookingEntity: bookingEntity}
}

// Packs returns every compiled pack, the fallback first.
func (t *TenantPacks) Packs() []*TenantPack {
	var out []*TenantPack
	if t.fallback != nil {
		out = append(out, t.fallback)
	}
	for _, e := range slices.Sorted(maps.Keys(t.byEntity)) {
		if p := t.byEntity[e]; p != t.fallback {
			out = append(out, p)
		}
	}
	return out
}

// TenantPacks returns the tenant's compiled packs: one per activation in shadow or live (an
// activation in shadow runs every rule in shadow). nil when the tenant has no activation. It
// must run inside the caller's transaction; authorization is the caller's (the risk engine
// evaluating an event).
func (s *Service) TenantPacks(ctx context.Context) (*TenantPacks, error) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out *TenantPacks
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		acts, err := s.store.Activations(ctx, scope.TenantID)
		if err != nil || len(acts) == 0 {
			return err
		}
		rates, err := s.store.FXRates(ctx)
		if err != nil {
			return err
		}
		usd, at := map[string]float64{}, map[string]time.Time{}
		for _, r := range rates {
			usd[r.Currency], at[r.Currency] = r.USDPerUnit, r.UpdatedAt
		}
		out = &TenantPacks{byEntity: map[string]*TenantPack{}, inactive: map[string]bool{}}
		for i := range acts {
			a := &acts[i]
			if a.Status != "live" && a.Status != "shadow" {
				out.inactive[a.BookingEntity] = true
				continue
			}
			p, err := s.compile(ctx, a, usd, at)
			if err != nil {
				return err
			}
			out.byEntity[a.BookingEntity] = p
			if a.Fallback {
				out.fallback = p
			}
		}
		return nil
	})
	return out, err
}

// compile compiles one activation's pack with the environment's configuration.
func (s *Service) compile(ctx context.Context, a *store.Activation, usd map[string]float64, at map[string]time.Time) (*TenantPack, error) {
	rec, err := s.store.Pack(ctx, a.PackID, a.PackVersion)
	if err != nil {
		return nil, err
	}
	p, err := s.decoded(ctx, rec)
	if err != nil {
		return nil, err
	}
	configured, err := s.store.CurrentVersions(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(p.Locale().Timezone)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", p.ID, err)
	}
	out := &TenantPack{
		Ref: p.ID + "@" + p.Version, Jurisdiction: p.Jurisdiction, Location: loc, LicenceType: a.LicenceType,
		DayStart: int(a.DayStartsAt), rates: usd, ratesAt: at,
	}
	for i := range p.Rules {
		def := &p.Rules[i]
		applies := applies(def, a)
		if def.Kind == pack.KindGovernance || def.Trigger == nil || applies == nil || !*applies {
			continue
		}
		// The environment's configuration, or the library's defaults.
		mode, settings := defaultMode(def, a), map[string]float64(nil)
		if v, ok := configured[def.ID]; ok {
			mode, settings = v.Mode, v.Settings
		}
		if mode == ModeOff {
			continue
		}
		if a.Status == "shadow" {
			// A country in shadow is watched, never enforced.
			mode = ModeShadow
		}
		out.rules = append(out.rules, tenantRule{r: interp.Compile(def, settings), mode: pack.Mode(mode)})
	}
	return out, nil
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
	// Restricted marks a compliance-only signal (GOV-09): only compliance roles may see it.
	Restricted bool
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
			Severity: def.Outcome.Severity, Restricted: def.Outcome.Visibility == "compliance_only",
		}
		tasks := def.Outcome.Tasks
		// A control's matching branch may set its own decision and tasks (ADR-0007).
		if i := slices.IndexFunc(def.Branches, func(b pack.Branch) bool { return b.Label == o.Branch }); i >= 0 && def.Kind == pack.KindControl {
			if b := def.Branches[i]; b.Decision != "" || len(b.Tasks) > 0 {
				o.Decision, tasks = b.Decision, b.Tasks
			}
		}
		for _, task := range tasks {
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
		loc, err := time.LoadLocation(lib.pack.Locale().Timezone)
		if err != nil {
			return fmt.Errorf("pack %s: %w", lib.pack.ID, err)
		}
		licence, dayStart := "", 0
		if a := lib.profile; a != nil {
			licence, dayStart = a.LicenceType, int(a.DayStartsAt)
		}
		one := func(settings map[string]float64) *TenantPack {
			return &TenantPack{
				Ref: ref, Jurisdiction: lib.pack.Jurisdiction, Location: loc, LicenceType: licence, DayStart: dayStart,
				rules: []tenantRule{{r: interp.Compile(def, settings), mode: pack.ModeLive}},
			}
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
