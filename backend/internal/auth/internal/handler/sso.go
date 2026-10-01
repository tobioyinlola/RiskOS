package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
)

// Error codes of single sign-on.
const (
	CodeSSONotConfigured = "SSO_NOT_CONFIGURED"
	CodeSSOFailed        = "SSO_FAILED"
	CodeSSOProvider      = "SSO_PROVIDER_UNAVAILABLE"
)

// ssoCookie binds an SSO request to the browser that started it. Lax, because the browser
// returns from the provider by a cross-site navigation; scoped to the SSO endpoints.
const ssoCookie = "riskos_sso"

func (h *ConsoleHandlers) ssoCookie(token string) string {
	c := http.Cookie{ //nolint:gosec // HttpOnly is fixed; Secure is validated in config
		Name: ssoCookie, Value: token, Path: "/api/v1/auth/sso", HttpOnly: true, Secure: h.cc.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 600,
	}
	return c.String()
}

func mapSSOError(err error) error {
	switch {
	case errors.Is(err, domain.ErrSSONotConfigured):
		return httpx.NewError(http.StatusNotFound, CodeSSONotConfigured, "Single sign-on is not set up for this email. Sign in with your password.", err)
	case errors.Is(err, domain.ErrSSOFailed):
		return httpx.NewError(http.StatusUnauthorized, CodeSSOFailed, "Single sign-on did not complete. Start again from the sign-in page.", err)
	case errors.Is(err, domain.ErrSSOProvider):
		return httpx.NewError(http.StatusBadGateway, CodeSSOProvider, "Your identity provider could not be reached. Try again shortly.", err)
	case errors.Is(err, domain.ErrSSONotFound):
		return httpx.NotFound(err)
	default:
		return mapConsoleError(err)
	}
}

// StartSSO implements POST /api/v1/auth/sso/start.
func (h *ConsoleHandlers) StartSSO(ctx context.Context, req apigen.StartSSORequestObject) (apigen.StartSSOResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	st, err := h.con.StartSSO(ctx, req.Body.Email, client(ctx))
	if err != nil {
		return nil, mapSSOError(err)
	}
	return apigen.StartSSO200JSONResponse{
		Body:    apigen.SSOStart{RedirectUrl: st.RedirectURL},
		Headers: apigen.StartSSO200ResponseHeaders{SetCookie: new(h.ssoCookie(st.BrowserToken))},
	}, nil
}

// FinishSSO implements POST /api/v1/auth/sso/callback.
func (h *ConsoleHandlers) FinishSSO(ctx context.Context, req apigen.FinishSSORequestObject) (apigen.FinishSSOResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	var browser string
	if req.Params.RiskosSso != nil {
		browser = *req.Params.RiskosSso
	}
	in, err := h.con.FinishSSO(ctx, req.Body.State, req.Body.Code, browser, client(ctx))
	if err != nil {
		return nil, mapSSOError(err)
	}
	st, err := h.stateFor(ctx, in)
	if err != nil {
		return nil, err
	}
	return apigen.FinishSSO200JSONResponse{Body: st, Headers: apigen.FinishSSO200ResponseHeaders{SetCookie: new(h.cookie(in.Token, h.cc.MaxAge))}}, nil
}

func ssoToAPI(c service.SSOConnection) apigen.SSOConnection {
	return apigen.SSOConnection{Issuer: c.Issuer, ClientId: c.ClientID, Enabled: c.Enabled, UpdatedAt: c.UpdatedAt, UpdatedBy: c.UpdatedBy}
}

// GetSSO implements GET /api/v1/sso.
func (h *ConsoleHandlers) GetSSO(ctx context.Context, _ apigen.GetSSORequestObject) (apigen.GetSSOResponseObject, error) {
	c, err := h.con.GetSSO(ctx)
	if err != nil {
		return nil, mapSSOError(err)
	}
	out := apigen.GetSSO200JSONResponse{RedirectUri: h.con.SSORedirectURL()}
	if c != nil {
		out.Connection = new(ssoToAPI(*c))
	}
	return out, nil
}

// PutSSO implements PUT /api/v1/sso.
func (h *ConsoleHandlers) PutSSO(ctx context.Context, req apigen.PutSSORequestObject) (apigen.PutSSOResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	c, err := h.con.PutSSO(ctx, service.SSOInput{
		Issuer: req.Body.Issuer, ClientID: req.Body.ClientId, ClientSecret: req.Body.ClientSecret, Enabled: req.Body.Enabled,
	})
	if err != nil {
		return nil, mapSSOError(err)
	}
	return apigen.PutSSO200JSONResponse(ssoToAPI(c)), nil
}

// DeleteSSO implements DELETE /api/v1/sso.
func (h *ConsoleHandlers) DeleteSSO(ctx context.Context, _ apigen.DeleteSSORequestObject) (apigen.DeleteSSOResponseObject, error) {
	if err := h.con.DeleteSSO(ctx); err != nil {
		return nil, mapSSOError(err)
	}
	return apigen.DeleteSSO204Response{}, nil
}
