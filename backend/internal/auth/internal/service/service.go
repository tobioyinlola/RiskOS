// Package service implements API key authentication and management.
package service

import (
	"context"
	"crypto/hmac"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/appmetrics"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Limits on API key management input.
const (
	maxNameLength      = 100
	maxScopes          = 20
	DefaultOverlap     = 24 * time.Hour
	MaxOverlap         = 7 * 24 * time.Hour
	maxCachedLookups   = 10_000
	tenantStatusActive = "active"
)

// Config configures the service.
type Config struct {
	// Pepper keys the HMAC of stored secrets. At least 32 bytes, from the secrets manager.
	Pepper []byte
	// PepperPrevious is, during a rotation (ADR-0008), the pepper secrets were hashed with
	// before Pepper. A key that verifies against it is accepted and rehashed under Pepper.
	// Empty outside a rotation.
	PepperPrevious []byte
	// CacheTTL bounds how long a key lookup is reused, and so how long a revoked key can keep
	// working on an instance (ARCHITECTURE.md §9: at most 5 seconds). Zero disables caching.
	CacheTTL time.Duration
	// Metrics records rotation progress; nil records nothing.
	Metrics *appmetrics.Metrics
}

// Service authenticates and manages API keys.
type Service struct {
	store          store.Store
	runner         txn.Runner
	audit          audit.Recorder
	clock          clock.Clock
	pepper         []byte
	pepperPrevious []byte
	cache          *lookupCache
	metrics        *appmetrics.Metrics
}

// New returns a Service.
func New(st store.Store, runner txn.Runner, rec audit.Recorder, clk clock.Clock, cfg Config) *Service {
	return &Service{
		store: st, runner: runner, audit: rec, clock: clk,
		pepper:         slices.Clone(cfg.Pepper),
		pepperPrevious: slices.Clone(cfg.PepperPrevious),
		cache:          newLookupCache(cfg.CacheTTL, maxCachedLookups),
		metrics:        cfg.Metrics,
	}
}

// Authenticate verifies an API key and returns the tenant scope it grants. Every failure
// (malformed, unknown, wrong secret, revoked, expired, environment mismatch) returns
// domain.ErrUnauthenticated; an inactive tenant returns domain.ErrTenantSuspended, but only after
// the key itself verified, so the tenant's state is revealed only to its key holders.
func (s *Service) Authenticate(ctx context.Context, token string) (tenancy.Scope, error) {
	// The HMAC is computed for every attempt, well-formed or not, so response time does not
	// reveal whether a lookup value exists.
	computed := domain.HashToken(s.pepper, token)
	var previous []byte
	if len(s.pepperPrevious) > 0 {
		previous = domain.HashToken(s.pepperPrevious, token)
	}
	lookup, kind, ok := domain.ParseToken(token)
	if !ok {
		return tenancy.Scope{}, domain.ErrUnauthenticated
	}
	now := s.clock.Now()
	res, err := s.lookup(ctx, lookup, now)
	if err != nil {
		return tenancy.Scope{}, err
	}
	// Both comparisons always run, so a key on the previous pepper is not distinguishable by time.
	var current, old bool
	if res != nil {
		current = hmac.Equal(computed, res.SecretHash)
		old = previous != nil && hmac.Equal(previous, res.SecretHash)
	}
	switch {
	case res == nil,
		!current && !old,
		res.Environment != kind,
		res.RevokedAt != nil && !now.Before(*res.RevokedAt),
		res.ExpiresAt != nil && !now.Before(*res.ExpiresAt):
		return tenancy.Scope{}, domain.ErrUnauthenticated
	case res.TenantStatus != tenantStatusActive:
		return tenancy.Scope{}, domain.ErrTenantSuspended
	}
	if old && !current {
		s.rehash(ctx, lookup, res, computed)
	}
	return tenancy.Scope{
		TenantID:        res.TenantID,
		EnvironmentID:   res.EnvironmentID,
		EnvironmentKind: res.Environment,
		Principal: tenancy.Principal{
			Type:        tenancy.PrincipalAPIKey,
			ID:          ids.Format(ids.PrefixAPIKey, res.ID),
			Permissions: res.Scopes,
		},
	}, nil
}

// rehash moves a key that verified under the previous pepper to the current one. It is best
// effort: the key already authenticated, so a failure is only counted, and the key is rehashed
// on a later request.
func (s *Service) rehash(ctx context.Context, lookup string, res *store.LookupResult, hash []byte) {
	// Not swapping is fine: another instance rehashed the key first, which is the same outcome.
	_, err := s.store.Rehash(context.WithoutCancel(ctx), res.ID, res.SecretHash, hash)
	s.cache.delete(lookup)
	s.metrics.PreviousPepperUsed(ctx, err == nil)
}

func (s *Service) lookup(ctx context.Context, lookup string, now time.Time) (*store.LookupResult, error) {
	if res, ok := s.cache.get(lookup, now); ok {
		return res, nil
	}
	res, err := s.store.Lookup(ctx, lookup)
	if err != nil {
		return nil, err
	}
	s.cache.put(lookup, res, now)
	return res, nil
}

// CreateKeyInput describes a key to create.
type CreateKeyInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// CreateKey creates a key in the caller's environment. Requires api_keys:write; the new key may
// only hold scopes the caller holds (no privilege escalation).
func (s *Service) CreateKey(ctx context.Context, in CreateKeyInput) (domain.CreatedKey, error) {
	if err := authz.Require(ctx, authz.APIKeysWrite); err != nil {
		return domain.CreatedKey{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.CreatedKey{}, err
	}
	name, err := validateName(in.Name)
	if err != nil {
		return domain.CreatedKey{}, err
	}
	scopes, err := validateScopes(in.Scopes, scope.Principal)
	if err != nil {
		return domain.CreatedKey{}, err
	}
	now := s.clock.Now()
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return domain.CreatedKey{}, fmt.Errorf("%w: expires_at must be in the future", domain.ErrInvalidInput)
	}

	var created domain.CreatedKey
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		created, err = s.createInTx(ctx, scope, name, scopes, in.ExpiresAt, now)
		return err
	})
	return created, err
}

