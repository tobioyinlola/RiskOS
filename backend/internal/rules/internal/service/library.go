package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// Errors.
var (
	// ErrNotFound means there is no such rule, no pack loaded, or no compliance profile.
	ErrNotFound = errors.New("not found")
	// ErrInvalidInput means a request is malformed; the message says why.
	ErrInvalidInput = errors.New("invalid input")
)

// Service serves the rules library: the pack a tenant is on and its compliance profile.
type Service struct {
	runner txn.Runner
	audit  audit.Recorder
	store  store.Store

	// packs caches decoded pack versions by "id@version". A loaded version never changes (the
	// database refuses updates), so entries never go stale.
	mu    sync.Mutex
	packs map[string]*pack.Pack

	// alerts and cases are read by the compliance workflow (nil until connected).
	alerts AlertSource
	cases  CaseSource
}

// New returns the service.
func New(runner txn.Runner, rec audit.Recorder) *Service {
	return &Service{runner: runner, audit: rec, packs: map[string]*pack.Pack{}}
}

// LoadPacks validates each built-in pack and stores every version not yet loaded, on pool
// connected as the schema owner. Loading is idempotent; a loaded version whose files have
// changed is an error (publish a new version instead).
func LoadPacks(ctx context.Context, pool store.Pool, packs []fs.FS, logger *slog.Logger) error {
	for _, fsys := range packs {
		p, err := pack.Load(fsys)
		if err != nil {
			return fmt.Errorf("load rule pack: %w", err)
		}
		rec, err := record(p)
		if err != nil {
			return err
		}
		inserted, err := store.SavePack(ctx, pool, rec)
		if err != nil {
			return err
		}
		logger.InfoContext(ctx, "rule pack", slog.String("pack", p.ID), slog.String("version", p.Version),
			slog.Int("rules", len(p.Rules)), slog.Bool("loaded", inserted), slog.String("checksum", p.Checksum))
	}
	created, err := store.EnsureProfiles(ctx, pool)
	if err != nil {
		return err
	}
	if created > 0 {
		logger.InfoContext(ctx, "compliance profiles created", slog.Int64("workspaces", created))
	}
	return nil
}

func record(p *pack.Pack) (store.PackRecord, error) {
	meta, err := json.Marshal(p)
	if err != nil {
		return store.PackRecord{}, fmt.Errorf("encode pack %s: %w", p.ID, err)
	}
	rec := store.PackRecord{
		ID: p.ID, Version: p.Version, Jurisdiction: p.Jurisdiction, Name: p.Name, Checksum: p.Checksum, Meta: meta,
	}
	for _, r := range p.Rules {
		def, err := json.Marshal(r)
		if err != nil {
			return store.PackRecord{}, fmt.Errorf("encode rule %s: %w", r.ID, err)
		}
		wording, err := json.Marshal(r.Text)
		if err != nil {
			return store.PackRecord{}, fmt.Errorf("encode wording of %s: %w", r.ID, err)
		}
		rec.Rules = append(rec.Rules, store.RuleRecord{ID: r.ID, Kind: string(r.Kind), Definition: def, Wording: wording})
	}
	return rec, nil
}

// PackView is a loaded pack version.
type PackView struct {
	*pack.Pack
	LoadedAt time.Time
	// Latest is the newest loaded version of the pack's jurisdiction: saving the compliance
	// profile adopts it.
	Latest string
}

// Profile is a tenant's compliance profile.
type Profile struct {
	Jurisdiction string
	LicenceType  string
	Capabilities []string
	PackID       string
	PackVersion  string
	UpdatedAt    time.Time
	UpdatedBy    string
	// Confirmed is false for the automatic profile nobody has saved yet: its institution type
	// ("other") is a placeholder.
	Confirmed bool
}

// RuleView is a rule as the caller may see it.
type RuleView struct {
	pack.Rule
	// Applicable says whether the rule applies to the tenant's profile; nil without a profile.
	Applicable *bool
	// Engineering is true when the caller may see the engineering logic and machine definition;
	// when false, Text.EngineeringLogic is empty.
	Engineering bool
	// Config is the rule's configuration in the caller's environment.
	Config RuleConfig
}

