package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
)

// Prefix is the admin realm's route prefix.
const Prefix = "/api/admin/"

// CookieName is the staff session cookie.
const CookieName = "riskos_staff"

// csrfHeader carries the staff session's CSRF token.
const csrfHeader = "X-CSRF-Token"

// publicPaths are reachable without a session: the passkey ceremonies.
var publicPaths = []string{ //nolint:gochecknoglobals // immutable list
	"/api/admin/v1/auth/enroll/options", "/api/admin/v1/auth/enroll",
	"/api/admin/v1/auth/login/options", "/api/admin/v1/auth/login",
}

type sessionKey struct{}

type clientKey struct{}

func sessionFrom(ctx context.Context) (service.Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(service.Session)
	return s, ok
}

func clientFrom(ctx context.Context) service.Client {
	cl, _ := ctx.Value(clientKey{}).(service.Client) //nolint:errcheck // absent means unknown
	return cl
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// Middleware authenticates admin realm requests: the staff session cookie, an allowed Origin
// for unsafe requests and, with a session, its CSRF token. Paths outside Prefix pass through.
func Middleware(svc *service.Service, origins []string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, Prefix) {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			cl := service.Client{UserAgent: r.UserAgent()}
			if ip, ok := requestctx.ClientIP(ctx); ok {
				cl.IP = &ip
			}
			ctx = context.WithValue(ctx, clientKey{}, cl)
			if unsafeMethod(r.Method) {
				if o := r.Header.Get("Origin"); o != "" && !slices.Contains(origins, o) {
					httpx.WriteError(w, r, logger, httpx.NewError(http.StatusForbidden, "CSRF_FAILED", "This request's origin is not allowed.", nil))
					return
				}
			}
			if slices.Contains(publicPaths, r.URL.Path) {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			var token string
			if c, err := r.Cookie(CookieName); err == nil {
				token = c.Value
			}
			sess, err := svc.ResolveSession(ctx, token)
			if err != nil {
				httpx.WriteError(w, r, logger, mapError(err))
				return
			}
			if unsafeMethod(r.Method) && !svc.CheckCSRF(sess, r.Header.Get(csrfHeader)) {
				httpx.WriteError(w, r, logger, httpx.NewError(http.StatusForbidden, "CSRF_FAILED", "The CSRF token is missing or invalid.", nil))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sessionKey{}, sess)))
		})
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, service.ErrUnauthenticated):
		return httpx.NewError(http.StatusUnauthorized, httpx.CodeUnauthenticated, "Sign in with your staff passkey.", err)
	case errors.Is(err, service.ErrPasskey):
		return httpx.NewError(http.StatusUnauthorized, httpx.CodeUnauthenticated, "The passkey could not be verified. Try again.", err)
	case errors.Is(err, service.ErrRateLimited):
		return httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited, "Too many attempts. Try again in a minute.", err).
			WithHeader("Retry-After", "60")
	case errors.Is(err, service.ErrForbidden):
		return httpx.NewError(http.StatusForbidden, httpx.CodeForbidden, capitalize(strings.TrimPrefix(err.Error(), service.ErrForbidden.Error()+": "))+".", err)
	case errors.Is(err, service.ErrNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, service.ErrTenantNotActive):
		return httpx.Conflict("The tenant is not active.", err)
	case errors.Is(err, service.ErrConflict):
		return httpx.Conflict(capitalize(strings.TrimPrefix(err.Error(), service.ErrConflict.Error()+": "))+".", err)
	case errors.Is(err, service.ErrInvalid):
		return httpx.InvalidRequest(capitalize(strings.TrimPrefix(err.Error(), service.ErrInvalid.Error()+": "))+".", err)
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

// Handlers implements the admin API.
type Handlers struct {
	svc    *service.Service
	secure bool
}

var _ adminapi.StrictServerInterface = (*Handlers)(nil)

// New returns the handlers. secure sets the cookie's Secure attribute.
func New(svc *service.Service, secure bool) *Handlers { return &Handlers{svc: svc, secure: secure} }

func (h *Handlers) cookie(token string, maxAge int) string {
	c := http.Cookie{ //nolint:gosec // HttpOnly and SameSite=Strict are fixed; Secure is validated in config
		Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: h.secure,
		SameSite: http.SameSiteStrictMode, MaxAge: maxAge,
	}
	return c.String()
}

func sessionToAPI(s service.Session) adminapi.StaffSession {
	return adminapi.StaffSession{
		Staff: adminapi.Staff{
			Id: s.Ref(), Email: s.Email, Name: s.Name, Role: adminapi.StaffRole(s.Role),
		},
		CsrfToken: s.CSRF, ExpiresAt: s.ExpiresAt, IdleExpiresAt: s.IdleExpiresAt,
	}
}

func credentialBytes(c map[string]any) ([]byte, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, httpx.InvalidRequest("The credential is invalid.", err)
	}
	return raw, nil
}

