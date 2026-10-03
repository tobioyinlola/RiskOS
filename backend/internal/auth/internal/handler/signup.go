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

// Error codes of sign-up and Google sign-in.
const (
	CodeSignupDisabled      = "SIGNUP_DISABLED"
	CodeSignupInvalid       = "SIGNUP_INVALID"
	CodeCodeInvalid         = "CODE_INVALID"
	CodeGoogleNotConfigured = "GOOGLE_NOT_CONFIGURED"
	CodeGoogleFailed        = "GOOGLE_FAILED"
	CodeProductionLocked    = "PRODUCTION_LOCKED"
)

// signupCookie ties a sign-up in flight to the browser. Lax, because a Google sign-up returns
// from Google by a cross-site navigation; scoped to the sign-up endpoints; one hour.
const signupCookie = "riskos_signup"

// googleCookie binds a Google request to the browser that started it; ten minutes.
const googleCookie = "riskos_google"

func (h *ConsoleHandlers) laxCookie(name, path, token string, maxAge int) string {
	c := http.Cookie{ //nolint:gosec // HttpOnly is fixed; Secure is validated in config
		Name: name, Value: token, Path: path, HttpOnly: true, Secure: h.cc.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	}
	return c.String()
}

func (h *ConsoleHandlers) signupCookie(token string) string {
	return h.laxCookie(signupCookie, "/api/v1/auth/signup", token, 3600)
}

func mapSignupError(err error) error {
	switch {
	case errors.Is(err, domain.ErrSignupDisabled):
		return httpx.NewError(http.StatusNotFound, CodeSignupDisabled, "Sign-up with email is not available. Contact RiskOS.", err)
	case errors.Is(err, domain.ErrSignupInvalid):
		return httpx.NewError(http.StatusUnauthorized, CodeSignupInvalid, "This sign-up has expired or ended. Start again.", err)
	case errors.Is(err, domain.ErrCodeInvalid):
		return httpx.NewError(http.StatusBadRequest, CodeCodeInvalid, "That code is incorrect. Check the latest email and try again.", err)
	case errors.Is(err, domain.ErrGoogleNotConfigured):
		return httpx.NewError(http.StatusNotFound, CodeGoogleNotConfigured, "Google sign-in is not available.", err)
	case errors.Is(err, domain.ErrGoogleFailed):
		return httpx.NewError(http.StatusUnauthorized, CodeGoogleFailed, "Google sign-in did not complete. Start again.", err)
	case errors.Is(err, domain.ErrEmailTaken):
		return httpx.Conflict("This email already has a RiskOS account. Sign in instead.", err)
	default:
		return mapSSOError(err)
	}
}

func pendingToAPI(p service.SignupPending) apigen.SignupPending {
	return apigen.SignupPending{Email: p.Email, ResendAfterSeconds: int(p.ResendAfter.Seconds()), ExpiresAt: p.ExpiresAt}
}