// Value returns a setting's value in the caller's environment: configured, else the default;
// nil when the institution has not set one.
func (v *RuleView) Value(p pack.Parameter) *float64 {
	if x, ok := v.Config.Settings[p.Name]; ok {
		return &x
	}
	return p.Default
}

// Filter narrows a rule list. Empty fields match everything.
type Filter struct {
	Group, Kind, Regulator, Verification, Query string
	Applicable                                  *bool
	// Jurisdiction browses another jurisdiction's pack; empty means the workspace's own.
	Jurisdiction string
}

// library is the tenant's pack and, if set, its profile.
type library struct {
	pack    PackView
	profile *store.Compliance
	// configured are the rules configured in the caller's environment, by ID.
	configured map[string]store.RuleVersion
}

// load returns the pack on the tenant's profile, or the most recently loaded pack without one.
// It must run inside the runner.
func (s *Service) load(ctx context.Context, tenant tenancy.Scope) (library, error) {
	var lib library
	c, err := s.store.Compliance(ctx, tenant.TenantID)
	var rec store.PackRecord
	switch {
	case err == nil:
		lib.profile = &c
		rec, err = s.store.Pack(ctx, c.PackID, c.PackVersion)
	case errors.Is(err, store.ErrNotFound):
		rec, err = s.store.LatestPack(ctx, "")
	}
	if errors.Is(err, store.ErrNotFound) {
		return library{}, fmt.Errorf("%w: no rule pack is loaded", ErrNotFound)
	}
	if err != nil {
		return library{}, err
	}
	p, err := s.decoded(ctx, rec)
	if err != nil {
		return library{}, err
	}
	lib.pack = PackView{Pack: p, LoadedAt: rec.LoadedAt, Latest: p.Version}
	if latest, err := s.store.LatestPack(ctx, p.Jurisdiction); err == nil {
		lib.pack.Latest = latest.Version
	} else if !errors.Is(err, store.ErrNotFound) {
		return library{}, err
	}
	if lib.configured, err = s.store.CurrentVersions(ctx, p.ID); err != nil {
		return library{}, err
	}
	return lib, nil
}

func (s *Service) decoded(ctx context.Context, rec store.PackRecord) (*pack.Pack, error) {
	key := rec.ID + "@" + rec.Version
	s.mu.Lock()
	p, ok := s.packs[key]
	s.mu.Unlock()
	if ok && p.Checksum == rec.Checksum {
		return p, nil
	}
	rules, err := s.store.PackRules(ctx, rec.ID, rec.Version)
	if err != nil {
		return nil, err
	}
	p = &pack.Pack{}
	if err := json.Unmarshal(rec.Meta, p); err != nil {
		return nil, fmt.Errorf("decode pack %s: %w", key, err)
	}
	p.Checksum = rec.Checksum
	p.Rules = make([]pack.Rule, len(rules))
	for i, r := range rules {
		if err := json.Unmarshal(r.Definition, &p.Rules[i]); err != nil {
			return nil, fmt.Errorf("decode rule %s of %s: %w", r.ID, key, err)
		}
		if err := json.Unmarshal(r.Wording, &p.Rules[i].Text); err != nil {
			return nil, fmt.Errorf("decode wording of %s of %s: %w", r.ID, key, err)
		}
	}
	s.mu.Lock()
	s.packs[key] = p
	s.mu.Unlock()
	return p, nil
}

func (s *Service) read(ctx context.Context) (library, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return library{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return library{}, err
	}
	var lib library
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		lib, err = s.load(ctx, scope)
		return err
	})
	return lib, err
}

// Pack returns the tenant's pack or, with a jurisdiction, that jurisdiction's latest pack to
// browse. Requires compliance:read.
func (s *Service) Pack(ctx context.Context, jurisdiction string) (PackView, error) {
	lib, err := s.browse(ctx, jurisdiction)
	return lib.pack, err
}

