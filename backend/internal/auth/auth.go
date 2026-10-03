package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/breach"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/mail"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Public types and errors.
type (
	// ConsoleConfig configures the dashboard realm.
	ConsoleConfig = service.ConsoleConfig
	// CookieConfig configures the console session cookie.
	CookieConfig = handler.CookieConfig
	// Limiter throttles console sign-in attempts.
	Limiter = service.Limiter
	// Role is a console user's role.
	Role = authz.Role
	// CreateKeyInput describes an API key to create.
	CreateKeyInput = service.CreateKeyInput
	// CreatedKey is a new API key with the only copy of its secret.
	CreatedKey = domain.CreatedKey
	// TenantCreator creates self-serve tenants (the tenants module, adapted by the composition
	// root).
	TenantCreator = service.TenantCreator
	// GoogleConfig registers RiskOS with Google for signing in and up.
	GoogleConfig = service.GoogleConfig
	// SignupEmailArgs is the job that emails a sign-up's code.
	SignupEmailArgs = service.SignupEmailArgs
)

// Errors.
var (
	ErrUnauthenticated = domain.ErrUnauthenticated
	ErrTenantSuspended = domain.ErrTenantSuspended
	ErrInvalidInput    = domain.ErrInvalidInput
	ErrEmailTaken      = domain.ErrEmailTaken
	// ErrStaffUnauthenticated and ErrSupportDenied are what a SupportResolver returns when the
	// staff session is missing or the support access cannot be used.
	ErrStaffUnauthenticated = domain.ErrStaffUnauthenticated
	ErrSupportDenied        = domain.ErrSupportDenied
)

// SupportResolver authorizes RiskOS staff's read-only support access to a tenant.
type SupportResolver = handler.SupportResolver

// CredentialHandlers are the auth operations of the public API.
type CredentialHandlers interface {
	GetWhoami(ctx context.Context, req apigen.GetWhoamiRequestObject) (apigen.GetWhoamiResponseObject, error)
	ListAPIKeys(ctx context.Context, req apigen.ListAPIKeysRequestObject) (apigen.ListAPIKeysResponseObject, error)
	CreateAPIKey(ctx context.Context, req apigen.CreateAPIKeyRequestObject) (apigen.CreateAPIKeyResponseObject, error)
	RotateAPIKey(ctx context.Context, req apigen.RotateAPIKeyRequestObject) (apigen.RotateAPIKeyResponseObject, error)
	RevokeAPIKey(ctx context.Context, req apigen.RevokeAPIKeyRequestObject) (apigen.RevokeAPIKeyResponseObject, error)
}

