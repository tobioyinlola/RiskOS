package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
)

func rateLimitToAPI(l ratelimit.Limit) adminapi.RateLimit {
	return adminapi.RateLimit{PerSecond: l.PerSecond, Burst: l.Burst}
}

func limitsToAPI(v service.LimitsView) adminapi.TenantLimits {
	out := adminapi.TenantLimits{}
	out.Defaults.PerKey = rateLimitToAPI(v.Defaults.PerKey)
	out.Defaults.PerTenant = rateLimitToAPI(v.Defaults.PerTenant)
	if o := v.Override; o != nil {
		out.Override = &adminapi.TenantLimitsOverride{
			PerKey:    adminapi.RateLimit{PerSecond: o.PerKeyPerSecond, Burst: int(o.PerKeyBurst)},
			PerTenant: adminapi.RateLimit{PerSecond: o.PerTenantPerSecond, Burst: int(o.PerTenantBurst)},
			Reason:    o.Reason,
			UpdatedBy: adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, o.UpdatedBy), Name: v.UpdatedByName},
			UpdatedAt: o.UpdatedAt,
		}
	}
	return out
}

func tenantIDOf(raw string) (uuid.UUID, error) {
	id, err := ids.Parse(ids.PrefixTenant, raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, httpx.NotFound(err)
	}
	return id, nil
}

// GetTenantLimits implements GET /api/admin/v1/tenants/{tenant_id}/limits.
func (h *Handlers) GetTenantLimits(ctx context.Context, req adminapi.GetTenantLimitsRequestObject) (adminapi.GetTenantLimitsResponseObject, error) {
	id, err := tenantIDOf(req.TenantId)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.Limits(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.GetTenantLimits200JSONResponse(limitsToAPI(v)), nil
}

// SetTenantLimits implements PUT /api/admin/v1/tenants/{tenant_id}/limits.
func (h *Handlers) SetTenantLimits(ctx context.Context, req adminapi.SetTenantLimitsRequestObject) (adminapi.SetTenantLimitsResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	id, err := tenantIDOf(req.TenantId)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.SetLimits(ctx, s, id, service.LimitsInput{
		PerKey:    ratelimit.Limit{PerSecond: req.Body.PerKey.PerSecond, Burst: req.Body.PerKey.Burst},
		PerTenant: ratelimit.Limit{PerSecond: req.Body.PerTenant.PerSecond, Burst: req.Body.PerTenant.Burst},
		Reason:    req.Body.Reason,
	})
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.SetTenantLimits200JSONResponse(limitsToAPI(v)), nil
}

// ClearTenantLimits implements DELETE /api/admin/v1/tenants/{tenant_id}/limits.
func (h *Handlers) ClearTenantLimits(ctx context.Context, req adminapi.ClearTenantLimitsRequestObject) (adminapi.ClearTenantLimitsResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	id, err := tenantIDOf(req.TenantId)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.ClearLimits(ctx, s, id)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.ClearTenantLimits200JSONResponse(limitsToAPI(v)), nil
}