func session(ctx context.Context) (service.Session, error) {
	s, ok := sessionFrom(ctx)
	if !ok {
		return service.Session{}, mapError(service.ErrUnauthenticated)
	}
	return s, nil
}

// StaffEnrollOptions implements POST /api/admin/v1/auth/enroll/options.
func (h *Handlers) StaffEnrollOptions(ctx context.Context, req adminapi.StaffEnrollOptionsRequestObject) (adminapi.StaffEnrollOptionsResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	id, opts, err := h.svc.EnrollOptions(ctx, req.Body.Token, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	var options map[string]any
	if err := json.Unmarshal(opts, &options); err != nil {
		return nil, err
	}
	return adminapi.StaffEnrollOptions200JSONResponse{CeremonyId: id, Options: options}, nil
}

// StaffEnroll implements POST /api/admin/v1/auth/enroll.
func (h *Handlers) StaffEnroll(ctx context.Context, req adminapi.StaffEnrollRequestObject) (adminapi.StaffEnrollResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	var name string
	if req.Body.Name != nil {
		name = *req.Body.Name
	}
	sess, token, err := h.svc.Enroll(ctx, req.Body.Token, req.Body.CeremonyId, name, cred, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.StaffEnroll200JSONResponse{
		Body:    sessionToAPI(sess),
		Headers: adminapi.StaffEnroll200ResponseHeaders{SetCookie: new(h.cookie(token, h.maxAge()))},
	}, nil
}

// StaffLoginOptions implements POST /api/admin/v1/auth/login/options.
func (h *Handlers) StaffLoginOptions(ctx context.Context, _ adminapi.StaffLoginOptionsRequestObject) (adminapi.StaffLoginOptionsResponseObject, error) {
	id, opts, err := h.svc.LoginOptions(ctx, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	var options map[string]any
	if err := json.Unmarshal(opts, &options); err != nil {
		return nil, err
	}
	return adminapi.StaffLoginOptions200JSONResponse{CeremonyId: id, Options: options}, nil
}

// StaffLogin implements POST /api/admin/v1/auth/login.
func (h *Handlers) StaffLogin(ctx context.Context, req adminapi.StaffLoginRequestObject) (adminapi.StaffLoginResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	sess, token, err := h.svc.Login(ctx, req.Body.CeremonyId, cred, clientFrom(ctx))
	if err != nil {
		return nil, mapError(err)
	}
	return adminapi.StaffLogin200JSONResponse{
		Body:    sessionToAPI(sess),
		Headers: adminapi.StaffLogin200ResponseHeaders{SetCookie: new(h.cookie(token, h.maxAge()))},
	}, nil
}

func (h *Handlers) maxAge() int { return int(h.svc.MaxAge().Seconds()) }

// GetStaffSession implements GET /api/admin/v1/auth/session.
func (h *Handlers) GetStaffSession(ctx context.Context, _ adminapi.GetStaffSessionRequestObject) (adminapi.GetStaffSessionResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	return adminapi.GetStaffSession200JSONResponse(sessionToAPI(s)), nil
}

// StaffLogout implements POST /api/admin/v1/auth/logout.
func (h *Handlers) StaffLogout(ctx context.Context, _ adminapi.StaffLogoutRequestObject) (adminapi.StaffLogoutResponseObject, error) {
	s, err := session(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Logout(ctx, s); err != nil {
		return nil, mapError(err)
	}
	return adminapi.StaffLogout204Response{Headers: adminapi.StaffLogout204ResponseHeaders{SetCookie: new(h.cookie("", -1))}}, nil
}

// ListAdminTenants implements GET /api/admin/v1/tenants.
func (h *Handlers) ListAdminTenants(ctx context.Context, _ adminapi.ListAdminTenantsRequestObject) (adminapi.ListAdminTenantsResponseObject, error) {
	rows, err := h.svc.TenantDirectory(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.ListAdminTenants200JSONResponse{Data: make([]adminapi.AdminTenant, len(rows))}
	for i, t := range rows {
		out.Data[i] = tenantToAPI(t)
	}
	return out, nil
}

// GetSecurityOverview implements GET /api/admin/v1/security.
func (h *Handlers) GetSecurityOverview(ctx context.Context, _ adminapi.GetSecurityOverviewRequestObject) (adminapi.GetSecurityOverviewResponseObject, error) {
	since, rows, err := h.svc.Security(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	out := adminapi.GetSecurityOverview200JSONResponse{Since: since, Data: make([]adminapi.TenantSecurity, len(rows))}
	for i, r := range rows {
		out.Data[i] = adminapi.TenantSecurity{
			TenantId: ids.Format(ids.PrefixTenant, r.TenantID), TenantName: r.TenantName, TenantStatus: r.TenantStatus,
			DegradedEvaluations: r.DegradedEvaluations, OpenSecurityAlerts: r.OpenSecurityAlerts,
			FailedLogins: r.FailedLogins, DisabledWebhooks: r.DisabledWebhooks,
		}
	}
	return out, nil
}