// ConsoleHandlers are the session and team operations of the public API.
type ConsoleHandlers interface {
	Login(ctx context.Context, req apigen.LoginRequestObject) (apigen.LoginResponseObject, error)
	GetSession(ctx context.Context, req apigen.GetSessionRequestObject) (apigen.GetSessionResponseObject, error)
	Logout(ctx context.Context, req apigen.LogoutRequestObject) (apigen.LogoutResponseObject, error)
	VerifyTotp(ctx context.Context, req apigen.VerifyTotpRequestObject) (apigen.VerifyTotpResponseObject, error)
	VerifyRecoveryCode(ctx context.Context, req apigen.VerifyRecoveryCodeRequestObject) (apigen.VerifyRecoveryCodeResponseObject, error)
	EnrollTotp(ctx context.Context, req apigen.EnrollTotpRequestObject) (apigen.EnrollTotpResponseObject, error)
	ConfirmTotp(ctx context.Context, req apigen.ConfirmTotpRequestObject) (apigen.ConfirmTotpResponseObject, error)
	PasskeyRegistrationOptions(ctx context.Context, req apigen.PasskeyRegistrationOptionsRequestObject) (apigen.PasskeyRegistrationOptionsResponseObject, error)
	RegisterPasskey(ctx context.Context, req apigen.RegisterPasskeyRequestObject) (apigen.RegisterPasskeyResponseObject, error)
	PasskeyLoginOptions(ctx context.Context, req apigen.PasskeyLoginOptionsRequestObject) (apigen.PasskeyLoginOptionsResponseObject, error)
	VerifyPasskey(ctx context.Context, req apigen.VerifyPasskeyRequestObject) (apigen.VerifyPasskeyResponseObject, error)
	DeletePasskey(ctx context.Context, req apigen.DeletePasskeyRequestObject) (apigen.DeletePasskeyResponseObject, error)
	RegenerateRecoveryCodes(ctx context.Context, req apigen.RegenerateRecoveryCodesRequestObject) (apigen.RegenerateRecoveryCodesResponseObject, error)
	AcceptInvite(ctx context.Context, req apigen.AcceptInviteRequestObject) (apigen.AcceptInviteResponseObject, error)
	StartSSO(ctx context.Context, req apigen.StartSSORequestObject) (apigen.StartSSOResponseObject, error)
	FinishSSO(ctx context.Context, req apigen.FinishSSORequestObject) (apigen.FinishSSOResponseObject, error)
	GetSSO(ctx context.Context, req apigen.GetSSORequestObject) (apigen.GetSSOResponseObject, error)
	PutSSO(ctx context.Context, req apigen.PutSSORequestObject) (apigen.PutSSOResponseObject, error)
	DeleteSSO(ctx context.Context, req apigen.DeleteSSORequestObject) (apigen.DeleteSSOResponseObject, error)
	GetSignupOptions(ctx context.Context, req apigen.GetSignupOptionsRequestObject) (apigen.GetSignupOptionsResponseObject, error)
	StartSignup(ctx context.Context, req apigen.StartSignupRequestObject) (apigen.StartSignupResponseObject, error)
	ResendSignupCode(ctx context.Context, req apigen.ResendSignupCodeRequestObject) (apigen.ResendSignupCodeResponseObject, error)
	VerifySignup(ctx context.Context, req apigen.VerifySignupRequestObject) (apigen.VerifySignupResponseObject, error)
	CompleteSignup(ctx context.Context, req apigen.CompleteSignupRequestObject) (apigen.CompleteSignupResponseObject, error)
	StartGoogle(ctx context.Context, req apigen.StartGoogleRequestObject) (apigen.StartGoogleResponseObject, error)
	FinishGoogle(ctx context.Context, req apigen.FinishGoogleRequestObject) (apigen.FinishGoogleResponseObject, error)
	ListTeamMembers(ctx context.Context, req apigen.ListTeamMembersRequestObject) (apigen.ListTeamMembersResponseObject, error)
	UpdateTeamMember(ctx context.Context, req apigen.UpdateTeamMemberRequestObject) (apigen.UpdateTeamMemberResponseObject, error)
	ListInvites(ctx context.Context, req apigen.ListInvitesRequestObject) (apigen.ListInvitesResponseObject, error)
	CreateInvite(ctx context.Context, req apigen.CreateInviteRequestObject) (apigen.CreateInviteResponseObject, error)
	RevokeInvite(ctx context.Context, req apigen.RevokeInviteRequestObject) (apigen.RevokeInviteResponseObject, error)
}

// Pool runs the pre-authentication lookups. *pgxpool.Pool connected as the application role
// satisfies it.
type Pool = store.Querier

// Config configures the auth module.
type Config struct {
	// Pepper keys the HMAC of stored API key secrets. At least 32 bytes.
	Pepper []byte
	// CacheTTL bounds API key lookup caching (the revocation staleness bound).
	CacheTTL time.Duration
	// Console configures the dashboard realm. Zero values take DefaultConsole's; without a
	// Secret a random one is used (sessions then do not survive a restart), which the API's
	// configuration validation refuses outside tests.
	Console ConsoleConfig
	Cookie  CookieConfig
	// Limiter throttles sign-in; nil disables throttling (tests).
	Limiter Limiter
	Logger  *slog.Logger
	// InviteEmails queues an email for each new invite and each sign-up code; nil when email is
	// not configured (sign-up with email is then unavailable).
	InviteEmails jobs.Enqueuer
	// Signup enables self-serve sign-up: Tenants creates workspaces (nil: sign-up off), Google
	// (optional) enables signing in and up with Google.
	Signup struct {
		Tenants TenantCreator
		Google  *GoogleConfig
	}
}

// InviteEmailArgs is the job that emails an invite's link.
type InviteEmailArgs = service.InviteEmailArgs

// BreachChecker reports whether a password appears in known data breaches.
type BreachChecker = service.BreachChecker

// PwnedPasswords returns a breach checker on the Pwned Passwords range API at baseURL
// (breach.DefaultURL in production), with k-anonymity.
func PwnedPasswords(baseURL string, timeout time.Duration) BreachChecker {
	if baseURL == "" {
		baseURL = breach.DefaultURL
	}
	return breach.New(baseURL, timeout)
}

// DefaultConsole returns the dashboard realm defaults (ARCHITECTURE.md §9, §16.7).
func DefaultConsole() ConsoleConfig {
	return ConsoleConfig{
		SessionTTL: 12 * time.Hour, IdleTimeout: 30 * time.Minute, PendingTTL: 15 * time.Minute,
		ReauthWindow: 10 * time.Minute, InviteTTL: 72 * time.Hour,
		RPID: "localhost", Origins: []string{"http://localhost:3000"},
	}
}