func (s *Service) createInTx(ctx context.Context, scope tenancy.Scope, name string, scopes []string, expiresAt *time.Time, now time.Time) (domain.CreatedKey, error) {
	token, lookup := domain.NewToken(scope.EnvironmentKind)
	id := ids.NewV7()
	if err := s.store.Insert(ctx, store.InsertParams{
		ID: id, Name: name, Lookup: lookup, SecretHash: domain.HashToken(s.pepper, token),
		Scopes: scopes, CreatedBy: scope.Principal.ID, ExpiresAt: expiresAt,
	}); err != nil {
		return domain.CreatedKey{}, err
	}
	if err := s.audit.Record(ctx, audit.Entry{
		Action: "api_key.created", ResourceType: "api_key", ResourceID: ids.Format(ids.PrefixAPIKey, id),
		Outcome:  audit.OutcomeSuccess,
		Metadata: map[string]any{"name": name, "scopes": scopes, "prefix": domain.DisplayPrefix(scope.EnvironmentKind, lookup)},
	}); err != nil {
		return domain.CreatedKey{}, err
	}
	return domain.CreatedKey{
		Key: domain.APIKey{
			ID: id, Name: name, Lookup: lookup, Environment: scope.EnvironmentKind, Scopes: scopes,
			CreatedAt: now, ExpiresAt: expiresAt,
		},
		Secret: token,
	}, nil
}

// ListKeys returns a newest-first page of the caller's environment's keys. Requires
// api_keys:read.
func (s *Service) ListKeys(ctx context.Context, p domain.Page) ([]domain.APIKey, error) {
	if err := authz.Require(ctx, authz.APIKeysRead); err != nil {
		return nil, err
	}
	var keys []domain.APIKey
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		keys, err = s.store.List(ctx, p)
		return err
	})
	return keys, err
}

