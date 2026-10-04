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

// LatestPack returns the most recently loaded pack version, of jurisdiction when it is not
// empty, without its rules.
func (Store) LatestPack(ctx context.Context, jurisdiction string) (PackRecord, error) {
	q, err := queries(ctx)
	if err != nil {
		return PackRecord{}, err
	}
	var j *string
	if jurisdiction != "" {
		j = &jurisdiction
	}
	r, err := q.LatestPack(ctx, j)
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

// Compliance is a tenant's compliance profile.
type Compliance struct {
	TenantID     uuid.UUID
	PackID       string
	PackVersion  string
	LicenceType  string
	Capabilities []string
	UpdatedAt    time.Time
	UpdatedBy    string
	// SignalDefault is how risk signals start without a configuration: "pack" (the pack's
	// defaults) or "off".
	SignalDefault string
}

// EnsureProfiles gives every workspace without a compliance profile the automatic one, on pool
// connected as the schema owner, and returns how many it created.
func EnsureProfiles(ctx context.Context, pool Pool) (int64, error) {
	n, err := sqlcgen.New(pool).EnsureComplianceProfiles(ctx)
	if err != nil {
		return 0, fmt.Errorf("ensure compliance profiles: %w", err)
	}
	return n, nil
}

// Compliance returns tenantID's compliance profile.
func (Store) Compliance(ctx context.Context, tenantID uuid.UUID) (Compliance, error) {
	q, err := queries(ctx)
	if err != nil {
		return Compliance{}, err
	}
	r, err := q.GetCompliance(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Compliance{}, ErrNotFound
	}
	if err != nil {
		return Compliance{}, fmt.Errorf("get compliance profile: %w", err)
	}
	return Compliance(r), nil
}

// SaveCompliance creates or replaces a tenant's compliance profile.
func (Store) SaveCompliance(ctx context.Context, c Compliance) (Compliance, error) {
	q, err := queries(ctx)
	if err != nil {
		return Compliance{}, err
	}
	r, err := q.UpsertCompliance(ctx, sqlcgen.UpsertComplianceParams{
		TenantID: c.TenantID, PackID: c.PackID, PackVersion: c.PackVersion, LicenceType: c.LicenceType,
		Capabilities: c.Capabilities, UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy,
	})
	if err != nil {
		return Compliance{}, fmt.Errorf("save compliance profile: %w", err)
	}
	return Compliance(r), nil
}