// browse is the tenant's library or, for another jurisdiction, that jurisdiction's latest pack
// read-only: no profile (applicability is unknown) and the library's default modes.
func (s *Service) browse(ctx context.Context, jurisdiction string) (library, error) {
	lib, err := s.read(ctx)
	if err != nil || jurisdiction == "" || lib.pack.Jurisdiction == jurisdiction {
		return lib, err
	}
	var other library
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		rec, err := s.store.LatestPack(ctx, jurisdiction)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: no rule pack is loaded for %s", ErrNotFound, jurisdiction)
		}
		if err != nil {
			return err
		}
		p, err := s.decoded(ctx, rec)
		if err != nil {
			return err
		}
		other = library{
			pack:       PackView{Pack: p, LoadedAt: rec.LoadedAt, Latest: p.Version},
			configured: map[string]store.RuleVersion{},
		}
		return nil
	})
	return other, err
}

// PackSummary is the latest version of one jurisdiction's pack.
type PackSummary struct {
	ID, Jurisdiction, Name, Version string
	Counts                          map[pack.Kind]int
	// Current is the pack the workspace's profile uses.
	Current bool
}

// Packs lists the latest pack of every loaded jurisdiction. Requires compliance:read.
func (s *Service) Packs(ctx context.Context) ([]PackSummary, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out []PackSummary
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		profile, err := s.store.Compliance(ctx, scope.TenantID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		recs, err := s.store.LatestPacks(ctx)
		if err != nil {
			return err
		}
		for _, rec := range recs {
			p, err := s.decoded(ctx, rec)
			if err != nil {
				return err
			}
			sum := PackSummary{
				ID: p.ID, Jurisdiction: p.Jurisdiction, Name: p.Name, Version: p.Version,
				Counts: map[pack.Kind]int{}, Current: profile.PackID == p.ID,
			}
			for _, r := range p.Rules {
				sum.Counts[r.Kind]++
			}
			out = append(out, sum)
		}
		return nil
	})
	return out, err
}

// applies reports whether r applies to profile c: its licence type is listed (or the rule
// applies to every institution) and every capability the rule requires is on.
func applies(r *pack.Rule, c *store.Compliance) *bool {
	if c == nil {
		return nil
	}
	ok := (len(r.AppliesTo) == 0 || slices.Contains(r.AppliesTo, c.LicenceType))
	for _, req := range r.Requires {
		ok = ok && slices.Contains(c.Capabilities, req)
	}
	return &ok
}

func (lib *library) view(r *pack.Rule, engineering bool) RuleView {
	var cfg *store.RuleVersion
	if c, ok := lib.configured[r.ID]; ok {
		cfg = &c
	}
	v := RuleView{Rule: *r, Applicable: applies(r, lib.profile), Engineering: engineering, Config: configOf(r, cfg, lib.profile)}
	if !engineering {
		v.Text.EngineeringLogic = ""
	}
	return v
}

func (f Filter) match(v *RuleView) bool {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	switch {
	case f.Group != "" && v.Group != f.Group,
		f.Kind != "" && string(v.Kind) != f.Kind,
		f.Regulator != "" && !slices.Contains(v.Regulators, f.Regulator),
		f.Verification != "" && string(v.Verification) != f.Verification,
		f.Applicable != nil && *v.Applicable != *f.Applicable:
		return false
	case q == "":
		return true
	default:
		return strings.Contains(strings.ToLower(v.ID+"\n"+v.Text.Name+"\n"+v.Text.Description), q)
	}
}

// Rules returns the pack's rules that match f, ordered by ID, without engineering logic.
// Requires compliance:read.
func (s *Service) Rules(ctx context.Context, f Filter) ([]RuleView, error) {
	lib, err := s.browse(ctx, f.Jurisdiction)
	if err != nil {
		return nil, err
	}
	if f.Applicable != nil && lib.profile == nil {
		return nil, fmt.Errorf("%w: set the workspace's compliance profile to filter by what applies", ErrInvalidInput)
	}
	out := []RuleView{}
	for i := range lib.pack.Rules {
		v := lib.view(&lib.pack.Rules[i], false)
		if f.match(&v) {
			out = append(out, v)
		}
	}
	return out, nil
}

// Rule returns one rule, with its engineering logic when the caller holds
// compliance:engineering. Requires compliance:read.
func (s *Service) Rule(ctx context.Context, id string) (RuleView, error) {
	lib, err := s.read(ctx)
	if err != nil {
		return RuleView{}, err
	}
	i := slices.IndexFunc(lib.pack.Rules, func(r pack.Rule) bool { return r.ID == id })
	if i < 0 {
		return RuleView{}, fmt.Errorf("%w: rule %s", ErrNotFound, id)
	}
	engineering := authz.Require(ctx, authz.ComplianceEngineering) == nil
	return lib.view(&lib.pack.Rules[i], engineering), nil
}

