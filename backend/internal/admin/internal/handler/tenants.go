package handler

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

func tenantToAPI(r sqlcgen.AdminTenantDirectoryRow) adminapi.AdminTenant {
	t := adminapi.AdminTenant{
		Id: ids.Format(ids.PrefixTenant, r.ID), Name: r.Name, Region: r.Region,
		Status: adminapi.AdminTenantStatus(r.Status), Origin: adminapi.AdminTenantOrigin(r.Origin),
		ProductionEnabled: r.ProductionEnabled, CreatedAt: r.CreatedAt,
		ActiveUsers: r.ActiveUsers, ActiveApiKeys: r.ActiveApiKeys,
	}
	if r.OnboardingStatus != "" {
		t.OnboardingStatus = &r.OnboardingStatus
	}
	return t
}

func auditToAPI(r sqlcgen.StaffAuditLog) adminapi.StaffAuditEntry {
	meta := map[string]any{}
	_ = json.Unmarshal(r.Metadata, &meta) //nolint:errcheck // written by us as an object
	e := adminapi.StaffAuditEntry{
		Id: r.ID.String(), OccurredAt: r.OccurredAt, Actor: r.Actor, Action: r.Action,
		ResourceId: r.ResourceID, Outcome: r.Outcome, Metadata: meta,
	}
	if r.TenantID != nil {
		id := ids.Format(ids.PrefixTenant, *r.TenantID)
		e.TenantId = &id
	}
	return e
}

// GetAdminOverview implements GET /api/admin/v1/overview.
func (h *Handlers) GetAdminOverview(ctx context.Context, _ adminapi.GetAdminOverviewRequestObject) (adminapi.GetAdminOverviewResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	o, err := h.svc.Overview(ctx, s.StaffID)
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.GetAdminOverview200JSONResponse{
		TenantsTotal: o.TenantsTotal, TenantsActive: o.TenantsActive, TenantsSuspended: o.TenantsSuspended,
		OnboardingSubmitted: o.OnboardingSubmitted, SupportPending: o.SupportPending,
		SupportActive: o.SupportActive, StaffActive: o.StaffActive,
		StatusRequestsPending: o.StatusRequestsPending, EmergencyReviewsPending: o.EmergencyReviewsPending,
	}, nil
}

// GetAdminTenant implements GET /api/admin/v1/tenants/{tenant_id}.
func (h *Handlers) GetAdminTenant(ctx context.Context, req adminapi.GetAdminTenantRequestObject) (adminapi.GetAdminTenantResponseObject, error) {
	tenantID, err := ids.Parse(ids.PrefixTenant, req.TenantId)
	if err != nil || tenantID == uuid.Nil {
		return nil, httpx.NotFound(err)
	}
	d, err := h.svc.TenantDetail(ctx, tenantID)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.GetAdminTenant200JSONResponse{
		Tenant: tenantToAPI(d.Tenant), Since: d.Since,
		Security: adminapi.TenantSecurity{
			TenantId: ids.Format(ids.PrefixTenant, d.Tenant.ID), TenantName: d.Tenant.Name, TenantStatus: d.Tenant.Status,
			DegradedEvaluations: d.Security.DegradedEvaluations, OpenSecurityAlerts: d.Security.OpenSecurityAlerts,
			FailedLogins: d.Security.FailedLogins, DisabledWebhooks: d.Security.DisabledWebhooks,
		},
		RecentStaffActions: make([]adminapi.StaffAuditEntry, len(d.Actions)),
	}
	for i, a := range d.Actions {
		out.RecentStaffActions[i] = auditToAPI(a)
	}
	return out, nil
}
