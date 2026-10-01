package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Header and cookie names of the dashboard realm.
const (
	CSRFHeader        = "X-CSRF-Token"
	EnvironmentHeader = "RiskOS-Environment"
	sessionPathPrefix = "/api/v1/auth/"
)

// Error codes of the dashboard realm.
const (
	CodeInvalidCredentials = "INVALID_CREDENTIALS" //nolint:gosec // an error code, not a credential
	CodeMFAInvalid         = "MFA_INVALID"
	CodeReauthRequired     = "REAUTH_REQUIRED"
	CodeCSRF               = "CSRF_FAILED"
	CodeSessionState       = "SESSION_STATE"
)

// CookieConfig configures the session cookie and the console's origin.
type CookieConfig struct {
	Name   string
	Secure bool
	MaxAge time.Duration
	// Origins are the console origins allowed to make cookie-authenticated requests.
	Origins []string
	// ConsoleURL is the console's base URL, used to build invite links.
	ConsoleURL string
}

type sessionKey struct{}

// WithSession returns ctx carrying a console session.
func WithSession(ctx context.Context, s service.Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFrom returns the console session in ctx, if the request came from the console.
func SessionFrom(ctx context.Context) (service.Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(service.Session)
	return s, ok
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// Middleware authenticates every request under ProtectedPrefix: an API key (Authorization:
// Bearer) or a console session cookie. It is the only place a request scope is created.
//
// Session endpoints (/api/v1/auth/...) run with an optional session of any state and do their
// own checks. Every other path needs an API key or an active session; a session request picks
// its environment with the RiskOS-Environment header. Cookie-authenticated unsafe requests must
// come from an allowed Origin and carry the session's CSRF token.
func Middleware(keys *service.Service, con *service.Console, cc CookieConfig, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, ProtectedPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get("Authorization") != "" {
				scope, err := keys.Authenticate(r.Context(), bearerToken(r))
				if err != nil {
					writeAuthError(w, r, logger, err)
					return
				}
				next.ServeHTTP(w, r.WithContext(tenancy.WithScope(r.Context(), scope)))
				return
			}
			if unsafeMethod(r.Method) && !originAllowed(r, cc.Origins) {
				logger.WarnContext(r.Context(), "cross-origin console request rejected")
				httpx.WriteError(w, r, logger, httpx.NewError(http.StatusForbidden, CodeCSRF, "This request's origin is not allowed.", nil))
				return
			}
			var token string
			if c, err := r.Cookie(cc.Name); err == nil {
				token = c.Value
			}
			if strings.HasPrefix(r.URL.Path, sessionPathPrefix) {
				ctx := withClient(r.Context(), r)
				if token != "" {
					if sess, err := con.ResolveSession(ctx, token); err == nil {
						if unsafeMethod(r.Method) && sess.State == service.StateActive && !con.CheckCSRF(sess, r.Header.Get(CSRFHeader)) {
							httpx.WriteError(w, r, logger, httpx.NewError(http.StatusForbidden, CodeCSRF, "The CSRF token is missing or invalid.", nil))
							return
						}
						ctx = WithSession(ctx, sess)
					}
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			sess, err := con.ResolveSession(r.Context(), token)
			if err == nil && sess.State != service.StateActive {
				err = domain.ErrUnauthenticated
			}
			if err != nil {
				writeAuthError(w, r, logger, err)
				return
			}
			if unsafeMethod(r.Method) && !con.CheckCSRF(sess, r.Header.Get(CSRFHeader)) {
				httpx.WriteError(w, r, logger, httpx.NewError(http.StatusForbidden, CodeCSRF, "The CSRF token is missing or invalid.", nil))
				return
			}
			kind := tenancy.EnvironmentKind(r.Header.Get(EnvironmentHeader))
			switch kind {
			case "":
				kind = tenancy.EnvironmentTest
			case tenancy.EnvironmentTest, tenancy.EnvironmentProduction:
			default:
				httpx.WriteError(w, r, logger, httpx.InvalidRequest("RiskOS-Environment must be test or production.", nil))
				return
			}
			scope, err := con.Scope(sess, kind)
			if err != nil {
				writeAuthError(w, r, logger, err)
				return
			}
			ctx := WithSession(tenancy.WithScope(r.Context(), scope), sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// originAllowed accepts requests without an Origin header (same-origin navigations and
// non-browser clients; the cookie is SameSite=Strict) or from a configured console origin.
func originAllowed(r *http.Request, origins []string) bool {
	o := r.Header.Get("Origin")
	return o == "" || slices.Contains(origins, o)
}

// ReauthMiddleware requires a recent second-factor confirmation for high-impact operations
// performed from the console. API keys are not affected.
func ReauthMiddleware(con *service.Console, operations []string) apigen.StrictMiddlewareFunc {
	return func(f apigen.StrictHandlerFunc, operationID string) apigen.StrictHandlerFunc {
		if !slices.Contains(operations, operationID) {
			return f
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
			if s, ok := SessionFrom(ctx); ok && !s.ReauthFresh(time.Now(), con.ReauthWindow()) {
				return nil, httpx.NewError(http.StatusForbidden, CodeReauthRequired,
					"Verify it's you again (authenticator code or passkey) to do this.", domain.ErrReauthRequired)
			}
			return f(ctx, w, r, req)
		}
	}
}

// ConsoleHandlers implements the session and team operations.
type ConsoleHandlers struct {
	con *service.Console
	cc  CookieConfig
}

// NewConsole returns the session and team handlers.
func NewConsole(con *service.Console, cc CookieConfig) *ConsoleHandlers {
	return &ConsoleHandlers{con: con, cc: cc}
}

func (h *ConsoleHandlers) cookie(token string, maxAge time.Duration) string {
	// Secure is configuration: required in production, off only for local development over http.
	c := http.Cookie{ //nolint:gosec // HttpOnly and SameSite=Strict are fixed; Secure is validated in config
		Name: h.cc.Name, Value: token, Path: "/", HttpOnly: true, Secure: h.cc.Secure,
		SameSite: http.SameSiteStrictMode, MaxAge: int(maxAge.Seconds()),
	}
	if maxAge <= 0 {
		c.MaxAge = -1
	}
	return c.String()
}

type clientKey struct{}

// withClient records where a session request came from (for throttling and session records):
// the client address worked out by httpx.ClientIP, which looks through trusted proxies such as
// the console's server.
func withClient(ctx context.Context, r *http.Request) context.Context {
	cl := service.Client{UserAgent: r.UserAgent()}
	if ip, ok := requestctx.ClientIP(ctx); ok {
		cl.IP = &ip
	}
	return context.WithValue(ctx, clientKey{}, cl)
}

func client(ctx context.Context) service.Client {
	cl, _ := ctx.Value(clientKey{}).(service.Client) //nolint:errcheck // absent means unknown client
	return cl
}

func (h *ConsoleHandlers) session(ctx context.Context) (service.Session, error) {
	s, ok := SessionFrom(ctx)
	if !ok {
		return service.Session{}, domain.ErrUnauthenticated
	}
	return s, nil
}

func methods(p service.Profile) []apigen.AuthStateMfaMethods {
	var out []apigen.AuthStateMfaMethods
	if p.User.TotpSecretEnc != nil {
		out = append(out, "totp")
	}
	if len(p.Passkeys) > 0 {
		out = append(out, "passkey")
	}
	if len(p.User.RecoveryHashes) > 0 {
		out = append(out, "recovery_code")
	}
	return out
}

func (h *ConsoleHandlers) state(ctx context.Context, s service.Session, codes []string) (apigen.AuthState, error) {
	p, err := h.con.Profile(ctx, s)
	if err != nil {
		return apigen.AuthState{}, err
	}
	out := apigen.AuthState{State: apigen.SessionState(s.State), RecoveryCodesRemaining: new(len(p.User.RecoveryHashes))}
	if m := methods(p); len(m) > 0 {
		out.MfaMethods = &m
	}
	if codes != nil {
		out.RecoveryCodes = &codes
	}
	return out, nil
}

// after re-reads the session after a step changed it (the request context still holds the old
// state).
func (h *ConsoleHandlers) after(s service.Session) service.Session {
	s.State = service.StateActive
	now := time.Now()
	s.ReauthAt = &now
	return s
}

// Login implements POST /api/v1/auth/login.
func (h *ConsoleHandlers) Login(ctx context.Context, req apigen.LoginRequestObject) (apigen.LoginResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in, err := h.con.Login(ctx, req.Body.Email, req.Body.Password, client(ctx))
	if err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.stateFor(ctx, in)
	if err != nil {
		return nil, err
	}
	return apigen.Login200JSONResponse{Body: st, Headers: apigen.Login200ResponseHeaders{SetCookie: new(h.cookie(in.Token, h.cc.MaxAge))}}, nil
}

// stateFor describes a freshly opened session (its tenant is resolved from the token).
func (h *ConsoleHandlers) stateFor(ctx context.Context, in service.SignIn) (apigen.AuthState, error) {
	s, err := h.con.ResolveSession(ctx, in.Token)
	if err != nil {
		return apigen.AuthState{}, err
	}
	return h.state(ctx, s, nil)
}

// GetSession implements GET /api/v1/auth/session.
func (h *ConsoleHandlers) GetSession(ctx context.Context, _ apigen.GetSessionRequestObject) (apigen.GetSessionResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	p, err := h.con.Profile(ctx, s)
	if err != nil {
		return nil, err
	}
	out := apigen.GetSession200JSONResponse{
		State: apigen.SessionState(s.State), CsrfToken: s.CSRF, ExpiresAt: s.ExpiresAt,
		Environments: []apigen.Environment{apigen.Environment(tenancy.EnvironmentTest), apigen.Environment(tenancy.EnvironmentProduction)},
	}
	out.User.Id, out.User.Email, out.User.Name, out.User.Role = ids.Format(ids.PrefixUser, p.User.ID), p.User.Email, p.User.Name, apigen.Role(p.User.Role)
	out.Tenant.Id, out.Tenant.Name = ids.Format(ids.PrefixTenant, s.TenantID), s.TenantName
	out.Permissions = []apigen.Scope{}
	if s.State == service.StateActive {
		for _, perm := range authz.RolePermissions(s.Role) {
			out.Permissions = append(out.Permissions, apigen.Scope(perm))
		}
	}
	if s.ReauthAt != nil {
		out.ReauthValidUntil = new(s.ReauthAt.Add(h.con.ReauthWindow()))
	}
	out.Mfa.Totp = p.User.TotpSecretEnc != nil
	out.Mfa.RecoveryCodesRemaining = len(p.User.RecoveryHashes)
	out.Mfa.Passkeys = make([]apigen.Passkey, len(p.Passkeys))
	for i, k := range p.Passkeys {
		out.Mfa.Passkeys[i] = apigen.Passkey{
			Id: ids.PrefixPasskey + "_" + base64.RawURLEncoding.EncodeToString(k.ID), Name: k.Name, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt,
		}
	}
	return out, nil
}

// Logout implements POST /api/v1/auth/logout.
func (h *ConsoleHandlers) Logout(ctx context.Context, _ apigen.LogoutRequestObject) (apigen.LogoutResponseObject, error) {
	cleared := apigen.Logout204Response{Headers: apigen.Logout204ResponseHeaders{SetCookie: new(h.cookie("", 0))}}
	s, err := h.session(ctx)
	if err != nil {
		return cleared, nil //nolint:nilerr // signing out without a session still clears the cookie
	}
	if err := h.con.Logout(ctx, s); err != nil {
		return nil, err
	}
	return cleared, nil
}

// VerifyTotp implements POST /api/v1/auth/mfa/totp/verify.
func (h *ConsoleHandlers) VerifyTotp(ctx context.Context, req apigen.VerifyTotpRequestObject) (apigen.VerifyTotpResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	if err := h.con.VerifyTOTP(ctx, s, req.Body.Code); err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.state(ctx, h.after(s), nil)
	if err != nil {
		return nil, err
	}
	return apigen.VerifyTotp200JSONResponse(st), nil
}

// VerifyRecoveryCode implements POST /api/v1/auth/mfa/recovery/verify.
func (h *ConsoleHandlers) VerifyRecoveryCode(ctx context.Context, req apigen.VerifyRecoveryCodeRequestObject) (apigen.VerifyRecoveryCodeResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	if _, err := h.con.UseRecoveryCode(ctx, s, req.Body.Code); err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.state(ctx, h.after(s), nil)
	if err != nil {
		return nil, err
	}
	return apigen.VerifyRecoveryCode200JSONResponse(st), nil
}

// EnrollTotp implements POST /api/v1/auth/mfa/totp/enroll.
func (h *ConsoleHandlers) EnrollTotp(ctx context.Context, _ apigen.EnrollTotpRequestObject) (apigen.EnrollTotpResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	e, err := h.con.BeginTOTP(ctx, s)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	return apigen.EnrollTotp200JSONResponse{Secret: e.Secret, OtpauthUri: e.URI}, nil
}

// ConfirmTotp implements POST /api/v1/auth/mfa/totp/confirm.
func (h *ConsoleHandlers) ConfirmTotp(ctx context.Context, req apigen.ConfirmTotpRequestObject) (apigen.ConfirmTotpResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	codes, err := h.con.ConfirmTOTP(ctx, s, req.Body.Code)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.state(ctx, h.after(s), codes)
	if err != nil {
		return nil, err
	}
	return apigen.ConfirmTotp200JSONResponse(st), nil
}

// PasskeyRegistrationOptions implements POST /api/v1/auth/mfa/passkeys/register/options.
func (h *ConsoleHandlers) PasskeyRegistrationOptions(ctx context.Context, _ apigen.PasskeyRegistrationOptionsRequestObject) (apigen.PasskeyRegistrationOptionsResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	raw, err := h.con.BeginPasskeyRegistration(ctx, s)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode passkey options: %w", err)
	}
	return apigen.PasskeyRegistrationOptions200JSONResponse(out), nil
}

// RegisterPasskey implements POST /api/v1/auth/mfa/passkeys/register.
func (h *ConsoleHandlers) RegisterPasskey(ctx context.Context, req apigen.RegisterPasskeyRequestObject) (apigen.RegisterPasskeyResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	cred, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, httpx.InvalidRequest("The credential is invalid.", err)
	}
	name := ""
	if req.Body.Name != nil {
		name = *req.Body.Name
	}
	codes, err := h.con.FinishPasskeyRegistration(ctx, s, name, cred)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.state(ctx, h.after(s), codes)
	if err != nil {
		return nil, err
	}
	return apigen.RegisterPasskey200JSONResponse(st), nil
}

// PasskeyLoginOptions implements POST /api/v1/auth/mfa/passkeys/login/options.
func (h *ConsoleHandlers) PasskeyLoginOptions(ctx context.Context, _ apigen.PasskeyLoginOptionsRequestObject) (apigen.PasskeyLoginOptionsResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	raw, err := h.con.BeginPasskeyLogin(ctx, s)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode passkey options: %w", err)
	}
	return apigen.PasskeyLoginOptions200JSONResponse(out), nil
}

// VerifyPasskey implements POST /api/v1/auth/mfa/passkeys/login.
func (h *ConsoleHandlers) VerifyPasskey(ctx context.Context, req apigen.VerifyPasskeyRequestObject) (apigen.VerifyPasskeyResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	cred, err := json.Marshal(req.Body.Credential)
	if err != nil {
		return nil, httpx.InvalidRequest("The credential is invalid.", err)
	}
	if err := h.con.FinishPasskeyLogin(ctx, s, cred); err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.state(ctx, h.after(s), nil)
	if err != nil {
		return nil, err
	}
	return apigen.VerifyPasskey200JSONResponse(st), nil
}

// DeletePasskey implements DELETE /api/v1/auth/mfa/passkeys/{passkey_id}.
func (h *ConsoleHandlers) DeletePasskey(ctx context.Context, req apigen.DeletePasskeyRequestObject) (apigen.DeletePasskeyResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	if !s.ReauthFresh(time.Now(), h.con.ReauthWindow()) {
		return nil, mapConsoleError(domain.ErrReauthRequired)
	}
	raw, ok := strings.CutPrefix(req.PasskeyId, ids.PrefixPasskey+"_")
	id, err := base64.RawURLEncoding.DecodeString(raw)
	if !ok || err != nil {
		return nil, httpx.NotFound(err)
	}
	if err := h.con.DeletePasskey(ctx, s, id); err != nil {
		return nil, mapConsoleError(err)
	}
	return apigen.DeletePasskey204Response{}, nil
}

// RegenerateRecoveryCodes implements POST /api/v1/auth/mfa/recovery-codes.
func (h *ConsoleHandlers) RegenerateRecoveryCodes(ctx context.Context, _ apigen.RegenerateRecoveryCodesRequestObject) (apigen.RegenerateRecoveryCodesResponseObject, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	if !s.ReauthFresh(time.Now(), h.con.ReauthWindow()) {
		return nil, mapConsoleError(domain.ErrReauthRequired)
	}
	codes, err := h.con.RegenerateRecoveryCodes(ctx, s)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	return apigen.RegenerateRecoveryCodes200JSONResponse{RecoveryCodes: codes}, nil
}

// AcceptInvite implements POST /api/v1/auth/invites/accept.
func (h *ConsoleHandlers) AcceptInvite(ctx context.Context, req apigen.AcceptInviteRequestObject) (apigen.AcceptInviteResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	in, err := h.con.AcceptInvite(ctx, req.Body.Token, req.Body.Name, req.Body.Password, client(ctx))
	if err != nil {
		return nil, mapConsoleError(err)
	}
	st, err := h.stateFor(ctx, in)
	if err != nil {
		return nil, err
	}
	return apigen.AcceptInvite200JSONResponse{Body: st, Headers: apigen.AcceptInvite200ResponseHeaders{SetCookie: new(h.cookie(in.Token, h.cc.MaxAge))}}, nil
}

func memberToAPI(m service.Member) apigen.TeamMember {
	return apigen.TeamMember{
		Id: ids.Format(ids.PrefixUser, m.User.ID), Email: m.User.Email, Name: m.User.Name, Role: apigen.Role(m.User.Role),
		Status: apigen.TeamMemberStatus(m.User.Status), MfaEnabled: m.MFAEnabled, CreatedAt: m.User.CreatedAt, LastLoginAt: m.User.LastLoginAt,
	}
}

// ListTeamMembers implements GET /api/v1/team/members.
func (h *ConsoleHandlers) ListTeamMembers(ctx context.Context, _ apigen.ListTeamMembersRequestObject) (apigen.ListTeamMembersResponseObject, error) {
	ms, err := h.con.ListMembers(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	out := apigen.ListTeamMembers200JSONResponse{Data: make([]apigen.TeamMember, len(ms))}
	for i, m := range ms {
		out.Data[i] = memberToAPI(m)
	}
	return out, nil
}

// UpdateTeamMember implements PATCH /api/v1/team/members/{user_id}.
func (h *ConsoleHandlers) UpdateTeamMember(ctx context.Context, req apigen.UpdateTeamMemberRequestObject) (apigen.UpdateTeamMemberResponseObject, error) {
	id, err := ids.Parse(ids.PrefixUser, req.UserId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	var role *authz.Role
	if req.Body.Role != nil {
		role = new(authz.Role(*req.Body.Role))
	}
	var status *string
	if req.Body.Status != nil {
		status = new(string(*req.Body.Status))
	}
	m, err := h.con.UpdateMember(ctx, id, role, status)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	return apigen.UpdateTeamMember200JSONResponse(memberToAPI(m)), nil
}

func inviteStatus(now time.Time, acceptedAt, revokedAt *time.Time, expiresAt time.Time) apigen.InviteStatus {
	switch {
	case acceptedAt != nil:
		return "accepted"
	case revokedAt != nil:
		return "revoked"
	case !now.Before(expiresAt):
		return "expired"
	default:
		return "pending"
	}
}

// ListInvites implements GET /api/v1/team/invites.
func (h *ConsoleHandlers) ListInvites(ctx context.Context, _ apigen.ListInvitesRequestObject) (apigen.ListInvitesResponseObject, error) {
	invs, err := h.con.ListInvites(ctx)
	if err != nil {
		return nil, mapConsoleError(err)
	}
	now := time.Now()
	out := apigen.ListInvites200JSONResponse{Data: make([]apigen.Invite, len(invs))}
	for i, v := range invs {
		out.Data[i] = apigen.Invite{
			Id: ids.Format(ids.PrefixInvite, v.ID), Email: v.Email, Role: apigen.Role(v.Role), CreatedAt: v.CreatedAt,
			CreatedBy: v.CreatedBy, ExpiresAt: v.ExpiresAt, Status: inviteStatus(now, v.AcceptedAt, v.RevokedAt, v.ExpiresAt),
		}
	}
	return out, nil
}

// InviteURL builds the link for an invite token. The token is in the fragment, so it never
// reaches server logs or Referer headers.
func InviteURL(consoleURL, token string) string {
	return strings.TrimRight(consoleURL, "/") + "/accept-invite#token=" + token
}

// CreateInvite implements POST /api/v1/team/invites.
func (h *ConsoleHandlers) CreateInvite(ctx context.Context, req apigen.CreateInviteRequestObject) (apigen.CreateInviteResponseObject, error) {
	inv, err := h.con.CreateInvite(ctx, req.Body.Email, authz.Role(req.Body.Role))
	if err != nil {
		return nil, mapConsoleError(err)
	}
	scope, _ := tenancy.FromContext(ctx) //nolint:errcheck // CreateInvite already required a scope
	return apigen.CreateInvite201JSONResponse{
		Invite: apigen.Invite{
			Id: ids.Format(ids.PrefixInvite, inv.ID), Email: inv.Email, Role: apigen.Role(inv.Role), Status: "pending",
			CreatedAt: time.Now(), CreatedBy: scope.Principal.ID, ExpiresAt: inv.ExpiresAt,
		},
		InviteUrl: InviteURL(h.cc.ConsoleURL, inv.Token),
	}, nil
}

// RevokeInvite implements POST /api/v1/team/invites/{invite_id}/revoke.
func (h *ConsoleHandlers) RevokeInvite(ctx context.Context, req apigen.RevokeInviteRequestObject) (apigen.RevokeInviteResponseObject, error) {
	id, err := ids.Parse(ids.PrefixInvite, req.InviteId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	if err := h.con.RevokeInvite(ctx, id); err != nil {
		return nil, mapConsoleError(err)
	}
	return apigen.RevokeInvite204Response{}, nil
}

func mapConsoleError(err error) error {
	var rl *domain.RateLimitedError
	switch {
	case errors.As(err, &rl):
		return httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited,
			fmt.Sprintf("Too many attempts. Try again in %d seconds.", int(math.Ceil(rl.RetryAfter.Seconds()))), err).
			WithHeader("Retry-After", strconv.Itoa(int(math.Ceil(rl.RetryAfter.Seconds()))))
	case errors.Is(err, domain.ErrInvalidCredentials):
		return httpx.NewError(http.StatusUnauthorized, CodeInvalidCredentials, "The email or password is incorrect.", err)
	case errors.Is(err, domain.ErrUnauthenticated):
		return httpx.NewError(http.StatusUnauthorized, httpx.CodeUnauthenticated, "Sign in to continue.", err)
	case errors.Is(err, domain.ErrTenantSuspended):
		return httpx.NewError(http.StatusForbidden, httpx.CodeTenantSuspended, "This account is not active.", err)
	case errors.Is(err, domain.ErrMFAInvalid):
		return httpx.NewError(http.StatusUnauthorized, CodeMFAInvalid, "The verification failed. Check the code or try your passkey again.", err)
	case errors.Is(err, domain.ErrSessionState):
		return httpx.NewError(http.StatusConflict, CodeSessionState, "This step is not available for your session. Sign in again.", err)
	case errors.Is(err, domain.ErrReauthRequired):
		return httpx.NewError(http.StatusForbidden, CodeReauthRequired, "Verify it's you again (authenticator code or passkey) to do this.", err)
	case errors.Is(err, domain.ErrInviteInvalid):
		return httpx.NewError(http.StatusConflict, httpx.CodeConflict, "This invite link is invalid, used or expired. Ask for a new one.", err)
	case errors.Is(err, domain.ErrEmailTaken):
		return httpx.Conflict("This email already has a RiskOS account.", err)
	case errors.Is(err, domain.ErrLastAdmin):
		return httpx.Conflict("The tenant must keep at least one active administrator.", err)
	case errors.Is(err, domain.ErrSelfChange):
		return httpx.Conflict("You cannot change your own role or status.", err)
	case errors.Is(err, domain.ErrConflict):
		return httpx.Conflict(capitalize(strings.TrimPrefix(err.Error(), domain.ErrConflict.Error()+": "))+".", err)
	case errors.Is(err, domain.ErrUserNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.InvalidRequest(capitalize(strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": "))+".", err)
	default:
		return err
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
