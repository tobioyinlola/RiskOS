package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
)

// Bounds of a tenant's limits: the table's checks, restated so the caller gets a message.
const (
	maxPerSecond = 100000
	maxBurst     = 1000000
)

// LimitsInput is the override staff set for a tenant.
type LimitsInput struct {
	PerKey    ratelimit.Limit
	PerTenant ratelimit.Limit
	Reason    string
}

// LimitsView is what the console shows for a tenant: the platform's limits, the override if any,
// and who set it.
type LimitsView struct {
	Defaults      ratelimit.Override
	Override      *sqlcgen.TenantLimit
	UpdatedByName string
}

func validLimit(name string, l ratelimit.Limit) error {
	if l.PerSecond <= 0 || l.PerSecond > maxPerSecond {
		return fmt.Errorf("%w: %s rate must be above 0 and at most %d per second", ErrInvalid, name, maxPerSecond)
	}
	if l.Burst < 1 || l.Burst > maxBurst {
		return fmt.Errorf("%w: %s burst must be between 1 and %d", ErrInvalid, name, maxBurst)
	}
	return nil
}

// overrideCache keeps recent lookups for the request path, so a limit check costs no query.
type overrideCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[uuid.UUID]cachedOverride
}

type cachedOverride struct {
	o  ratelimit.Override
	ok bool
	at time.Time
}

func newOverrideCache(ttl time.Duration) *overrideCache {
	return &overrideCache{ttl: ttl, entries: map[uuid.UUID]cachedOverride{}}
}

func (c *overrideCache) get(id uuid.UUID, now time.Time) (ratelimit.Override, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, hit := c.entries[id]
	if !hit || now.Sub(e.at) >= c.ttl {
		return ratelimit.Override{}, false, false
	}
	return e.o, e.ok, true
}

func (c *overrideCache) put(id uuid.UUID, o ratelimit.Override, ok bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) > 10_000 {
		clear(c.entries)
	}
	c.entries[id] = cachedOverride{o: o, ok: ok, at: now}
}

func (c *overrideCache) forget(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}

func toOverride(l sqlcgen.TenantLimit) ratelimit.Override {
	return ratelimit.Override{
		PerKey:    ratelimit.Limit{PerSecond: l.PerKeyPerSecond, Burst: int(l.PerKeyBurst)},
		PerTenant: ratelimit.Limit{PerSecond: l.PerTenantPerSecond, Burst: int(l.PerTenantBurst)},
	}
}

// Override implements ratelimit.Overrides for the request path: the tenant's override, from a
// short-lived cache. A failed lookup reports none, so limiting falls back to the platform's.
func (s *Service) Override(ctx context.Context, tenantID uuid.UUID) (ratelimit.Override, bool) {
	now := s.clock.Now()
	if o, ok, hit := s.limits.get(tenantID, now); hit {
		return o, ok
	}
	row, err := s.store.Q().GetTenantLimits(ctx, tenantID)
	switch {
	case err == nil:
		o := toOverride(row)
		s.limits.put(tenantID, o, true, now)
		return o, true
	case errors.Is(store.Classify(err, "get tenant limits"), store.ErrNotFound):
		s.limits.put(tenantID, ratelimit.Override{}, false, now)
		return ratelimit.Override{}, false
	default:
		return ratelimit.Override{}, false
	}
}

// Limits returns a tenant's effective limits for the console.
func (s *Service) Limits(ctx context.Context, tenantID uuid.UUID) (LimitsView, error) {
	if err := s.tenantExists(ctx, tenantID); err != nil {
		return LimitsView{}, err
	}
	view := LimitsView{Defaults: s.cfg.DefaultLimits}
	row, err := s.store.Q().GetTenantLimits(ctx, tenantID)
	switch {
	case err == nil:
		view.Override = &row
		sn, _, err := s.names(ctx)
		if err != nil {
			return LimitsView{}, err
		}
		view.UpdatedByName = sn[row.UpdatedBy]
	case errors.Is(store.Classify(err, "get tenant limits"), store.ErrNotFound):
	default:
		return LimitsView{}, store.Classify(err, "get tenant limits")
	}
	return view, nil
}

