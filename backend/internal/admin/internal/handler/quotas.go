package handler

import (
	"context"

	"github.com/oapi-codegen/runtime/types"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func quotaToAPI(v service.QuotaView) adminapi.TenantQuota {
	q := v.Quota
	if q == nil {
		return adminapi.TenantQuota{}
	}
	usage := adminapi.TenantQuotaUsage{Events: q.UsageEvents, CheckedAt: q.UsageCheckedAt, Exceeded: v.Exceeded()}
	if q.UsageMonth != nil {
		usage.Month = &types.Date{Time: *q.UsageMonth}
	}
	return adminapi.TenantQuota{Quota: &adminapi.TenantQuotaDetail{
		MonthlyEvents: q.MonthlyEvents,
		Reason:        q.Reason,
		UpdatedBy:     adminapi.StaffRef{Id: ids.Format(ids.PrefixStaff, q.UpdatedBy), Name: v.UpdatedByName},
		UpdatedAt:     q.UpdatedAt,
		Usage:         usage,
	}}
}

// GetTenantQuota implements GET /api/admin/v1/tenants/{tenant_id}/quota.
func (h *Handlers) GetTenantQuota(ctx context.Context, req adminapi.GetTenantQuotaRequestObject) (adminapi.GetTenantQuotaResponseObject, error) {
	id, err := tenantIDOf(req.TenantId)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.Quota(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.GetTenantQuota200JSONResponse(quotaToAPI(v)), nil
}

// SetTenantQuota implements PUT /api/admin/v1/tenants/{tenant_id}/quota.
func (h *Handlers) SetTenantQuota(ctx context.Context, req adminapi.SetTenantQuotaRequestObject) (adminapi.SetTenantQuotaResponseObject, error) {
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
	v, err := h.svc.SetQuota(ctx, s, id, service.QuotaInput{MonthlyEvents: req.Body.MonthlyEvents, Reason: req.Body.Reason})
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.SetTenantQuota200JSONResponse(quotaToAPI(v)), nil
}

// ClearTenantQuota implements DELETE /api/admin/v1/tenants/{tenant_id}/quota.
func (h *Handlers) ClearTenantQuota(ctx context.Context, req adminapi.ClearTenantQuotaRequestObject) (adminapi.ClearTenantQuotaResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	id, err := tenantIDOf(req.TenantId)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.ClearQuota(ctx, s, id)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.ClearTenantQuota200JSONResponse(quotaToAPI(v)), nil
}
