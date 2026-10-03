package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/store"
)

// Onboarding errors.
var (
	// ErrNoOnboarding means the tenant has no onboarding (created by an operator).
	ErrNoOnboarding = errors.New("this workspace has no onboarding")
	// ErrOnboardingState means the onboarding is not in a state that allows the operation.
	ErrOnboardingState = errors.New("onboarding cannot change in its current state")
)

// Onboarding statuses.
const (
	OnboardingDraft     = "draft"
	OnboardingSubmitted = "submitted"
	OnboardingApproved  = "approved"
	OnboardingRejected  = "rejected"
)

// Answers are a workspace's know-your-business answers (docs/CONSOLE.md#onboarding). Every field
// is optional while drafting; Missing lists what submitting needs.
type Answers struct {
	LegalName              string   `json:"legal_name,omitempty"`
	TradingName            string   `json:"trading_name,omitempty"`
	Country                string   `json:"country,omitempty"`
	RegistrationNumber     string   `json:"registration_number,omitempty"`
	Website                string   `json:"website,omitempty"`
	Industry               string   `json:"industry,omitempty"`
	Regulated              *bool    `json:"regulated,omitempty"`
	Regulator              string   `json:"regulator,omitempty"`
	LicenceNumber          string   `json:"licence_number,omitempty"`
	AMLProgram             *bool    `json:"aml_program,omitempty"`
	ComplianceOfficerName  string   `json:"compliance_officer_name,omitempty"`
	ComplianceOfficerEmail string   `json:"compliance_officer_email,omitempty"`
	ScreeningProvider      string   `json:"screening_provider,omitempty"`
	MonitoredEvents        []string `json:"monitored_events,omitempty"`
	MonthlyVolume          string   `json:"monthly_volume,omitempty"`
	RegionsServed          []string `json:"regions_served,omitempty"`
	GoLive                 string   `json:"go_live,omitempty"`
	DataProtection         []string `json:"data_protection,omitempty"`
	DataResidency          string   `json:"data_residency,omitempty"`
	DPOEmail               string   `json:"dpo_email,omitempty"`
	AcceptTerms            bool     `json:"accept_terms,omitempty"`
	AcceptDPA              bool     `json:"accept_dpa,omitempty"`
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`) //nolint:gochecknoglobals // compiled once, never mutated

//nolint:gochecknoglobals // fixed vocabularies, never mutated
var (
	industries      = []string{"payments", "lending", "banking", "crypto", "marketplace", "insurance", "gaming", "other"}
	monitoredEvents = []string{"payments", "payouts", "transfers", "logins", "signups", "card_transactions"}
	volumes         = []string{"under_10k", "10k_100k", "100k_1m", "1m_10m", "over_10m"}
	goLives         = []string{"now", "within_3_months", "within_6_months", "later"}
	privacyRegimes  = []string{"gdpr", "uk_gdpr", "ndpr", "popia", "ccpa", "lgpd", "other"}
	residencies     = []string{"eu", "uk", "us", "africa", "no_preference"}
)

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && len(s) <= 254
}

// normalize trims every text answer and checks each present answer's form. It does not require
// anything: a draft may be incomplete.
func (a *Answers) normalize() error {
	text := []struct {
		v    *string
		name string
		max  int
	}{
		{&a.LegalName, "legal_name", 200}, {&a.TradingName, "trading_name", 200},
		{&a.RegistrationNumber, "registration_number", 100}, {&a.Website, "website", 300},
		{&a.Regulator, "regulator", 200}, {&a.LicenceNumber, "licence_number", 100},
		{&a.ComplianceOfficerName, "compliance_officer_name", 100},
		{&a.ComplianceOfficerEmail, "compliance_officer_email", 254},
		{&a.ScreeningProvider, "screening_provider", 200}, {&a.DPOEmail, "dpo_email", 254},
	}
	for _, f := range text {
		*f.v = strings.TrimSpace(*f.v)
		if utf8.RuneCountInString(*f.v) > f.max {
			return fmt.Errorf("%w: %s must be at most %d characters", ErrInvalidInput, f.name, f.max)
		}
	}
	a.ComplianceOfficerEmail = strings.ToLower(a.ComplianceOfficerEmail)
	a.DPOEmail = strings.ToLower(a.DPOEmail)
	a.Country = strings.ToUpper(strings.TrimSpace(a.Country))
	switch {
	case a.Country != "" && !countryPattern.MatchString(a.Country):
		return fmt.Errorf("%w: country must be a two-letter ISO code", ErrInvalidInput)
	case a.Website != "" && !validWebsite(a.Website):
		return fmt.Errorf("%w: website must be an http or https address", ErrInvalidInput)
	case a.Industry != "" && !oneOf(a.Industry, industries):
		return fmt.Errorf("%w: industry is not one of the listed values", ErrInvalidInput)
	case a.ComplianceOfficerEmail != "" && !validEmail(a.ComplianceOfficerEmail):
		return fmt.Errorf("%w: compliance_officer_email is not a valid address", ErrInvalidInput)
	case a.DPOEmail != "" && !validEmail(a.DPOEmail):
		return fmt.Errorf("%w: dpo_email is not a valid address", ErrInvalidInput)
	case a.MonthlyVolume != "" && !oneOf(a.MonthlyVolume, volumes):
		return fmt.Errorf("%w: monthly_volume is not one of the listed values", ErrInvalidInput)
	case a.GoLive != "" && !oneOf(a.GoLive, goLives):
		return fmt.Errorf("%w: go_live is not one of the listed values", ErrInvalidInput)
	case a.DataResidency != "" && !oneOf(a.DataResidency, residencies):
		return fmt.Errorf("%w: data_residency is not one of the listed values", ErrInvalidInput)
	}
	for _, list := range []struct {
		v       []string
		name    string
		allowed []string
	}{{a.MonitoredEvents, "monitored_events", monitoredEvents}, {a.DataProtection, "data_protection", privacyRegimes}} {
		seen := map[string]bool{}
		for _, v := range list.v {
			if !oneOf(v, list.allowed) || seen[v] {
				return fmt.Errorf("%w: %s has an unknown or repeated value", ErrInvalidInput, list.name)
			}
			seen[v] = true
		}
	}
	if len(a.RegionsServed) > 60 {
		return fmt.Errorf("%w: regions_served lists at most 60 countries", ErrInvalidInput)
	}
	seen := map[string]bool{}
	for i, r := range a.RegionsServed {
		r = strings.ToUpper(strings.TrimSpace(r))
		if !countryPattern.MatchString(r) || seen[r] {
			return fmt.Errorf("%w: regions_served must be distinct two-letter ISO codes", ErrInvalidInput)
		}
		seen[r] = true
		a.RegionsServed[i] = r
	}
	return nil
}

func validWebsite(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

// Missing lists, by field name, the answers submitting still needs.
func (a Answers) Missing() []string {
	out := []string{}
	need := func(ok bool, name string) {
		if !ok {
			out = append(out, name)
		}
	}
	need(a.LegalName != "", "legal_name")
	need(a.Country != "", "country")
	need(a.RegistrationNumber != "", "registration_number")
	need(a.Website != "", "website")
	need(a.Industry != "", "industry")
	need(a.Regulated != nil, "regulated")
	if a.Regulated != nil && *a.Regulated {
		need(a.Regulator != "", "regulator")
		need(a.LicenceNumber != "", "licence_number")
	}
	need(a.AMLProgram != nil, "aml_program")
	need(a.ComplianceOfficerName != "", "compliance_officer_name")
	need(a.ComplianceOfficerEmail != "", "compliance_officer_email")
	need(len(a.MonitoredEvents) > 0, "monitored_events")
	need(a.MonthlyVolume != "", "monthly_volume")
	need(len(a.RegionsServed) > 0, "regions_served")
	need(a.GoLive != "", "go_live")
	need(len(a.DataProtection) > 0, "data_protection")
	need(a.DataResidency != "", "data_residency")
	need(a.AcceptTerms, "accept_terms")
	need(a.AcceptDPA, "accept_dpa")
	return out
}

// OnboardingView is an onboarding as the console and the admin realm show it.
type OnboardingView struct {
	TenantID          uuid.UUID
	TenantName        string
	Status            string
	Answers           Answers
	Missing           []string
	ProductionEnabled bool
	UpdatedAt         time.Time
	SubmittedAt       *time.Time
	SubmittedBy       *string
	ReviewedAt        *time.Time
	ReviewedBy        *string
	ReviewNote        *string
}

func view(o store.Onboarding) (OnboardingView, error) {
	var a Answers
	if err := json.Unmarshal(o.Answers, &a); err != nil {
		return OnboardingView{}, fmt.Errorf("decode onboarding answers: %w", err)
	}
	return OnboardingView{
		TenantID: o.TenantID, TenantName: o.TenantName, Status: o.Status, Answers: a, Missing: a.Missing(),
		ProductionEnabled: o.ProductionEnabled, UpdatedAt: o.UpdatedAt, SubmittedAt: o.SubmittedAt,
		SubmittedBy: o.SubmittedBy, ReviewedAt: o.ReviewedAt, ReviewedBy: o.ReviewedBy, ReviewNote: o.ReviewNote,
	}, nil
}

// load reads the scope tenant's onboarding in the transaction of ctx.
func (s *Service) load(ctx context.Context, tenantID uuid.UUID, forUpdate bool) (OnboardingView, error) {
	o, err := s.store.Onboarding(ctx, tenantID, forUpdate)
	if errors.Is(err, store.ErrNotFound) {
		return OnboardingView{}, ErrNoOnboarding
	}
	if err != nil {
		return OnboardingView{}, err
	}
	return view(o)
}

// customer checks that the caller is a console user with permission p and returns the scope.
func customer(ctx context.Context, p authz.Permission) (tenancy.Scope, error) {
	if err := authz.Require(ctx, p); err != nil {
		return tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return tenancy.Scope{}, err
	}
	if scope.Principal.Type != tenancy.PrincipalUser {
		// Onboarding is done in the console, never with an API key.
		return tenancy.Scope{}, authz.ErrForbidden
	}
	return scope, nil
}

func (s *Service) recordOnboarding(ctx context.Context, action string, meta map[string]any) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, scope.TenantID),
		Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: meta,
	})
}

// Onboarding returns the caller's workspace onboarding. Requires team:read.
func (s *Service) Onboarding(ctx context.Context) (OnboardingView, error) {
	scope, err := customer(ctx, authz.TeamRead)
	if err != nil {
		return OnboardingView{}, err
	}
	var out OnboardingView
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		out, err = s.load(ctx, scope.TenantID, false)
		return err
	})
	return out, err
}

// SaveOnboarding replaces the answers of a draft or rejected onboarding. Requires team:write.
func (s *Service) SaveOnboarding(ctx context.Context, a Answers, now time.Time) (OnboardingView, error) {
	scope, err := customer(ctx, authz.TeamWrite)
	if err != nil {
		return OnboardingView{}, err
	}
	if err := a.normalize(); err != nil {
		return OnboardingView{}, err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return OnboardingView{}, fmt.Errorf("encode onboarding answers: %w", err)
	}
	var out OnboardingView
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.load(ctx, scope.TenantID, true)
		if err != nil {
			return err
		}
		if cur.Status != OnboardingDraft && cur.Status != OnboardingRejected {
			return fmt.Errorf("%w: it was %s", ErrOnboardingState, cur.Status)
		}
		if err := s.store.SaveOnboardingAnswers(ctx, scope.TenantID, raw, scope.Principal.ID, now); err != nil {
			return err
		}
		if err := s.recordOnboarding(ctx, "onboarding.saved", map[string]any{"missing": len(a.Missing())}); err != nil {
			return err
		}
		out, err = s.load(ctx, scope.TenantID, false)
		return err
	})
	return out, err
}

// SubmitOnboarding sends a complete draft (or a corrected rejected one) for review. Requires
// team:write.
func (s *Service) SubmitOnboarding(ctx context.Context, now time.Time) (OnboardingView, error) {
	scope, err := customer(ctx, authz.TeamWrite)
	if err != nil {
		return OnboardingView{}, err
	}
	var out OnboardingView
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.load(ctx, scope.TenantID, true)
		if err != nil {
			return err
		}
		if cur.Status != OnboardingDraft && cur.Status != OnboardingRejected {
			return fmt.Errorf("%w: it was %s", ErrOnboardingState, cur.Status)
		}
		if len(cur.Missing) > 0 {
			return fmt.Errorf("%w: answer %s first", ErrInvalidInput, strings.Join(cur.Missing, ", "))
		}
		if err := s.store.SubmitOnboarding(ctx, scope.TenantID, scope.Principal.ID, now); err != nil {
			return err
		}
		if err := s.recordOnboarding(ctx, "onboarding.submitted", nil); err != nil {
			return err
		}
		out, err = s.load(ctx, scope.TenantID, false)
		return err
	})
	return out, err
}

// StaffOnboarding returns a tenant's onboarding for RiskOS staff. ctx must be the tenant scope the
// admin module obtained with StaffScope; reading is recorded in the tenant's audit log.
func (s *Service) StaffOnboarding(ctx context.Context) (OnboardingView, error) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return OnboardingView{}, err
	}
	var out OnboardingView
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		out, err = s.load(ctx, scope.TenantID, false)
		if err != nil {
			return err
		}
		return s.recordOnboarding(ctx, "onboarding.viewed_by_staff", nil)
	})
	return out, err
}

// ReviewOnboarding records a staff decision on a submitted onboarding: approving opens the
// tenant's production environment; rejecting needs a note the customer will see. ctx must be the
// tenant scope the admin module obtained with StaffScope.
func (s *Service) ReviewOnboarding(ctx context.Context, approve bool, note string, now time.Time) (OnboardingView, error) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return OnboardingView{}, err
	}
	note = strings.TrimSpace(note)
	if n := utf8.RuneCountInString(note); n > 1000 || (!approve && n < 10) {
		return OnboardingView{}, fmt.Errorf("%w: give the customer a note of 10 to 1000 characters", ErrInvalidInput)
	}
	status, action := OnboardingApproved, "onboarding.approved"
	if !approve {
		status, action = OnboardingRejected, "onboarding.rejected"
	}
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	var out OnboardingView
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		cur, err := s.load(ctx, scope.TenantID, true)
		if err != nil {
			return err
		}
		if cur.Status != OnboardingSubmitted {
			return fmt.Errorf("%w: it was %s", ErrOnboardingState, cur.Status)
		}
		if err := s.store.ReviewOnboarding(ctx, scope.TenantID, status, scope.Principal.ID, notePtr, now); err != nil {
			return err
		}
		if err := s.recordOnboarding(ctx, action, map[string]any{"note": note, "production_enabled": approve}); err != nil {
			return err
		}
		out, err = s.load(ctx, scope.TenantID, false)
		return err
	})
	return out, err
}
