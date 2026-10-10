package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

// Pool is the database pool (*pgxpool.Pool) connected as the application role.
type Pool = store.Pool

// Staff is a staff member.
type Staff = service.Staff

// Errors for operator tooling.
var (
	ErrNotFound = service.ErrNotFound
	ErrInvalid  = service.ErrInvalid
	ErrConflict = service.ErrConflict
)

// Config configures the admin realm.
type Config struct {
	// Secret derives the realm's CSRF key: the console secret.
	Secret []byte
	// SecretPrevious is the console secret before a rotation, or nil (ADR-0008).
	SecretPrevious []byte
	// RPID and Origins configure passkeys and allowed origins: the console's.
	RPID    string
	Origins []string
	// CookieSecure sets Secure on the staff session cookie (required in production).
	CookieSecure bool
	// Limiter throttles passkey ceremonies per client address; nil disables throttling.
	Limiter *ratelimit.Limiter
	Logger  *slog.Logger
}

// Module is the admin realm.
type Module struct {
	svc     *service.Service
	h       *handler.Handlers
	origins []string
	logger  *slog.Logger
}

// limiter adapts the platform limiter: 30 ceremonies per minute per address, failing open.
type limiter struct{ l *ratelimit.Limiter }

func (a limiter) Allow(ctx context.Context, key string) bool {
	d, err := a.l.Allow(ctx, ratelimit.Rule{Key: key, Limit: ratelimit.Limit{PerSecond: 0.5, Burst: 30}})
	return err != nil || d.Allowed
}

// New returns the admin realm.
func New(pool Pool, tenantsModule *tenants.Module, runner txn.Runner, rec audit.Recorder, clk clock.Clock, cfg Config) (*Module, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	sc := service.DefaultConfig()
	sc.Secret, sc.SecretPrevious, sc.RPID, sc.Origins = cfg.Secret, cfg.SecretPrevious, cfg.RPID, cfg.Origins
	var lim service.Limiter
	if cfg.Limiter != nil {
		lim = limiter{cfg.Limiter}
	}
	svc, err := service.New(store.New(pool), tenantsModule,
		service.TenantErrors{NotFound: tenants.ErrNotFound, NotActive: tenants.ErrNotActive},
		runner, rec, clk, lim, sc, logger)
	if err != nil {
		return nil, err
	}
	return &Module{svc: svc, h: handler.New(svc, cfg.CookieSecure), origins: cfg.Origins, logger: logger}, nil
}

// Middleware authenticates admin realm requests (paths under /api/admin/).
func (m *Module) Middleware() func(http.Handler) http.Handler {
	return handler.Middleware(m.svc, m.origins, m.logger)
}

// Mount adds the admin API's routes to r.
func (m *Module) Mount(r chi.Router) {
	writeErr := func(w http.ResponseWriter, req *http.Request, err error) { httpx.WriteError(w, req, m.logger, err) }
	strict := adminapi.NewStrictHandlerWithOptions(m.h, nil, adminapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeErr(w, req, httpx.InvalidRequest("The request is invalid.", err))
		},
		ResponseErrorHandlerFunc: writeErr,
	})
	adminapi.HandlerWithOptions(strict, adminapi.ChiServerOptions{
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeErr(w, req, httpx.InvalidRequest("The request parameters are invalid.", err))
		},
	})
}

// SupportScope implements auth.SupportResolver: a read-only tenant scope for a staff member's
// approved support access, recorded in the tenant's audit log.
func (m *Module) SupportScope(ctx context.Context, staffToken, grant string, kind tenancy.EnvironmentKind, request string) (tenancy.Scope, error) {
	scope, err := m.svc.SupportScope(ctx, staffToken, grant, kind, request)
	switch {
	case err == nil:
		return scope, nil
	case errors.Is(err, service.ErrUnauthenticated):
		return tenancy.Scope{}, auth.ErrStaffUnauthenticated
	case errors.Is(err, service.ErrForbidden):
		return tenancy.Scope{}, auth.ErrSupportDenied
	default:
		return tenancy.Scope{}, err
	}
}

var _ auth.SupportResolver = (*Module)(nil)

// CreateStaff creates a staff member and returns their enrollment token (operator CLI).
func (m *Module) CreateStaff(ctx context.Context, email, name string, role authz.Role, operator string) (string, error) {
	return m.svc.CreateStaff(ctx, email, name, role, operator)
}

// ReissueEnrollment returns a new enrollment token for an active staff member (operator CLI).
func (m *Module) ReissueEnrollment(ctx context.Context, email, operator string) (string, error) {
	return m.svc.ReissueEnrollment(ctx, email, operator)
}

// SetStaffStatus enables or disables a staff member (operator CLI).
func (m *Module) SetStaffStatus(ctx context.Context, email string, active bool, operator string) error {
	return m.svc.SetStaffStatus(ctx, email, active, operator)
}

// ListStaff returns every staff member (operator CLI).
func (m *Module) ListStaff(ctx context.Context) ([]Staff, error) { return m.svc.ListStaff(ctx) }

// EnrollURL builds the enrollment link for a token; the token is in the fragment.
func EnrollURL(consoleURL, token string) string {
	return trimSlash(consoleURL) + "/admin/enroll#token=" + token
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// EnrollTTL is how long an enrollment link stays valid.
const EnrollTTL = 24 * time.Hour