func profileView(c store.Compliance, jurisdiction string) Profile {
	return Profile{
		Jurisdiction: jurisdiction, LicenceType: c.LicenceType, Capabilities: c.Capabilities,
		PackID: c.PackID, PackVersion: c.PackVersion, UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy,
		Confirmed: c.UpdatedBy != SystemProfileAuthor,
	}
}

// Profile returns the tenant's compliance profile, or ErrNotFound. Requires compliance:read.
func (s *Service) Profile(ctx context.Context) (Profile, error) {
	lib, err := s.read(ctx)
	if err != nil {
		return Profile{}, err
	}
	if lib.profile == nil {
		return Profile{}, fmt.Errorf("%w: no compliance profile is set", ErrNotFound)
	}
	return profileView(*lib.profile, lib.pack.Jurisdiction), nil
}

// ProfileInput is a new compliance profile.
type ProfileInput struct {
	Jurisdiction string
	LicenceType  string
	Capabilities []string
}

var jurisdictionPattern = regexp.MustCompile(`^[A-Z]{2}$`) //nolint:gochecknoglobals // compiled once, never mutated

func has(opts []pack.Licence, id string) bool {
	return slices.ContainsFunc(opts, func(o pack.Licence) bool { return o.ID == id })
}

// SaveProfile sets the tenant's compliance profile. The tenant keeps its pack version unless
// the jurisdiction changes, which moves it to that jurisdiction's latest pack. Audited.
// Requires compliance:write, held only by people.
func (s *Service) SaveProfile(ctx context.Context, in ProfileInput, now time.Time) (Profile, error) {
	if err := authz.Require(ctx, authz.ComplianceWrite); err != nil {
		return Profile{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return Profile{}, err
	}
	if scope.Principal.Type != tenancy.PrincipalUser {
		return Profile{}, authz.ErrForbidden
	}
	in.Jurisdiction = strings.ToUpper(strings.TrimSpace(in.Jurisdiction))
	if !jurisdictionPattern.MatchString(in.Jurisdiction) {
		return Profile{}, fmt.Errorf("%w: jurisdiction must be a two-letter country code", ErrInvalidInput)
	}
	caps := slices.Clone(in.Capabilities)
	slices.Sort(caps)
	caps = slices.Compact(caps)
	var out Profile
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		lib, err := s.load(ctx, scope)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		// Saving the profile adopts the jurisdiction's latest pack version; rule configuration
		// carries over (it is kept per pack, not per version).
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
		if !has(p.LicenceTypes, in.LicenceType) {
			return fmt.Errorf("%w: licence_type %q is not one of the pack's licence types", ErrInvalidInput, in.LicenceType)
		}
		for _, c := range caps {
			if !has(p.Capabilities, c) {
				return fmt.Errorf("%w: capability %q is not one of the pack's capabilities", ErrInvalidInput, c)
			}
		}
		saved, err := s.store.SaveCompliance(ctx, store.Compliance{
			TenantID: scope.TenantID, PackID: p.ID, PackVersion: p.Version, LicenceType: in.LicenceType,
			Capabilities: caps, UpdatedAt: now, UpdatedBy: scope.Principal.ID,
		})
		if err != nil {
			return err
		}
		meta := map[string]any{
			"jurisdiction": in.Jurisdiction, "licence_type": in.LicenceType, "capabilities": caps,
			"pack": p.ID, "pack_version": p.Version,
		}
		if lib.profile != nil {
			meta["previous"] = map[string]any{
				"jurisdiction": lib.pack.Jurisdiction, "licence_type": lib.profile.LicenceType,
				"capabilities": lib.profile.Capabilities, "pack_version": lib.profile.PackVersion,
			}
		}
		out = profileView(saved, p.Jurisdiction)
		return s.audit.Record(ctx, audit.Entry{
			Action: "compliance_profile.updated", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, scope.TenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: meta,
		})
	})
	return out, err
}
