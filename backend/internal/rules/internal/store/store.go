// Package store is the only code that reads or writes the rule pack tables and tenant
// compliance profiles.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store/sqlcgen"
)

// Errors.
var (
	// ErrNotFound means the pack, version or profile does not exist.
	ErrNotFound = errors.New("not found")
	// ErrChecksumChanged means a pack version is already loaded with different content.
	ErrChecksumChanged = errors.New("a loaded pack version cannot change")
)

// Pool is the database pool (*pgxpool.Pool). Loading packs needs the schema owner.
type Pool interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PackRecord is a pack version as stored: metadata and rules as JSON.
type PackRecord struct {
	ID, Version, Jurisdiction, Name, Checksum string
	Meta                                      []byte
	LoadedAt                                  time.Time
	Rules                                     []RuleRecord
}

// RuleRecord is one stored rule: its machine definition and its library wording.
type RuleRecord struct {
	ID, Kind            string
	Definition, Wording []byte
}

// SavePack inserts a pack version with its rules in one transaction on pool. It returns false,
// without writing, when the version is already loaded with the same checksum, and
// ErrChecksumChanged when it is loaded with different content.
func SavePack(ctx context.Context, pool Pool, p PackRecord) (bool, error) {
	inserted := false
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		sum, err := q.PackChecksum(ctx, sqlcgen.PackChecksumParams{ID: p.ID, Version: p.Version})
		switch {
		case err == nil && sum == p.Checksum:
			return nil
		case err == nil:
			return fmt.Errorf("%w: %s %s is loaded with checksum %s, the files have %s",
				ErrChecksumChanged, p.ID, p.Version, sum, p.Checksum)
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("read pack checksum: %w", err)
		}
		if err := q.InsertPack(ctx, sqlcgen.InsertPackParams{
			ID: p.ID, Version: p.Version, Jurisdiction: p.Jurisdiction, Name: p.Name, Checksum: p.Checksum, Meta: p.Meta,
		}); err != nil {
			return fmt.Errorf("insert pack: %w", err)
		}
		for _, r := range p.Rules {
			if err := q.InsertPackRule(ctx, sqlcgen.InsertPackRuleParams{
				PackID: p.ID, PackVersion: p.Version, RuleID: r.ID, Kind: r.Kind, Definition: r.Definition, Wording: r.Wording,
			}); err != nil {
				return fmt.Errorf("insert rule %s: %w", r.ID, err)
			}
		}
		inserted = true
		return nil
	})
	return inserted, err
}

// Store operates inside the tenant transaction carried by the context.
type Store struct{}

func queries(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, err
	}
	return sqlcgen.New(tx), nil
}

func packRecord(r sqlcgen.RulePack) PackRecord {
	return PackRecord{
		ID: r.ID, Version: r.Version, Jurisdiction: r.Jurisdiction, Name: r.Name, Checksum: r.Checksum,
		Meta: r.Meta, LoadedAt: r.LoadedAt,
	}
}

// Pack returns a pack version's metadata, without its rules.
func (Store) Pack(ctx context.Context, id, version string) (PackRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return PackRecord{}, err
	}
	r, err := q.GetPack(ctx, sqlcgen.GetPackParams{ID: id, Version: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return PackRecord{}, ErrNotFound
	}
	if err != nil {
		return PackRecord{}, fmt.Errorf("get pack: %w", err)
	}
	return packRecord(r), nil
}

// LatestPack returns a jurisdiction's latest pack version, without its rules. Packs of different
// countries are never compared: their versions are unrelated.
func (Store) LatestPack(ctx context.Context, jurisdiction string) (PackRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return PackRecord{}, err
	}
	r, err := q.LatestPack(ctx, jurisdiction)
	if errors.Is(err, pgx.ErrNoRows) {
		return PackRecord{}, ErrNotFound
	}
	if err != nil {
		return PackRecord{}, fmt.Errorf("latest pack: %w", err)
	}
	return packRecord(r), nil
}

// LatestPacks returns the latest version of every jurisdiction's pack, by jurisdiction.
func (Store) LatestPacks(ctx context.Context) ([]PackRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.LatestPacks(ctx)
	if err != nil {
		return nil, fmt.Errorf("latest packs: %w", err)
	}
	out := make([]PackRecord, len(rows))
	for i, r := range rows {
		out[i] = PackRecord{
			ID: r.ID, Version: r.Version, Jurisdiction: r.Jurisdiction, Name: r.Name, Checksum: r.Checksum,
			Meta: r.Meta, LoadedAt: r.LoadedAt,
		}
	}
	return out, nil
}

