package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

// Activation statuses (ADR-0007).
const (
	StatusDraft     = "draft"
	StatusShadow    = "shadow"
	StatusLive      = "live"
	StatusSuspended = "suspended"
)

// Activation change states.
const (
	ChangeApplied  = "applied"
	ChangePending  = "pending"
	ChangeApproved = "approved"
	ChangeRejected = "rejected"
)

var bookingEntityPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`) //nolint:gochecknoglobals // compiled once, never mutated

// ActivationInput is a new activation, or an activation's new settings (Jurisdiction and
// BookingEntity are fixed once created).
type ActivationInput struct {
	Jurisdiction, BookingEntity, LicenceType string
	Capabilities                             []string
	DayStartsAt                              int
	Rationale                                string
}

// ActivationResult is a change applied at once or proposed for a second person's approval.
type ActivationResult struct {
	Applied  *store.Activation
	Proposed *store.ActivationChange
}

// ReadinessItem is one thing to settle before an activation goes live.
type ReadinessItem struct {
	// Blocking items stop the activation going live; others are warnings.
	Blocking bool
	// Kind is licence, setting, fx_rate or open_question.
	Kind    string
	Rule    string
	Message string
}

// Readiness is the readiness check of an activation.
type Readiness struct {
	Ready bool
	Items []ReadinessItem
}

func rationaleOf(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > 1000 {
		return "", fmt.Errorf("%w: a rationale of 1 to 1,000 characters is required", ErrInvalidInput)
	}
	return s, nil
}

// checkSettings validates a licence type, capabilities and day start against a pack.
func checkSettings(p *pack.Pack, in *ActivationInput) error {
	if !has(p.LicenceTypes, in.LicenceType) {
		return fmt.Errorf("%w: licence_type %q is not one of the pack's licence types", ErrInvalidInput, in.LicenceType)
	}
	caps := slices.Clone(in.Capabilities)
	slices.Sort(caps)
	in.Capabilities = slices.Compact(caps)
	for _, c := range in.Capabilities {
		if !has(p.Capabilities, c) {
			return fmt.Errorf("%w: capability %q is not one of the pack's capabilities", ErrInvalidInput, c)
		}
	}
	if in.DayStartsAt < 0 || in.DayStartsAt > 23 {
		return fmt.Errorf("%w: day_starts_at must be an hour from 0 to 23", ErrInvalidInput)
	}
	if in.Capabilities == nil {
		in.Capabilities = []string{}
	}
	return nil
}

func (s *Service) activation(ctx context.Context, tenantID, id uuid.UUID) (store.Activation, error) {
	acts, err := s.store.Activations(ctx, tenantID)
	if err != nil {
		return store.Activation{}, err
	}
	i := slices.IndexFunc(acts, func(a store.Activation) bool { return a.ID == id })
	if i < 0 {
		return store.Activation{}, fmt.Errorf("%w: activation", ErrNotFound)
	}
	return acts[i], nil
}

func (s *Service) recordActivation(ctx context.Context, action string, a store.Activation, meta map[string]any) error {
	meta["jurisdiction"], meta["booking_entity"] = a.Jurisdiction, a.BookingEntity
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "jurisdiction_activation", ResourceID: ids.Format(ids.PrefixActivation, a.ID),
		Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: meta,
	})
}

// CreateActivation adds a draft activation of a country for a booking entity, on the country's
// latest pack. A draft judges no event. Audited. Requires compliance:configure (people only).
func (s *Service) CreateActivation(ctx context.Context, in ActivationInput, now time.Time) (store.Activation, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return store.Activation{}, err
	}
	in.Jurisdiction = strings.ToUpper(strings.TrimSpace(in.Jurisdiction))
	if !jurisdictionPattern.MatchString(in.Jurisdiction) {
		return store.Activation{}, fmt.Errorf("%w: jurisdiction must be a two-letter country code", ErrInvalidInput)
	}
	if !bookingEntityPattern.MatchString(in.BookingEntity) {
		return store.Activation{}, fmt.Errorf("%w: booking_entity must be 1 to 64 letters, digits or . _ : -", ErrInvalidInput)
	}
	rationale, err := rationaleOf(in.Rationale)
	if err != nil {
		return store.Activation{}, err
	}
	var out store.Activation
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		rec, err := s.store.LatestPack(ctx, in.Jurisdiction)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: there is no rule pack for jurisdiction %s", ErrInvalidInput, in.Jurisdiction)
		}
		if err != nil {
			return err
		}
		p, err := s.decoded(ctx, rec)
		if err != nil {
			return err
		}
		if err := checkSettings(p, &in); err != nil {
			return err
		}
		out, err = s.store.CreateActivation(ctx, store.ActivationInput{
			ID: ids.NewV7(), TenantID: scope.TenantID, Jurisdiction: in.Jurisdiction, BookingEntity: in.BookingEntity,
			PackID: p.ID, PackVersion: p.Version, LicenceType: in.LicenceType, Capabilities: in.Capabilities,
			DayStartsAt: int16(in.DayStartsAt), Rationale: &rationale, UpdatedAt: now, UpdatedBy: scope.Principal.ID, //nolint:gosec // checked 0 to 23
		})
		if errors.Is(err, store.ErrConflict) {
			return fmt.Errorf("%w: booking entity %s already has an activation", ErrConflict, in.BookingEntity)
		}
		if err != nil {
			return err
		}
		to := StatusDraft
		if err := s.store.InsertActivationChange(ctx, scope.TenantID, store.ActivationChange{
			ID: ids.NewV7(), ActivationID: out.ID, Action: "create", StatusTo: &to, Rationale: rationale,
			State: ChangeApplied, ProposedBy: scope.Principal.ID, ProposedAt: now,
			Details: map[string]any{"licence_type": in.LicenceType, "capabilities": in.Capabilities, "day_starts_at": in.DayStartsAt},
		}); err != nil {
			return err
		}
		return s.recordActivation(ctx, "jurisdiction_activation.created", out, map[string]any{
			"licence_type": in.LicenceType, "capabilities": in.Capabilities, "pack": p.ID + "@" + p.Version, "rationale": rationale,
		})
	})
	return out, err
}

// propose records a change: applied at once (apply runs) unless approval is needed, when it is
// kept pending for another person with rules:approve.
func (s *Service) propose(ctx context.Context, scope tenancy.Scope, a store.Activation, c store.ActivationChange, approval bool,
	apply func(ctx context.Context, approvedBy *string) (store.Activation, error),
) (ActivationResult, error) {
	c.ID, c.ActivationID, c.ProposedBy = ids.NewV7(), a.ID, scope.Principal.ID
	if approval {
		c.State = ChangePending
		if err := s.store.InsertActivationChange(ctx, scope.TenantID, c); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ActivationResult{}, fmt.Errorf("%w: this activation already has a change awaiting approval", ErrConflict)
			}
			return ActivationResult{}, err
		}
		return ActivationResult{Proposed: &c}, s.recordActivation(ctx, "jurisdiction_activation.change_proposed", a,
			map[string]any{"change_id": ids.Format(ids.PrefixActivationChange, c.ID), "action": c.Action, "status_to": c.StatusTo, "rationale": c.Rationale})
	}
	c.State = ChangeApplied
	out, err := apply(ctx, nil)
	if err != nil {
		return ActivationResult{}, err
	}
	if err := s.store.InsertActivationChange(ctx, scope.TenantID, c); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ActivationResult{}, fmt.Errorf("%w: this activation already has a change awaiting approval", ErrConflict)
		}
		return ActivationResult{}, err
	}
	return ActivationResult{Applied: &out}, s.recordActivation(ctx, "jurisdiction_activation.changed", out,
		map[string]any{"action": c.Action, "status_to": c.StatusTo, "rationale": c.Rationale, "details": c.Details})
}

// UpdateActivation changes an activation's licence type, capabilities and day start. On a live
// activation this can loosen its controls, so a second person approves it. Audited. Requires
// compliance:configure (people only).
func (s *Service) UpdateActivation(ctx context.Context, id uuid.UUID, in ActivationInput, now time.Time) (ActivationResult, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return ActivationResult{}, err
	}
	rationale, err := rationaleOf(in.Rationale)
	if err != nil {
		return ActivationResult{}, err
	}
	var out ActivationResult
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		a, err := s.activation(ctx, scope.TenantID, id)
		if err != nil {
			return err
		}
		rec, err := s.store.Pack(ctx, a.PackID, a.PackVersion)
		if err != nil {
			return err
		}
		p, err := s.decoded(ctx, rec)
		if err != nil {
			return err
		}
		if err := checkSettings(p, &in); err != nil {
			return err
		}
		c := store.ActivationChange{
			Action: "update", Rationale: rationale, ProposedAt: now,
			Details: map[string]any{"licence_type": in.LicenceType, "capabilities": in.Capabilities, "day_starts_at": in.DayStartsAt},
		}
		out, err = s.propose(ctx, scope, a, c, a.Status == StatusLive, func(ctx context.Context, _ *string) (store.Activation, error) {
			return s.applyUpdate(ctx, a, c.Details, scope.Principal.ID, now)
		})
		return err
	})
	return out, err
}

func (s *Service) applyUpdate(ctx context.Context, a store.Activation, d map[string]any, by string, now time.Time) (store.Activation, error) {
	in := store.ActivationInput{ID: a.ID, TenantID: a.TenantID, UpdatedAt: now, UpdatedBy: by}
	in.LicenceType, _ = d["licence_type"].(string)
	switch caps := d["capabilities"].(type) {
	case []string:
		in.Capabilities = caps
	case []any: // read back from JSON
		for _, c := range caps {
			if s, ok := c.(string); ok {
				in.Capabilities = append(in.Capabilities, s)
			}
		}
	}
	if in.Capabilities == nil {
		in.Capabilities = []string{}
	}
	switch h := d["day_starts_at"].(type) {
	case int:
		in.DayStartsAt = int16(h) //nolint:gosec // checked 0 to 23
	case float64:
		in.DayStartsAt = int16(h)
	}
	return s.store.UpdateActivation(ctx, in)
}

// statusNeedsApproval reports whether moving an activation between statuses needs a second
// person: going live, and leaving live (which stops or softens its controls).
func statusNeedsApproval(from, to string) bool {
	return to == StatusLive || from == StatusLive
}

// SetActivationStatus moves an activation through draft, shadow, live and suspended. Going live
// needs a passing readiness check; going live and leaving live need a second person. Audited.
// Requires compliance:configure (people only).
func (s *Service) SetActivationStatus(ctx context.Context, id uuid.UUID, status, rationale string, now time.Time) (ActivationResult, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return ActivationResult{}, err
	}
	if !slices.Contains([]string{StatusDraft, StatusShadow, StatusLive, StatusSuspended}, status) {
		return ActivationResult{}, fmt.Errorf("%w: status must be draft, shadow, live or suspended", ErrInvalidInput)
	}
	if rationale, err = rationaleOf(rationale); err != nil {
		return ActivationResult{}, err
	}
	var out ActivationResult
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		a, err := s.activation(ctx, scope.TenantID, id)
		if err != nil {
			return err
		}
		if a.Status == status {
			out.Applied = &a
			return nil
		}
		if a.Fallback && (status == StatusDraft || status == StatusSuspended) {
			return fmt.Errorf("%w: the fallback activation cannot be put in %s; make another activation the fallback first", ErrConflict, status)
		}
		if status == StatusLive {
			if err := s.mustBeReady(ctx, scope, a); err != nil {
				return err
			}
		}
		from := a.Status
		c := store.ActivationChange{Action: "status", StatusFrom: &from, StatusTo: &status, Rationale: rationale, ProposedAt: now}
		out, err = s.propose(ctx, scope, a, c, statusNeedsApproval(from, status), func(ctx context.Context, approvedBy *string) (store.Activation, error) {
			return s.store.SetActivationStatus(ctx, scope.TenantID, a.ID, status, rationale, scope.Principal.ID, approvedBy, now)
		})
		return err
	})
	return out, err
}

// MakeFallback makes an activation the one that judges events naming no booking entity. It
// changes which country's controls judge those events, so a second person approves it.
// Audited. Requires compliance:configure (people only).
func (s *Service) MakeFallback(ctx context.Context, id uuid.UUID, rationale string, now time.Time) (ActivationResult, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return ActivationResult{}, err
	}
	if rationale, err = rationaleOf(rationale); err != nil {
		return ActivationResult{}, err
	}
	var out ActivationResult
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		a, err := s.activation(ctx, scope.TenantID, id)
		if err != nil {
			return err
		}
		if a.Fallback {
			out.Applied = &a
			return nil
		}
		if a.Status != StatusLive && a.Status != StatusShadow {
			return fmt.Errorf("%w: only a live or shadow activation can be the fallback", ErrConflict)
		}
		c := store.ActivationChange{Action: "fallback", Rationale: rationale, ProposedAt: now}
		out, err = s.propose(ctx, scope, a, c, true, nil)
		return err
	})
	return out, err
}

// ActivationChanges lists the tenant's activation changes (the history of each activation and
// those awaiting approval), newest first. Requires compliance:read.
func (s *Service) ActivationChanges(ctx context.Context, activation *uuid.UUID, state string) ([]store.ActivationChange, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.ActivationChange
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		out, err = s.store.ActivationChanges(ctx, scope.TenantID, activation, state)
		return err
	})
	return out, err
}

// DecideActivationChange approves (applies) or rejects a pending activation change. Only
// another person holding rules:approve may decide it; going live is checked for readiness
// again. Audited.
func (s *Service) DecideActivationChange(ctx context.Context, id uuid.UUID, approve bool, note string, now time.Time) (store.ActivationChange, error) {
	scope, err := person(ctx, authz.RulesApprove)
	if err != nil {
		return store.ActivationChange{}, err
	}
	var out store.ActivationChange
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		c, err := s.store.ActivationChange(ctx, scope.TenantID, id)
		if err != nil {
			return err
		}
		if c.State != ChangePending {
			return fmt.Errorf("%w: the change is already %s", ErrConflict, c.State)
		}
		if c.ProposedBy == scope.Principal.ID {
			return ErrOwnChange
		}
		a, err := s.activation(ctx, scope.TenantID, c.ActivationID)
		if err != nil {
			return err
		}
		state, action := ChangeRejected, "jurisdiction_activation.change_rejected"
		if approve {
			state, action = ChangeApproved, "jurisdiction_activation.change_approved"
			if err := s.applyChange(ctx, scope, a, c, now); err != nil {
				return err
			}
		}
		if err := s.store.DecideActivationChange(ctx, scope.TenantID, id, state, scope.Principal.ID, noteOf(note), now); err != nil {
			return err
		}
		out = c
		out.State, out.DecidedBy, out.DecidedAt, out.DecisionNote = state, &scope.Principal.ID, &now, noteOf(note)
		return s.recordActivation(ctx, action, a, map[string]any{
			"change_id": ids.Format(ids.PrefixActivationChange, id), "action": c.Action, "proposed_by": c.ProposedBy, "note": note,
		})
	})
	return out, err
}

// applyChange applies an approved change, credited to its proposer and approver.
func (s *Service) applyChange(ctx context.Context, scope tenancy.Scope, a store.Activation, c store.ActivationChange, now time.Time) error {
	switch c.Action {
	case "status":
		if a.Status != *c.StatusFrom {
			return fmt.Errorf("%w: the activation changed since the proposal; propose again", ErrConflict)
		}
		if *c.StatusTo == StatusLive {
			if err := s.mustBeReady(ctx, scope, a); err != nil {
				return err
			}
		}
		_, err := s.store.SetActivationStatus(ctx, scope.TenantID, a.ID, *c.StatusTo, c.Rationale, c.ProposedBy, &scope.Principal.ID, now)
		return err
	case "fallback":
		_, err := s.store.SetFallback(ctx, scope.TenantID, a.ID, c.ProposedBy, now)
		return err
	case "update":
		_, err := s.applyUpdate(ctx, a, c.Details, c.ProposedBy, now)
		return err
	}
	return fmt.Errorf("%w: unknown change %s", ErrInvalidInput, c.Action)
}

func (s *Service) mustBeReady(ctx context.Context, scope tenancy.Scope, a store.Activation) error {
	r, err := s.readiness(ctx, scope, a)
	if err != nil {
		return err
	}
	if r.Ready {
		return nil
	}
	var blocking []string
	for _, it := range r.Items {
		if it.Blocking {
			blocking = append(blocking, it.Message)
		}
	}
	return fmt.Errorf("%w: not ready to go live: %s", ErrConflict, strings.Join(blocking, "; "))
}

// ActivationReadiness checks what must be settled before an activation goes live, in the
// caller's environment. Requires compliance:read.
func (s *Service) ActivationReadiness(ctx context.Context, id uuid.UUID) (Readiness, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return Readiness{}, err
	}
	var out Readiness
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		a, err := s.activation(ctx, scope.TenantID, id)
		if err != nil {
			return err
		}
		out, err = s.readiness(ctx, scope, a)
		return err
	})
	return out, err
}

// readiness lists, for the rules that apply to the activation and are not off in the caller's
// environment: settings the institution must enter (blocking for regulatory controls, which
// would otherwise not be evaluated), a missing exchange rate for a US dollar threshold
// (blocking for controls), and open questions in the library nobody has verified (warnings).
func (s *Service) readiness(ctx context.Context, scope tenancy.Scope, a store.Activation) (Readiness, error) {
	rec, err := s.store.Pack(ctx, a.PackID, a.PackVersion)
	if err != nil {
		return Readiness{}, err
	}
	p, err := s.decoded(ctx, rec)
	if err != nil {
		return Readiness{}, err
	}
	configured, err := s.store.CurrentVersions(ctx, p.ID)
	if err != nil {
		return Readiness{}, err
	}
	rates, err := s.store.FXRates(ctx)
	if err != nil {
		return Readiness{}, err
	}
	verified, err := s.store.GapVerifications(ctx, scope.TenantID, p.ID)
	if err != nil {
		return Readiness{}, err
	}
	currency := p.Locale().Currency
	hasRate := slices.ContainsFunc(rates, func(r store.FXRate) bool { return r.Currency == currency })
	items := []ReadinessItem{}
	if a.LicenceType == "other" {
		items = append(items, ReadinessItem{Blocking: true, Kind: "licence", Message: "Choose the institution's licence type"})
	}
	running := map[string]bool{}
	for i := range p.Rules {
		def := &p.Rules[i]
		if def.Kind == pack.KindGovernance || !*applies(def, &a) {
			continue
		}
		var v *store.RuleVersion
		if c, ok := configured[def.ID]; ok {
			v = &c
		}
		cfg := configOf(def, v, &a)
		if cfg.Mode == ModeOff {
			continue
		}
		running[def.ID] = true
		control := def.Kind == pack.KindControl
		usd := false
		for _, prm := range def.Parameters {
			if _, set := cfg.Settings[prm.Name]; !set && prm.Default == nil {
				items = append(items, ReadinessItem{
					Blocking: control, Kind: "setting", Rule: def.ID,
					Message: fmt.Sprintf("%s: enter %q (%s); until then it is not evaluated", def.ID, prm.Name, prm.Label),
				})
			}
			usd = usd || (prm.Type == pack.ParamMoney && prm.Currency == "USD")
		}
		if usd && !hasRate && currency != "USD" {
			items = append(items, ReadinessItem{
				Blocking: control, Kind: "fx_rate", Rule: def.ID,
				Message: fmt.Sprintf("%s: set a %s to US dollar rate, or send txn.amount_usd, for its dollar threshold", def.ID, currency),
			})
		}
	}
	for _, g := range p.Gaps {
		if _, ok := verified[g.ID]; ok {
			continue
		}
		var hit []string
		for _, r := range g.Rules {
			if running[r] {
				hit = append(hit, r)
			}
		}
		if len(hit) > 0 {
			items = append(items, ReadinessItem{
				Kind: "open_question", Rule: hit[0],
				Message: fmt.Sprintf("%s (%s) is not verified against the original text: %s", g.ID, g.Title, strings.Join(hit, ", ")),
			})
		}
	}
	ready := !slices.ContainsFunc(items, func(it ReadinessItem) bool { return it.Blocking })
	return Readiness{Ready: ready, Items: items}, nil
}

// VerifyGap records that the institution checked an open question of a jurisdiction's library
// against the original text: who, when, a note and an optional link. The rules it names show it.
// Audited. Requires compliance:configure (people only).
func (s *Service) VerifyGap(ctx context.Context, jurisdiction, gapID, note, link string, now time.Time) (store.GapVerification, error) {
	scope, err := person(ctx, authz.ComplianceConfigure)
	if err != nil {
		return store.GapVerification{}, err
	}
	note, link = strings.TrimSpace(note), strings.TrimSpace(link)
	if note == "" || len([]rune(note)) > 1000 {
		return store.GapVerification{}, fmt.Errorf("%w: a note of 1 to 1,000 characters is required", ErrInvalidInput)
	}
	if link != "" && (len(link) > 500 || (!strings.HasPrefix(link, "https://") && !strings.HasPrefix(link, "http://"))) {
		return store.GapVerification{}, fmt.Errorf("%w: link must be an http or https address of at most 500 characters", ErrInvalidInput)
	}
	var out store.GapVerification
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		lib, err := s.load(ctx, scope, jurisdiction)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(lib.pack.Gaps, func(g pack.Gap) bool { return g.ID == gapID })
		if i < 0 {
			return fmt.Errorf("%w: open question %s", ErrNotFound, gapID)
		}
		out = store.GapVerification{PackID: lib.pack.ID, GapID: gapID, VerifiedBy: scope.Principal.ID, VerifiedAt: now, Note: note}
		if link != "" {
			out.Link = &link
		}
		if err := s.store.VerifyGap(ctx, scope.TenantID, out); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return fmt.Errorf("%w: %s is already verified", ErrConflict, gapID)
			}
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "compliance_gap.verified", ResourceType: "compliance_gap", ResourceID: lib.pack.ID + ":" + gapID,
			Outcome: audit.OutcomeSuccess, TenantLevel: true,
			Metadata: map[string]any{"pack": lib.pack.ID, "rules": lib.pack.Gaps[i].Rules, "note": note, "link": link},
		})
	})
	return out, err
}
