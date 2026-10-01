// Package store is the only code that reads or writes the api_keys table.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Querier runs single-row queries outside a tenant transaction. It is satisfied by
// *pgxpool.Pool connected as the application role.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LookupResult is what authentication needs about a key, read before the tenant is known.
type LookupResult struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	EnvironmentID uuid.UUID
	Environment   tenancy.EnvironmentKind
	SecretHash    []byte
	Scopes        []string
	ExpiresAt     *time.Time
	RevokedAt     *time.Time
	TenantStatus  string
}

// Store reads and writes API keys.
type Store struct {
	pool Querier
}

// lookupTimeout bounds the pre-authentication lookups (API keys, console sessions). They run on
// every request, before the tenant is known; while the database is unreachable they fail fast
// with txn.ErrUnavailable instead of holding the request.
const lookupTimeout = time.Second

// preAuth bounds a pre-authentication query and classifies its failure.
func preAuth(ctx context.Context, what string, q func(ctx context.Context) error) error {
	qctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	err := q(qctx)
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if ctx.Err() == nil && (db.Unreachable(err) || qctx.Err() != nil) {
		return fmt.Errorf("%s: %w: %w", what, txn.ErrUnavailable, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}

// New returns a Store. pool is used only for Lookup; everything else runs in the tenant
// transaction carried by the context.
func New(pool Querier) Store { return Store{pool: pool} }

// Lookup finds a key by its public lookup value through the auth_lookup_api_key function, the
// only query that is not tenant-scoped (it runs before the tenant is known). It returns nil and
// no error when no key matches.
func (s Store) Lookup(ctx context.Context, lookup string) (*LookupResult, error) {
	var r LookupResult
	var env string
	err := preAuth(ctx, "look up api key", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`SELECT id, tenant_id, environment_id, environment_kind, secret_hash, scopes, expires_at, revoked_at, tenant_status
			 FROM auth_lookup_api_key($1)`, lookup,
		).Scan(&r.ID, &r.TenantID, &r.EnvironmentID, &env, &r.SecretHash, &r.Scopes, &r.ExpiresAt, &r.RevokedAt, &r.TenantStatus)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // "no such key" is a normal outcome, not an error
	}
	if err != nil {
		return nil, err
	}
	r.Environment = tenancy.EnvironmentKind(env)
	return &r, nil
}

func scoped(ctx context.Context) (*sqlcgen.Queries, tenancy.Scope, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	return sqlcgen.New(tx), scope, nil
}

// InsertParams describes a new key.
type InsertParams struct {
	ID         uuid.UUID
	Name       string
	Lookup     string
	SecretHash []byte
	Scopes     []string
	CreatedBy  string
	ExpiresAt  *time.Time
}

// Insert stores a new key in the current environment.
func (Store) Insert(ctx context.Context, p InsertParams) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertAPIKey(ctx, sqlcgen.InsertAPIKeyParams{
		ID: p.ID, TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, Name: p.Name,
		Lookup: p.Lookup, SecretHash: p.SecretHash, Scopes: p.Scopes, CreatedBy: p.CreatedBy, ExpiresAt: p.ExpiresAt,
	}); err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	return nil
}

// Get returns a key of the current environment, or domain.ErrNotFound.
func (Store) Get(ctx context.Context, id uuid.UUID) (domain.APIKey, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.APIKey{}, err
	}
	row, err := q.GetAPIKey(ctx, sqlcgen.GetAPIKeyParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id})
	if err != nil {
		return domain.APIKey{}, notFound(err, "get api key")
	}
	return toKey(scope, sqlcgen.ListAPIKeysRow(row)), nil
}

// List returns a newest-first page of the current environment's keys.
func (Store) List(ctx context.Context, p domain.Page) ([]domain.APIKey, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAPIKeys(ctx, sqlcgen.ListAPIKeysParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, BeforeID: p.Before,
		RowLimit: int32(p.Limit), //nolint:gosec // limit validated to 1..101
	})
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	out := make([]domain.APIKey, len(rows))
	for i, r := range rows {
		out[i] = toKey(scope, r)
	}
	return out, nil
}

// Revoke sets revoked_at if not already set and returns the key.
func (Store) Revoke(ctx context.Context, id uuid.UUID, at time.Time) (domain.APIKey, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.APIKey{}, err
	}
	row, err := q.RevokeAPIKey(ctx, sqlcgen.RevokeAPIKeyParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, RevokedAt: &at})
	if err != nil {
		return domain.APIKey{}, notFound(err, "revoke api key")
	}
	return toKey(scope, sqlcgen.ListAPIKeysRow(row)), nil
}

// MarkReplaced records that id was rotated into replacedBy and ends its validity at expiresAt.
// It returns domain.ErrConflict if the key was revoked or rotated concurrently.
func (Store) MarkReplaced(ctx context.Context, id, replacedBy uuid.UUID, expiresAt time.Time) (domain.APIKey, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.APIKey{}, err
	}
	row, err := q.MarkAPIKeyReplaced(ctx, sqlcgen.MarkAPIKeyReplacedParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: id, ReplacedBy: &replacedBy, ExpiresAt: &expiresAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.APIKey{}, domain.ErrConflict
	}
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("mark api key replaced: %w", err)
	}
	return toKey(scope, sqlcgen.ListAPIKeysRow(row)), nil
}

func notFound(err error, op string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}

func toKey(scope tenancy.Scope, r sqlcgen.ListAPIKeysRow) domain.APIKey {
	return domain.APIKey{
		ID: r.ID, Name: r.Name, Lookup: r.Lookup, Environment: scope.EnvironmentKind, Scopes: r.Scopes,
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt, ReplacedBy: r.ReplacedBy,
	}
}