func (s *Service) tenantExists(ctx context.Context, tenantID uuid.UUID) error {
	rows, err := s.store.Q().AdminTenants(ctx)
	if err != nil {
		return store.Classify(err, "get tenant")
	}
	for _, r := range rows {
		if r.ID == tenantID {
			return nil
		}
	}
	return ErrNotFound
}

// SetLimits replaces a tenant's per-key and per-tenant limits, with a reason. It takes effect
// within the cache lifetime on every API instance. Recorded in the staff audit log with the
// reason, and in the tenant's own audit log with the new values and no reason.
func (s *Service) SetLimits(ctx context.Context, sess Session, tenantID uuid.UUID, in LimitsInput) (LimitsView, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 10 || n > 500 {
		return LimitsView{}, fmt.Errorf("%w: give a reason of 10 to 500 characters", ErrInvalid)
	}
	if err := validLimit("per-key", in.PerKey); err != nil {
		return LimitsView{}, err
	}
	if err := validLimit("per-tenant", in.PerTenant); err != nil {
		return LimitsView{}, err
	}
	if err := s.tenantExists(ctx, tenantID); err != nil {
		return LimitsView{}, err
	}
	tctx := s.tenants.StaffTenantScope(ctx, tenantID, sess.Ref())
	err := s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		if err := q.UpsertTenantLimits(ctx, sqlcgen.UpsertTenantLimitsParams{
			TenantID: tenantID, PerKeyPerSecond: in.PerKey.PerSecond, PerKeyBurst: int32(in.PerKey.Burst), //nolint:gosec // 1..1e6
			PerTenantPerSecond: in.PerTenant.PerSecond, PerTenantBurst: int32(in.PerTenant.Burst), //nolint:gosec // 1..1e6
			Reason: in.Reason, UpdatedBy: sess.StaffID, Now: s.clock.Now(),
		}); err != nil {
			return store.Classify(err, "set tenant limits")
		}
		values := map[string]any{
			"per_key_per_second": in.PerKey.PerSecond, "per_key_burst": in.PerKey.Burst,
			"per_tenant_per_second": in.PerTenant.PerSecond, "per_tenant_burst": in.PerTenant.Burst,
		}
		staff := map[string]any{"reason": in.Reason}
		for k, v := range values {
			staff[k] = v
		}
		if err := s.record(ctx, q, sess.Ref(), "tenant_limits.set", &tenantID, ids.Format(ids.PrefixTenant, tenantID), "success", staff); err != nil {
			return err
		}
		values["staff_name"] = sess.Name
		return s.audit.Record(ctx, audit.Entry{
			Action: "tenant.limits_changed", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, tenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true, Metadata: values,
		})
	})
	if err != nil {
		return LimitsView{}, err
	}
	s.limits.forget(tenantID)
	return s.Limits(ctx, tenantID)
}

// ClearLimits removes a tenant's override, so the platform's limits apply again.
func (s *Service) ClearLimits(ctx context.Context, sess Session, tenantID uuid.UUID) (LimitsView, error) {
	if err := s.tenantExists(ctx, tenantID); err != nil {
		return LimitsView{}, err
	}
	tctx := s.tenants.StaffTenantScope(ctx, tenantID, sess.Ref())
	err := s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		n, err := q.DeleteTenantLimits(ctx, tenantID)
		if err != nil {
			return store.Classify(err, "clear tenant limits")
		}
		if n == 0 {
			return nil // nothing to clear is the state asked for
		}
		if err := s.record(ctx, q, sess.Ref(), "tenant_limits.cleared", &tenantID, ids.Format(ids.PrefixTenant, tenantID), "success", nil); err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "tenant.limits_changed", ResourceType: "tenant", ResourceID: ids.Format(ids.PrefixTenant, tenantID),
			Outcome: audit.OutcomeSuccess, TenantLevel: true,
			Metadata: map[string]any{"staff_name": sess.Name, "cleared": true},
		})
	})
	if err != nil {
		return LimitsView{}, err
	}
	s.limits.forget(tenantID)
	return s.Limits(ctx, tenantID)
}