// Module is the auth module.
type Module struct {
	svc     *service.Service
	console *service.Console
	cookie  CookieConfig
	clock   clock.Clock
	support SupportResolver
}

// PasskeyRP returns the console's passkey relying party ID and allowed origins; the admin realm
// uses the same.
func (m *Module) PasskeyRP() (string, []string) { return m.console.PasskeyRP() }

// SetSupport lets RiskOS staff with approved support access read a tenant through the customer
// API (the admin module). Call before Middleware.
func (m *Module) SetSupport(s SupportResolver) { m.support = s }

type noLimit struct{}

func (noLimit) Allow(context.Context, ...ratelimit.Rule) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true}, nil
}

// New returns the auth module.
func New(pool Pool, runner txn.Runner, rec audit.Recorder, clk clock.Clock, cfg Config) *Module {
	st := store.New(pool)
	cc := withConsoleDefaults(cfg.Console)
	if cc.SSORedirectURL == "" && cfg.Cookie.ConsoleURL != "" {
		cc.SSORedirectURL = strings.TrimRight(cfg.Cookie.ConsoleURL, "/") + "/sso/callback"
	}
	if cfg.Limiter == nil {
		cfg.Limiter = noLimit{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	con, err := service.NewConsole(st, runner, rec, clk, cfg.Limiter, cc, logger)
	if err != nil {
		// Only an invalid relying party configuration can fail here, and configuration
		// validation rejects it before this point.
		panic(fmt.Sprintf("auth: console configuration: %v", err))
	}
	if cfg.InviteEmails != nil {
		con.SetInviteEmails(cfg.InviteEmails)
	}
	if cfg.Signup.Tenants != nil {
		google := cfg.Signup.Google
		if google != nil && google.RedirectURL == "" && cfg.Cookie.ConsoleURL != "" {
			g := *google
			g.RedirectURL = strings.TrimRight(cfg.Cookie.ConsoleURL, "/") + "/signup/google"
			google = &g
		}
		con.SetSignup(service.SignupConfig{Tenants: cfg.Signup.Tenants, Google: google})
	}
	cookie := cfg.Cookie
	if cookie.Name == "" {
		cookie.Name = "riskos_session"
	}
	if cookie.MaxAge == 0 {
		cookie.MaxAge = cc.SessionTTL
	}
	if cookie.Origins == nil {
		cookie.Origins = cc.Origins
	}
	return &Module{
		svc:     service.New(st, runner, rec, clk, service.Config{Pepper: cfg.Pepper, CacheTTL: cfg.CacheTTL}),
		console: con, cookie: cookie, clock: clk,
	}
}

func withConsoleDefaults(c ConsoleConfig) ConsoleConfig {
	d := DefaultConsole()
	if len(c.Secret) == 0 {
		c.Secret = make([]byte, 32)
		_, _ = rand.Read(c.Secret)
	}
	for dst, def := range map[*time.Duration]time.Duration{
		&c.SessionTTL: d.SessionTTL, &c.IdleTimeout: d.IdleTimeout, &c.PendingTTL: d.PendingTTL,
		&c.ReauthWindow: d.ReauthWindow, &c.InviteTTL: d.InviteTTL,
	} {
		if *dst == 0 {
			*dst = def
		}
	}
	if c.RPID == "" {
		c.RPID = d.RPID
	}
	if len(c.Origins) == 0 {
		c.Origins = d.Origins
	}
	return c
}

// Middleware authenticates /api/v1 requests (API key or console session) and sets the scope.
func (m *Module) Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return handler.Middleware(m.svc, m.console, m.cookie, m.support, logger)
}

// ReauthOperations are the operations a console user may only perform shortly after verifying
// a second factor again (ARCHITECTURE.md §16.7). Names are the generated server's method names
// (the strict middleware's operation IDs); a test checks each exists.
func ReauthOperations() []string {
	return []string{
		"CreateAPIKey", "RotateAPIKey", "RevokeAPIKey", "CreateWebhook", "UpdateWebhook", "RotateWebhookSecret",
		"PutRiskSettings", "CreateRule", "UpdateRule", "RollbackRule", "ApproveRuleChange", "CreateDenyEntry", "DeleteDenyEntry", "CreateInvite", "UpdateTeamMember", "PutSSO", "DeleteSSO",
		"PutComplianceProfile", "PutPackRuleConfig", "RollbackPackRule", "ApprovePackRuleChange",
		"PutComplianceFXRate", "DeleteComplianceFXRate", "ApproveComplianceReport", "FileComplianceReport",
		"ConfirmSuspicion",
	}
}

// ReauthMiddleware enforces ReauthOperations for console sessions.
func (m *Module) ReauthMiddleware() apigen.StrictMiddlewareFunc {
	return handler.ReauthMiddleware(m.console, ReauthOperations())
}

// HTTPHandlers returns the module's API key handlers.
func (m *Module) HTTPHandlers() CredentialHandlers { return handler.New(m.svc, m.clock) }

// ConsoleHTTPHandlers returns the session and team handlers.
func (m *Module) ConsoleHTTPHandlers() ConsoleHandlers {
	return handler.NewConsole(m.console, m.cookie)
}

// CreateKey creates an API key in the environment of the scope in ctx. Used by operator tooling
// to issue a tenant's first key; customers use the API.
func (m *Module) CreateKey(ctx context.Context, in CreateKeyInput) (CreatedKey, error) {
	return m.svc.CreateKey(ctx, in)
}

// Invite creates an invite in the tenant of the scope in ctx and returns its link (shown once).
// Operator tooling uses it to invite a tenant's first administrator.
func (m *Module) Invite(ctx context.Context, email string, role Role) (string, error) {
	inv, err := m.console.CreateInvite(ctx, email, role)
	if err != nil {
		return "", err
	}
	return handler.InviteURL(m.cookie.ConsoleURL, inv.Token), nil
}

// InviteEmailJob returns the worker handler for InviteEmailArgs. Invites accepted, revoked or
// expired before the job runs are skipped. Delivery is at least once: a retry after a send whose
// confirmation was lost sends the same link again.
func (m *Module) InviteEmailJob(sender mail.Sender) func(context.Context, InviteEmailArgs) error {
	return func(ctx context.Context, a InviteEmailArgs) error {
		inv, err := m.console.PendingInviteForEmail(ctx, a)
		if err != nil || inv == nil {
			return err
		}
		if err := sender.Send(ctx, inviteMessage(inv, handler.InviteURL(m.cookie.ConsoleURL, inv.Token))); err != nil {
			return err
		}
		return m.console.InviteEmailed(ctx, a)
	}
}

func inviteMessage(inv *service.PendingInvite, link string) mail.Message {
	role := strings.ReplaceAll(string(inv.Role), "_", " ")
	return mail.Message{
		To:      inv.Email,
		Subject: "You're invited to " + oneLine(inv.TenantName) + " on RiskOS",
		Text: "You have been invited to join " + oneLine(inv.TenantName) + " on RiskOS as " + role + ".\n\n" +
			"Accept the invite and choose a password:\n" + link + "\n\n" +
			"The link works once and expires on " + inv.ExpiresAt.UTC().Format("2 January 2006 at 15:04 UTC") + ".\n" +
			"You will also set up a passkey or an authenticator app when you first sign in.\n\n" +
			"If you did not expect this invite, ignore this email.\n",
	}
}

// SignupEmailJob returns the worker handler for SignupEmailArgs: the sign-up's code, or, when the
// email already has an account, a note saying so. Sign-ups finished, expired or superseded by a
// newer code are skipped.
func (m *Module) SignupEmailJob(sender mail.Sender) func(context.Context, SignupEmailArgs) error {
	return func(ctx context.Context, a SignupEmailArgs) error {
		p, err := m.console.SignupEmailFor(ctx, a)
		if err != nil || p == nil {
			return err
		}
		return sender.Send(ctx, signupMessage(p, m.cookie.ConsoleURL))
	}
}

func signupMessage(p *service.PendingSignupEmail, consoleURL string) mail.Message {
	signIn := strings.TrimRight(consoleURL, "/") + "/login"
	if p.Code == "" {
		return mail.Message{
			To:      p.Email,
			Subject: "You already have a RiskOS account",
			Text: "Hello " + oneLine(p.Name) + ",\n\n" +
				"Someone, probably you, tried to create a RiskOS workspace with this email address. It " +
				"already has an account, so no new workspace was created.\n\n" +
				"Sign in instead:\n" + signIn + "\n\n" +
				"If this was not you, you can ignore this email.\n",
		}
	}
	return mail.Message{
		To:      p.Email,
		Subject: p.Code + " is your RiskOS verification code",
		Text: "Hello " + oneLine(p.Name) + ",\n\n" +
			"Enter this code to verify your email and create your RiskOS workspace:\n\n" +
			"    " + p.Code + "\n\n" +
			"It works until " + p.ExpiresAt.UTC().Format("15:04 UTC on 2 January 2006") + ". A newer code replaces it.\n\n" +
			"If you did not try to sign up, ignore this email: nothing is created without the code.\n",
	}
}

// oneLine keeps a tenant-chosen name on one line in a subject.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// DefaultCacheTTL is the default lookup cache TTL: the revocation staleness bound.
const DefaultCacheTTL = 5 * time.Second
