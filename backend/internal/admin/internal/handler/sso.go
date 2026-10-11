package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
)

// Error codes of staff single sign-on.
const (
	CodeSSORequired      = "SSO_REQUIRED"
	CodeSSOFailed        = "SSO_FAILED"
	CodeSSOProvider      = "SSO_PROVIDER_UNAVAILABLE"
	CodeSSONotConfigured = "SSO_NOT_CONFIGURED"
)

// ssoCookie binds an SSO request to the browser that started it. Lax, because the browser
// returns from the provider by a cross-site navigation; scoped to the SSO endpoints.
const ssoCookie = "riskos_staff_sso"

func (h *Handlers) ssoCookie(token string) string {
	c := http.Cookie{ //nolint:gosec // HttpOnly is fixed; Secure is validated in config
		Name: ssoCookie, Value: token, Path: "/api/admin/v1/auth/sso", HttpOnly: true, Secure: h.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 600,
	}
	return c.String()
}

// GetStaffAuthMethods implements GET /api/admin/v1/auth/methods.
func (h *Handlers) GetStaffAuthMethods(_ context.Context, _ adminapi.GetStaffAuthMethodsRequestObject) (adminapi.GetStaffAuthMethodsResponseObject, error) {
	m := h.svc.Methods()
	return adminapi.GetStaffAuthMethods200JSONResponse{SsoEnabled: m.SSOEnabled, SsoRequired: m.SSORequired}, nil
}

// StartStaffSSO implements POST /api/admin/v1/auth/sso/start.
func (h *Handlers) StartStaffSSO(ctx context.Context, _ adminapi.StartStaffSSORequestObject) (adminapi.StartStaffSSOResponseObject, error) {
	redirect, browser, err := h.svc.StartSSO(ctx, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.StartStaffSSO200JSONResponse{
		Body:    adminapi.StaffSSOStart{RedirectUrl: redirect},
		Headers: adminapi.StartStaffSSO200ResponseHeaders{SetCookie: new(h.ssoCookie(browser))},
	}, nil
}

// FinishStaffSSO implements POST /api/admin/v1/auth/sso/callback.
func (h *Handlers) FinishStaffSSO(ctx context.Context, req adminapi.FinishStaffSSORequestObject) (adminapi.FinishStaffSSOResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	var browser string
	if req.Params.RiskosStaffSso != nil {
		browser = *req.Params.RiskosStaffSso
	}
	id, opts, err := h.svc.FinishSSO(ctx, req.Body.State, req.Body.Code, browser, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	var options map[string]any
	if err := json.Unmarshal(opts, &options); err != nil {
		return nil, err
	}
	return adminapi.FinishStaffSSO200JSONResponse{CeremonyId: id, Options: options}, nil
}

// StaffSSOLogin implements POST /api/admin/v1/auth/sso/login.
func (h *Handlers) StaffSSOLogin(ctx context.Context, req adminapi.StaffSSOLoginRequestObject) (adminapi.StaffSSOLoginResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	sess, token, err := h.svc.SSOLogin(ctx, req.Body.CeremonyId, cred, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.StaffSSOLogin200JSONResponse{
		Body:    sessionToAPI(sess),
		Headers: adminapi.StaffSSOLogin200ResponseHeaders{SetCookie: new(h.cookie(token, h.maxAge()))},
	}, nil
}