func cookieParam(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// GetSignupOptions implements GET /api/v1/auth/signup/options.
func (h *ConsoleHandlers) GetSignupOptions(_ context.Context, _ apigen.GetSignupOptionsRequestObject) (apigen.GetSignupOptionsResponseObject, error) {
	email, google := h.con.SignupOptions()
	return apigen.GetSignupOptions200JSONResponse{Email: email, Google: google}, nil
}

// StartSignup implements POST /api/v1/auth/signup.
func (h *ConsoleHandlers) StartSignup(ctx context.Context, req apigen.StartSignupRequestObject) (apigen.StartSignupResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	p, err := h.con.StartSignup(ctx, service.SignupInput{
		Name: req.Body.Name, Email: req.Body.Email, BusinessName: req.Body.BusinessName, Password: req.Body.Password,
	}, client(ctx))
	if err != nil {
		return nil, mapSignupError(err)
	}
	return apigen.StartSignup202JSONResponse{
		Body: pendingToAPI(p), Headers: apigen.StartSignup202ResponseHeaders{SetCookie: new(h.signupCookie(p.Token))},
	}, nil
}

// ResendSignupCode implements POST /api/v1/auth/signup/resend.
func (h *ConsoleHandlers) ResendSignupCode(ctx context.Context, req apigen.ResendSignupCodeRequestObject) (apigen.ResendSignupCodeResponseObject, error) {
	p, err := h.con.ResendSignupCode(ctx, cookieParam(req.Params.RiskosSignup))
	if err != nil {
		return nil, mapSignupError(err)
	}
	return apigen.ResendSignupCode200JSONResponse(pendingToAPI(p)), nil
}

// VerifySignup implements POST /api/v1/auth/signup/verify.
func (h *ConsoleHandlers) VerifySignup(ctx context.Context, req apigen.VerifySignupRequestObject) (apigen.VerifySignupResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in, err := h.con.VerifySignup(ctx, cookieParam(req.Params.RiskosSignup), req.Body.Code, client(ctx))
	if err != nil {
		return nil, mapSignupError(err)
	}
	st, err := h.stateFor(ctx, in)
	if err != nil {
		return nil, err
	}
	return apigen.VerifySignup200JSONResponse{Body: st, Headers: apigen.VerifySignup200ResponseHeaders{SetCookie: new(h.cookie(in.Token, h.cc.MaxAge))}}, nil
}

// CompleteSignup implements POST /api/v1/auth/signup/complete.
func (h *ConsoleHandlers) CompleteSignup(ctx context.Context, req apigen.CompleteSignupRequestObject) (apigen.CompleteSignupResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in, err := h.con.CompleteSignup(ctx, cookieParam(req.Params.RiskosSignup), req.Body.BusinessName, client(ctx))
	if err != nil {
		return nil, mapSignupError(err)
	}
	st, err := h.stateFor(ctx, in)
	if err != nil {
		return nil, err
	}
	return apigen.CompleteSignup200JSONResponse{Body: st, Headers: apigen.CompleteSignup200ResponseHeaders{SetCookie: new(h.cookie(in.Token, h.cc.MaxAge))}}, nil
}

// StartGoogle implements POST /api/v1/auth/google/start.
func (h *ConsoleHandlers) StartGoogle(ctx context.Context, _ apigen.StartGoogleRequestObject) (apigen.StartGoogleResponseObject, error) {
	st, err := h.con.StartGoogle(ctx, client(ctx))
	if err != nil {
		return nil, mapSignupError(err)
	}
	return apigen.StartGoogle200JSONResponse{
		Body:    apigen.SSOStart{RedirectUrl: st.RedirectURL},
		Headers: apigen.StartGoogle200ResponseHeaders{SetCookie: new(h.laxCookie(googleCookie, "/api/v1/auth/google", st.BrowserToken, 600))},
	}, nil
}

// FinishGoogle implements POST /api/v1/auth/google/callback.
func (h *ConsoleHandlers) FinishGoogle(ctx context.Context, req apigen.FinishGoogleRequestObject) (apigen.FinishGoogleResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	out, err := h.con.FinishGoogle(ctx, req.Body.State, req.Body.Code, cookieParam(req.Params.RiskosGoogle), client(ctx))
	if err != nil {
		return nil, mapSignupError(err)
	}
	if out.SignIn != nil {
		st, err := h.stateFor(ctx, *out.SignIn)
		if err != nil {
			return nil, err
		}
		return apigen.FinishGoogle200JSONResponse{
			Body:    apigen.GoogleResult{Outcome: apigen.SignedIn, Auth: &st},
			Headers: apigen.FinishGoogle200ResponseHeaders{SetCookie: new(h.cookie(out.SignIn.Token, h.cc.MaxAge))},
		}, nil
	}
	return apigen.FinishGoogle200JSONResponse{
		Body:    apigen.GoogleResult{Outcome: apigen.NeedsBusiness, Email: &out.Signup.Email, Name: &out.Name},
		Headers: apigen.FinishGoogle200ResponseHeaders{SetCookie: new(h.signupCookie(out.Signup.Token))},
	}, nil
}