// RotateKey replaces a key with a new one that has the same name and scopes. The old key keeps
// working for overlap, then expires. Requires api_keys:write and every scope of the key.
func (s *Service) RotateKey(ctx context.Context, id uuid.UUID, overlap time.Duration) (domain.CreatedKey, error) {
	if err := authz.Require(ctx, authz.APIKeysWrite); err != nil {
		return domain.CreatedKey{}, err
	}
	if overlap < 0 || overlap > MaxOverlap {
		return domain.CreatedKey{}, fmt.Errorf("%w: overlap must be between 0 and 7 days", domain.ErrInvalidInput)
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.CreatedKey{}, err
	}
	now := s.clock.Now()

	var created domain.CreatedKey
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		old, err := s.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if old.ReplacedBy != nil || old.Status(now) != domain.StatusActive {
			return domain.ErrConflict
		}
		for _, sc := range old.Scopes {
			if !scope.Principal.Can(sc) {
				return authz.ErrForbidden
			}
		}
		if created, err = s.createInTx(ctx, scope, old.Name, old.Scopes, nil, now); err != nil {
			return err
		}
		if _, err := s.store.MarkReplaced(ctx, old.ID, created.Key.ID, now.Add(overlap)); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "api_key.rotated", ResourceType: "api_key", ResourceID: ids.Format(ids.PrefixAPIKey, old.ID),
			Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{
				"replaced_by":     ids.Format(ids.PrefixAPIKey, created.Key.ID),
				"overlap_seconds": int64(overlap / time.Second),
			},
		})
	})
	if err != nil {
		return domain.CreatedKey{}, err
	}
	return created, nil
}

// RevokeKey revokes a key immediately. Other instances stop accepting it within the lookup cache
// TTL. Revoking a revoked key returns it unchanged and records nothing. Requires api_keys:write.
func (s *Service) RevokeKey(ctx context.Context, id uuid.UUID) (domain.APIKey, error) {
	if err := authz.Require(ctx, authz.APIKeysWrite); err != nil {
		return domain.APIKey{}, err
	}
	now := s.clock.Now()
	var key domain.APIKey
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		current, err := s.store.Get(ctx, id)
		if err != nil {
			return err
		}
		if current.RevokedAt != nil {
			key = current
			return nil
		}
		if key, err = s.store.Revoke(ctx, id, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "api_key.revoked", ResourceType: "api_key", ResourceID: ids.Format(ids.PrefixAPIKey, id),
			Outcome: audit.OutcomeSuccess,
		})
	})
	if err != nil {
		return domain.APIKey{}, err
	}
	s.cache.delete(key.Lookup)
	return key, nil
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameLength {
		return "", fmt.Errorf("%w: name must be 1-%d characters", domain.ErrInvalidInput, maxNameLength)
	}
	return name, nil
}

// validateScopes checks that scopes are known, unique, bounded in number and all held by the
// caller, and returns them sorted.
func validateScopes(scopes []string, caller tenancy.Principal) ([]string, error) {
	if len(scopes) < 1 || len(scopes) > maxScopes {
		return nil, fmt.Errorf("%w: between 1 and %d scopes are required", domain.ErrInvalidInput, maxScopes)
	}
	out := slices.Clone(scopes)
	slices.Sort(out)
	if len(slices.Compact(slices.Clone(out))) != len(out) {
		return nil, fmt.Errorf("%w: scopes must be unique", domain.ErrInvalidInput)
	}
	for _, sc := range out {
		if !authz.IsCustomerPermission(sc) {
			return nil, fmt.Errorf("%w: unknown scope %q", domain.ErrInvalidInput, sc)
		}
		if !caller.Can(sc) {
			// A credential can never mint a more powerful one.
			return nil, authz.ErrForbidden
		}
	}
	return out, nil
}