// PackRules returns a pack version's rules, ordered by ID.
func (Store) PackRules(ctx context.Context, id, version string) ([]RuleRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPackRules(ctx, sqlcgen.ListPackRulesParams{PackID: id, PackVersion: version})
	if err != nil {
		return nil, fmt.Errorf("list pack rules: %w", err)
	}
	out := make([]RuleRecord, len(rows))
	for i, r := range rows {
		out[i] = RuleRecord{ID: r.RuleID, Definition: r.Definition, Wording: r.Wording}
	}
	return out, nil
}

// Activation is one of a tenant's jurisdiction activations (ADR-0007): the pack it is on for one
// country and booking entity, what kind of institution that entity is, and its status. The
// fallback activation is the tenant's compliance profile: events that name no booking entity are
// judged by it.
type Activation struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	Jurisdiction  string
	BookingEntity string
	// Status is draft, shadow, live or suspended.
	Status       string
	Fallback     bool
	PackID       string
	PackVersion  string
	LicenceType  string
	Capabilities []string
	// SignalDefault is how risk signals start without a configuration: "pack" (the pack's
	// defaults) or "off".
	SignalDefault string
	EffectiveFrom *time.Time
	ActivatedAt   *time.Time
	ActivatedBy   *string
	Rationale     *string
	ApprovedBy    *string
	UpdatedAt     time.Time
	UpdatedBy     string
	// DayStartsAt is the hour a business day starts in the jurisdiction (calendar windows).
	DayStartsAt int16
}

// Jurisdiction is a row of the jurisdictions reference table.
type Jurisdiction = sqlcgen.Jurisdiction

// EnsureProfiles gives every workspace without an activation the automatic one, on pool
// connected as the schema owner, and returns how many it created.
func EnsureProfiles(ctx context.Context, pool Pool) (int64, error) {
	n, err := sqlcgen.New(pool).EnsureComplianceProfiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("ensure compliance profiles: %w", err)
	}
	return n, nil
}

// CheckJurisdiction refuses a pack whose jurisdiction is not in the reference table, or whose
// currency or time zone differs from its row. It runs on pool as the schema owner.
func CheckJurisdiction(ctx context.Context, pool Pool, code, currency, timezone string) error {
	j, err := sqlcgen.New(pool).GetJurisdiction(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("jurisdiction %s is not in the jurisdictions table: add its row in a migration", code)
	}
	if err != nil {
		return fmt.Errorf("get jurisdiction: %w", err)
	}
	if j.Currency != currency || j.Timezone != timezone {
		return fmt.Errorf("jurisdiction %s is %s in %s, the pack says %s in %s", code, j.Currency, j.Timezone, currency, timezone)
	}
	return nil
}

// Jurisdictions returns the reference table, by code.
func (Store) Jurisdictions(ctx context.Context) ([]Jurisdiction, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListJurisdictions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list jurisdictions: %w", err)
	}
	return out, nil
}

// Activations returns tenantID's activations, the fallback first.
func (Store) Activations(ctx context.Context, tenantID uuid.UUID) ([]Activation, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListActivations(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list activations: %w", err)
	}
	out := make([]Activation, len(rows))
	for i, r := range rows {
		out[i] = Activation(r)
	}
	return out, nil
}

// SaveFallback sets the tenant's fallback activation (its compliance profile): the pack,
// jurisdiction, licence type and capabilities. It creates the activation if there is none.
func (Store) SaveFallback(ctx context.Context, a Activation) (Activation, error) {
	q, err := queries(ctx)
	if err != nil {
		return Activation{}, err
	}
	r, err := q.UpdateFallbackActivation(ctx, sqlcgen.UpdateFallbackActivationParams{
		TenantID: a.TenantID, Jurisdiction: a.Jurisdiction, PackID: a.PackID, PackVersion: a.PackVersion,
		LicenceType: a.LicenceType, Capabilities: a.Capabilities, UpdatedAt: a.UpdatedAt, UpdatedBy: a.UpdatedBy,
	})
	if err == nil {
		return Activation(r), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Activation{}, fmt.Errorf("save compliance profile: %w", err)
	}
	ins, err := q.InsertFallbackActivation(ctx, sqlcgen.InsertFallbackActivationParams{
		ID: a.ID, TenantID: a.TenantID, Jurisdiction: a.Jurisdiction, PackID: a.PackID, PackVersion: a.PackVersion,
		LicenceType: a.LicenceType, Capabilities: a.Capabilities, UpdatedAt: a.UpdatedAt, UpdatedBy: a.UpdatedBy,
	})
	if err != nil {
		return Activation{}, fmt.Errorf("create compliance profile: %w", err)
	}
	return Activation(ins), nil
}
