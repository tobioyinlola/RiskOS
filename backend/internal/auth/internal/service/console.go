package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/console"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/sso"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Session states.
const (
	StateMFARequired        = "mfa_required"
	StateEnrollmentRequired = "enrollment_required"
	StateActive             = "active"
)

// Limiter throttles sign-in and verification attempts.
type Limiter interface {
	Allow(ctx context.Context, rules ...ratelimit.Rule) (ratelimit.Decision, error)
}

// ConsoleConfig configures the dashboard realm.
type ConsoleConfig struct {
	// Secret keys MFA secret encryption and CSRF tokens (at least 32 bytes, from the secrets
	// manager).
	Secret []byte
	// SessionTTL is the absolute lifetime of a signed-in session; IdleTimeout ends one unused
	// for that long.
	SessionTTL  time.Duration
	IdleTimeout time.Duration
	// PendingTTL bounds a session waiting for its second factor or enrollment.
	PendingTTL time.Duration
	// ReauthWindow is how long a second-factor confirmation allows high-impact actions.
	ReauthWindow time.Duration
	// InviteTTL is how long an invite link stays valid.
	InviteTTL time.Duration
	// RPID and Origins identify the console to passkeys (WebAuthn relying party).
	RPID    string
	Origins []string
	// Breach rejects passwords found in known breaches; nil disables the check.
	Breach BreachChecker
	// SSORedirectURL is the console's SSO callback, registered at identity providers.
	SSORedirectURL string
	// SSOAllowPrivate lets identity providers be http and on private addresses (local
	// development and tests only; configuration refuses it in production).
	SSOAllowPrivate bool
}

// BreachChecker reports whether a password appears in known data breaches.
type BreachChecker interface {
	Breached(ctx context.Context, password string) (bool, error)
}

// Session is a resolved console session.
type Session struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	TenantName      string
	UserID          uuid.UUID
	State           string
	Role            authz.Role
	CreatedAt       time.Time
	ExpiresAt       time.Time
	ReauthAt        *time.Time
	TestEnvID       uuid.UUID
	ProductionEnvID uuid.UUID
	// ProductionEnabled is false while a self-serve tenant's onboarding is not approved: the
	// session cannot act in production.
	ProductionEnabled bool
	// CSRF is the token the console must echo in X-CSRF-Token on unsafe requests.
	CSRF string
}

// ReauthFresh reports whether the session confirmed a second factor within window.
func (s Session) ReauthFresh(now time.Time, window time.Duration) bool {
	return s.ReauthAt != nil && now.Sub(*s.ReauthAt) < window
}

// Console implements the dashboard realm: sign-in, MFA, sessions, invites and team management.
type Console struct {
	store   store.Store
	runner  txn.Runner
	audit   audit.Recorder
	clock   clock.Clock
	limiter Limiter
	sealer  *console.Sealer
	csrfKey []byte
	wa      *webauthn.WebAuthn
	cfg     ConsoleConfig
	logger  *slog.Logger
	// inviteKey derives invite tokens (console.InviteToken).
	inviteKey []byte
	// emails queues invite emails; nil when email is not configured.
	emails jobs.Enqueuer
	// sso talks to identity providers; ssoKey derives each request's PKCE verifier and nonce.
	sso    *sso.Client
	ssoKey []byte
	// signup configures self-serve sign-up; signupKey derives its emailed codes.
	signup    SignupConfig
	signupKey []byte
}

// PasskeyRP returns the passkey relying party ID and allowed origins.
func (c *Console) PasskeyRP() (string, []string) { return c.cfg.RPID, c.cfg.Origins }

// SetInviteEmails makes new invites queue an email through q.
func (c *Console) SetInviteEmails(q jobs.Enqueuer) { c.emails = q }

// ssoTimeout bounds each request to an identity provider.
const ssoTimeout = 5 * time.Second

// NewConsole returns the dashboard realm service.
func NewConsole(st store.Store, runner txn.Runner, rec audit.Recorder, clk clock.Clock, limiter Limiter, cfg ConsoleConfig, logger *slog.Logger) (*Console, error) {
	sealer, err := console.NewSealer(cfg.Secret)
	if err != nil {
		return nil, err
	}
	csrf := hmac.New(sha256.New, cfg.Secret)
	csrf.Write([]byte("riskos-csrf-key"))
	invite := hmac.New(sha256.New, cfg.Secret)
	invite.Write([]byte("riskos-invite-key"))
	ssoKey := hmac.New(sha256.New, cfg.Secret)
	ssoKey.Write([]byte("riskos-sso-key"))
	signupKey := hmac.New(sha256.New, cfg.Secret)
	signupKey.Write([]byte("riskos-signup-code-key"))
	wa, err := webauthn.New(&webauthn.Config{RPID: cfg.RPID, RPDisplayName: "RiskOS", RPOrigins: cfg.Origins})
	if err != nil {
		return nil, fmt.Errorf("configure passkeys: %w", err)
	}
	return &Console{
		store: st, runner: runner, audit: rec, clock: clk, limiter: limiter, sealer: sealer,
		csrfKey: csrf.Sum(nil), wa: wa, cfg: cfg, logger: logger, inviteKey: invite.Sum(nil),
		sso: sso.New(ssoTimeout, cfg.SSOAllowPrivate), ssoKey: ssoKey.Sum(nil), signupKey: signupKey.Sum(nil),
	}, nil
}

func (c *Console) csrfFor(sessionID uuid.UUID) string {
	m := hmac.New(sha256.New, c.csrfKey)
	m.Write(sessionID[:])
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// CheckCSRF verifies a presented CSRF token for a session in constant time.
func (c *Console) CheckCSRF(sess Session, token string) bool {
	return token != "" && hmac.Equal([]byte(token), []byte(c.csrfFor(sess.ID)))
}

// ReauthWindow returns the configured window.
func (c *Console) ReauthWindow() time.Duration { return c.cfg.ReauthWindow }

// userPrincipal is how a user appears in scopes and the audit log.
func userPrincipal(userID uuid.UUID, role authz.Role) tenancy.Principal {
	return tenancy.Principal{Type: tenancy.PrincipalUser, ID: ids.Format(ids.PrefixUser, userID), Permissions: authz.RolePermissions(role)}
}

// tenantCtx scopes ctx to a tenant (no environment) for the realm's own tenant-level tables.
func tenantCtx(ctx context.Context, tenantID uuid.UUID, p tenancy.Principal) context.Context {
	return tenancy.WithScope(ctx, tenancy.Scope{TenantID: tenantID, Principal: p})
}

func (c *Console) record(ctx context.Context, action, resourceType, resourceID string, outcome audit.Outcome, meta map[string]any) error {
	return c.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, Metadata: meta, TenantLevel: true,
	})
}

// ResolveSession finds and checks a session by its cookie token. Any failure is
// domain.ErrUnauthenticated (domain.ErrTenantSuspended for an inactive tenant).
func (c *Console) ResolveSession(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, domain.ErrUnauthenticated
	}
	r, err := c.store.SessionByToken(ctx, console.HashToken(token))
	if err != nil {
		return Session{}, err
	}
	now := c.clock.Now()
	switch {
	case r == nil, r.RevokedAt != nil, !now.Before(r.ExpiresAt), r.UserStatus != "active",
		r.State == StateActive && now.Sub(r.LastSeenAt) >= c.cfg.IdleTimeout:
		return Session{}, domain.ErrUnauthenticated
	case r.TenantStatus != tenantStatusActive:
		return Session{}, domain.ErrTenantSuspended
	}
	s := Session{
		ID: r.ID, TenantID: r.TenantID, TenantName: r.TenantName, UserID: r.UserID, State: r.State,
		Role: authz.Role(r.UserRole), CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, ReauthAt: r.ReauthAt,
		TestEnvID: r.TestEnvID, ProductionEnvID: r.ProductionEnvID, ProductionEnabled: r.ProductionEnabled, CSRF: c.csrfFor(r.ID),
	}
	if now.Sub(r.LastSeenAt) >= time.Minute {
		tctx := tenantCtx(ctx, s.TenantID, userPrincipal(s.UserID, s.Role))
		if err := c.runner.Run(tctx, func(ctx context.Context) error { return c.store.TouchSession(ctx, s.ID, now) }); err != nil {
			return Session{}, err
		}
	}
	return s, nil
}

// Scope returns the request scope of an active session in an environment.
func (c *Console) Scope(s Session, kind tenancy.EnvironmentKind) (tenancy.Scope, error) {
	if s.State != StateActive {
		return tenancy.Scope{}, domain.ErrUnauthenticated
	}
	env := s.TestEnvID
	if kind == tenancy.EnvironmentProduction {
		if !s.ProductionEnabled {
			return tenancy.Scope{}, domain.ErrProductionLocked
		}
		env = s.ProductionEnvID
	}
	return tenancy.Scope{TenantID: s.TenantID, EnvironmentID: env, EnvironmentKind: kind, Principal: userPrincipal(s.UserID, s.Role)}, nil
}

func (c *Console) throttle(ctx context.Context, rules ...ratelimit.Rule) error {
	d, err := c.limiter.Allow(ctx, rules...)
	if err != nil {
		// Consistent with API rate limiting: an outage of Redis must not lock everyone out.
		c.logger.WarnContext(ctx, "sign-in limiter unavailable; attempt allowed", slog.Any("error", err))
		return nil
	}
	if !d.Allowed {
		return &domain.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

func limitKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "rl:console:" + parts[0] + ":" + hex.EncodeToString(h[:12])
}

// Client describes where a request came from.
type Client struct {
	IP        *netip.Addr
	UserAgent string
}

func (cl Client) ipString() string {
	if cl.IP == nil {
		return "unknown"
	}
	return cl.IP.String()
}

// SignIn is a new session and its token (for the cookie; never stored).
type SignIn struct {
	Token   string
	Session Session
}

// Login verifies email and password and opens a session that still needs its second factor (or
// its first enrollment). Unknown emails, wrong passwords and disabled users are indistinguishable,
// in response and in time.
func (c *Console) Login(ctx context.Context, email, password string, cl Client) (SignIn, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("email", email), Limit: ratelimit.Limit{PerSecond: 10.0 / 900, Burst: 10}},
		ratelimit.Rule{Key: limitKey("ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 60.0 / 900, Burst: 60}},
	); err != nil {
		return SignIn{}, err
	}
	u, err := c.store.UserByEmail(ctx, email)
	if err != nil {
		return SignIn{}, err
	}
	if u == nil || u.PasswordHash == nil {
		// Unknown emails and Google-only accounts (no password) look the same.
		console.VerifyDummy(password)
		return SignIn{}, domain.ErrInvalidCredentials
	}
	ok, err := console.VerifyPassword(password, *u.PasswordHash)
	if err != nil {
		return SignIn{}, err
	}
	sys := tenancy.Principal{Type: tenancy.PrincipalUser, ID: ids.Format(ids.PrefixUser, u.ID)}
	tctx := tenantCtx(ctx, u.TenantID, sys)
	if !ok || u.Status != "active" {
		reason := "wrong_password"
		if ok {
			reason = "user_disabled"
		}
		if err := c.runner.Run(tctx, func(ctx context.Context) error {
			return c.record(ctx, "user.login_failed", "user", sys.ID, audit.OutcomeFailure, map[string]any{"reason": reason})
		}); err != nil {
			return SignIn{}, err
		}
		return SignIn{}, domain.ErrInvalidCredentials
	}
	if u.TenantStatus != tenantStatusActive {
		return SignIn{}, domain.ErrTenantSuspended
	}
	var out SignIn
	err = c.runner.Run(tctx, func(ctx context.Context) error {
		user, err := c.store.GetUser(ctx, u.ID, false)
		if err != nil {
			return err
		}
		creds, err := c.store.Credentials(ctx, u.ID)
		if err != nil {
			return err
		}
		state := StateEnrollmentRequired
		if user.TotpSecretEnc != nil || len(creds) > 0 {
			state = StateMFARequired
		}
		out, err = c.openSession(ctx, u.ID, state, cl)
		return err
	})
	return out, err
}

func (c *Console) openSession(ctx context.Context, userID uuid.UUID, state string, cl Client) (SignIn, error) {
	token, hash, err := console.NewToken()
	if err != nil {
		return SignIn{}, err
	}
	now := c.clock.Now()
	id := ids.NewV7()
	ua := cl.UserAgent
	if utf8.RuneCountInString(ua) > 256 {
		ua = string([]rune(ua)[:256])
	}
	if err := c.store.InsertSession(ctx, store.NewSession{
		ID: id, UserID: userID, TokenHash: hash, State: state, Now: now, ExpiresAt: now.Add(c.cfg.PendingTTL), IP: cl.IP, UserAgent: ua,
	}); err != nil {
		return SignIn{}, err
	}
	return SignIn{Token: token, Session: Session{ID: id, UserID: userID, State: state, ExpiresAt: now.Add(c.cfg.PendingTTL), CSRF: c.csrfFor(id)}}, nil
}

// sessionCtx scopes ctx to the session's tenant, acting as its user.
func (c *Console) sessionCtx(ctx context.Context, s Session) context.Context {
	return tenantCtx(ctx, s.TenantID, userPrincipal(s.UserID, s.Role))
}

// complete finishes a second-factor step: a pending session becomes active; an active one
// records a fresh confirmation (re-authentication).
func (c *Console) complete(ctx context.Context, s Session, method string) error {
	now := c.clock.Now()
	if s.State == StateActive {
		if err := c.store.SetReauth(ctx, s.ID, now); err != nil {
			return err
		}
		return c.record(ctx, "user.reauthenticated", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, map[string]any{"method": method})
	}
	if err := c.store.SetSessionState(ctx, s.ID, StateActive, now, now.Add(c.cfg.SessionTTL)); err != nil {
		return err
	}
	if err := c.store.TouchLogin(ctx, s.UserID, now); err != nil {
		return err
	}
	return c.record(ctx, "user.login", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, map[string]any{"method": method})
}

func requireState(s Session, states ...string) error {
	for _, st := range states {
		if s.State == st {
			return nil
		}
	}
	return domain.ErrSessionState
}

func (c *Console) throttleMFA(ctx context.Context, s Session) error {
	return c.throttle(ctx, ratelimit.Rule{Key: limitKey("mfa", s.UserID.String()), Limit: ratelimit.Limit{PerSecond: 5.0 / 300, Burst: 5}})
}

// VerifyTOTP checks an authenticator-app code: completing sign-in, or re-authenticating.
func (c *Console) VerifyTOTP(ctx context.Context, s Session, code string) error {
	if err := requireState(s, StateMFARequired, StateActive); err != nil {
		return err
	}
	if err := c.throttleMFA(ctx, s); err != nil {
		return err
	}
	return c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		u, err := c.store.GetUser(ctx, s.UserID, true)
		if err != nil {
			return err
		}
		if u.TotpSecretEnc == nil {
			return domain.ErrMFAInvalid
		}
		secret, err := c.sealer.Open(u.TotpSecretEnc, s.UserID[:])
		if err != nil {
			return err
		}
		var last int64
		if u.TotpLastStep != nil {
			last = *u.TotpLastStep
		}
		step, ok := console.VerifyTOTP(secret, code, c.clock.Now(), last)
		if !ok {
			return c.failMFA(ctx, s, "totp")
		}
		if err := c.store.SetTOTPStep(ctx, s.UserID, step); err != nil {
			return err
		}
		return c.complete(ctx, s, "totp")
	})
}

// failMFA audits a failed verification; the audit entry must commit even though the attempt
// fails, so it is written in its own transaction.
func (c *Console) failMFA(ctx context.Context, s Session, method string) error {
	// A fresh context on purpose: the caller's carries the transaction that is about to roll back.
	if err := c.runner.Run(c.sessionCtx(context.Background(), s), func(ctx context.Context) error { //nolint:contextcheck // see above
		return c.record(ctx, "user.mfa_failed", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeFailure, map[string]any{"method": method})
	}); err != nil {
		c.logger.WarnContext(ctx, "failed verification not audited", slog.Any("error", err))
	}
	return domain.ErrMFAInvalid
}

// UseRecoveryCode completes sign-in (or re-authenticates) with a one-time recovery code.
func (c *Console) UseRecoveryCode(ctx context.Context, s Session, code string) (remaining int, err error) {
	if err := requireState(s, StateMFARequired, StateActive); err != nil {
		return 0, err
	}
	if err := c.throttleMFA(ctx, s); err != nil {
		return 0, err
	}
	err = c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		u, err := c.store.GetUser(ctx, s.UserID, true)
		if err != nil {
			return err
		}
		want := console.HashRecoveryCode(code)
		idx := -1
		for i, h := range u.RecoveryHashes {
			if hmac.Equal(h, want) {
				idx = i
			}
		}
		if idx < 0 {
			return c.failMFA(ctx, s, "recovery_code")
		}
		left := append(append([][]byte{}, u.RecoveryHashes[:idx]...), u.RecoveryHashes[idx+1:]...)
		if err := c.store.SetRecoveryHashes(ctx, s.UserID, left, c.clock.Now()); err != nil {
			return err
		}
		remaining = len(left)
		return c.complete(ctx, s, "recovery_code")
	})
	return remaining, err
}

// ceremony is what a session holds between the two steps of an enrollment or passkey check.
type ceremony struct {
	Kind       string                `json:"kind"`
	TOTPSecret []byte                `json:"totp_secret,omitempty"` // sealed
	WebAuthn   *webauthn.SessionData `json:"webauthn,omitempty"`
}

func (c *Console) saveCeremony(ctx context.Context, s Session, cer ceremony) error {
	b, err := json.Marshal(cer)
	if err != nil {
		return fmt.Errorf("encode ceremony: %w", err)
	}
	return c.store.SetCeremony(ctx, s.ID, b)
}

func (c *Console) takeCeremony(ctx context.Context, s Session, kind string) (ceremony, error) {
	raw, err := c.store.Ceremony(ctx, s.ID)
	if err != nil {
		return ceremony{}, err
	}
	var cer ceremony
	if raw == nil || json.Unmarshal(raw, &cer) != nil || cer.Kind != kind {
		return ceremony{}, domain.ErrSessionState
	}
	return cer, c.store.SetCeremony(ctx, s.ID, nil)
}

// canEnroll: new users enroll during sign-in; signed-in users add factors after re-authenticating.
func (c *Console) canEnroll(s Session) error {
	switch {
	case s.State == StateEnrollmentRequired:
		return nil
	case s.State == StateActive && s.ReauthFresh(c.clock.Now(), c.cfg.ReauthWindow):
		return nil
	case s.State == StateActive:
		return domain.ErrReauthRequired
	default:
		return domain.ErrSessionState
	}
}

// TOTPEnrollment is a pending authenticator-app enrollment.
type TOTPEnrollment struct {
	Secret string
	URI    string
}

// BeginTOTP starts authenticator-app enrollment.
func (c *Console) BeginTOTP(ctx context.Context, s Session) (TOTPEnrollment, error) {
	if err := c.canEnroll(s); err != nil {
		return TOTPEnrollment{}, err
	}
	secret, err := console.NewTOTPSecret()
	if err != nil {
		return TOTPEnrollment{}, err
	}
	sealed, err := c.sealer.Seal(secret, s.UserID[:])
	if err != nil {
		return TOTPEnrollment{}, err
	}
	var email string
	err = c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		u, err := c.store.GetUser(ctx, s.UserID, false)
		if err != nil {
			return err
		}
		email = u.Email
		return c.saveCeremony(ctx, s, ceremony{Kind: "totp", TOTPSecret: sealed})
	})
	if err != nil {
		return TOTPEnrollment{}, err
	}
	return TOTPEnrollment{Secret: console.EncodeSecret(secret), URI: console.OTPAuthURI(secret, email)}, nil
}

// ConfirmTOTP finishes enrollment with a first code. It returns recovery codes (shown once)
// when the user had none.
func (c *Console) ConfirmTOTP(ctx context.Context, s Session, code string) ([]string, error) {
	if err := c.canEnroll(s); err != nil {
		return nil, err
	}
	var codes []string
	err := c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		cer, err := c.takeCeremony(ctx, s, "totp")
		if err != nil {
			return err
		}
		secret, err := c.sealer.Open(cer.TOTPSecret, s.UserID[:])
		if err != nil {
			return err
		}
		step, ok := console.VerifyTOTP(secret, code, c.clock.Now(), 0)
		if !ok {
			return domain.ErrMFAInvalid
		}
		if err := c.store.SetTOTP(ctx, s.UserID, cer.TOTPSecret, &step, c.clock.Now()); err != nil {
			return err
		}
		if codes, err = c.ensureRecoveryCodes(ctx, s); err != nil {
			return err
		}
		if err := c.record(ctx, "user.mfa_enrolled", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, map[string]any{"method": "totp"}); err != nil {
			return err
		}
		return c.complete(ctx, s, "totp_enrollment")
	})
	return codes, err
}

func (c *Console) ensureRecoveryCodes(ctx context.Context, s Session) ([]string, error) {
	u, err := c.store.GetUser(ctx, s.UserID, false)
	if err != nil || len(u.RecoveryHashes) > 0 {
		return nil, err
	}
	codes, hashes, err := console.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	return codes, c.store.SetRecoveryHashes(ctx, s.UserID, hashes, c.clock.Now())
}

// RegenerateRecoveryCodes replaces a user's recovery codes (shown once). The caller must have
// re-authenticated.
func (c *Console) RegenerateRecoveryCodes(ctx context.Context, s Session) ([]string, error) {
	if err := requireState(s, StateActive); err != nil {
		return nil, err
	}
	codes, hashes, err := console.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	err = c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		if err := c.store.SetRecoveryHashes(ctx, s.UserID, hashes, c.clock.Now()); err != nil {
			return err
		}
		return c.record(ctx, "user.recovery_codes_regenerated", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, nil)
	})
	return codes, err
}

// waUser adapts a user and their passkeys to the WebAuthn library.
type waUser struct {
	u     store.User
	creds []webauthn.Credential
}

func (w waUser) WebAuthnID() []byte                         { return w.u.ID[:] }
func (w waUser) WebAuthnName() string                       { return w.u.Email }
func (w waUser) WebAuthnDisplayName() string                { return w.u.Name }
func (w waUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }

func (c *Console) loadWAUser(ctx context.Context, userID uuid.UUID) (waUser, error) {
	u, err := c.store.GetUser(ctx, userID, false)
	if err != nil {
		return waUser{}, err
	}
	stored, err := c.store.Credentials(ctx, userID)
	if err != nil {
		return waUser{}, err
	}
	w := waUser{u: u}
	for _, sc := range stored {
		cred := webauthn.Credential{
			ID: sc.ID, PublicKey: sc.PublicKey, AttestationType: sc.AttestationType,
			Authenticator: webauthn.Authenticator{AAGUID: sc.Aaguid, SignCount: uint32(sc.SignCount)}, //nolint:gosec // stored from uint32
		}
		for _, t := range sc.Transports {
			cred.Transport = append(cred.Transport, protocol.AuthenticatorTransport(t))
		}
		_ = json.Unmarshal(sc.Flags, &cred.Flags)
		w.creds = append(w.creds, cred)
	}
	return w, nil
}

// BeginPasskeyRegistration returns WebAuthn creation options for adding a passkey.
func (c *Console) BeginPasskeyRegistration(ctx context.Context, s Session) (json.RawMessage, error) {
	if err := c.canEnroll(s); err != nil {
		return nil, err
	}
	var out json.RawMessage
	err := c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		w, err := c.loadWAUser(ctx, s.UserID)
		if err != nil {
			return err
		}
		exclude := make([]protocol.CredentialDescriptor, len(w.creds))
		for i := range w.creds {
			exclude[i] = w.creds[i].Descriptor()
		}
		creation, data, err := c.wa.BeginRegistration(w,
			webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
				ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationRequired,
			}),
			webauthn.WithExclusions(exclude))
		if err != nil {
			return fmt.Errorf("begin passkey registration: %w", err)
		}
		if out, err = json.Marshal(creation); err != nil {
			return fmt.Errorf("encode passkey options: %w", err)
		}
		return c.saveCeremony(ctx, s, ceremony{Kind: "passkey_register", WebAuthn: data})
	})
	return out, err
}

// FinishPasskeyRegistration verifies the authenticator's response and stores the passkey.
// Returns recovery codes (shown once) when the user had none.
func (c *Console) FinishPasskeyRegistration(ctx context.Context, s Session, name string, response []byte) ([]string, error) {
	if err := c.canEnroll(s); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	if utf8.RuneCountInString(name) > 100 {
		return nil, fmt.Errorf("%w: passkey name must be at most 100 characters", domain.ErrInvalidInput)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return nil, domain.ErrMFAInvalid
	}
	var codes []string
	err = c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		cer, err := c.takeCeremony(ctx, s, "passkey_register")
		if err != nil || cer.WebAuthn == nil {
			return errors.Join(err, domain.ErrSessionState)
		}
		w, err := c.loadWAUser(ctx, s.UserID)
		if err != nil {
			return err
		}
		cred, err := c.wa.CreateCredential(w, *cer.WebAuthn, parsed)
		if err != nil {
			return domain.ErrMFAInvalid
		}
		flags, err := json.Marshal(cred.Flags)
		if err != nil {
			return fmt.Errorf("encode passkey flags: %w", err)
		}
		transports := make([]string, len(cred.Transport))
		for i, t := range cred.Transport {
			transports[i] = string(t)
		}
		if err := c.store.InsertCredential(ctx, sqlcgen.InsertCredentialParams{
			ID: cred.ID, UserID: s.UserID, Name: name, PublicKey: cred.PublicKey, AttestationType: cred.AttestationType,
			Aaguid: cred.Authenticator.AAGUID, SignCount: int64(cred.Authenticator.SignCount), Transports: transports,
			Flags: flags, Now: c.clock.Now(),
		}); err != nil {
			return err
		}
		if codes, err = c.ensureRecoveryCodes(ctx, s); err != nil {
			return err
		}
		if err := c.record(ctx, "user.mfa_enrolled", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, map[string]any{"method": "passkey"}); err != nil {
			return err
		}
		return c.complete(ctx, s, "passkey_enrollment")
	})
	return codes, err
}

// BeginPasskeyLogin returns WebAuthn assertion options for the session's user.
func (c *Console) BeginPasskeyLogin(ctx context.Context, s Session) (json.RawMessage, error) {
	if err := requireState(s, StateMFARequired, StateActive); err != nil {
		return nil, err
	}
	var out json.RawMessage
	err := c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		w, err := c.loadWAUser(ctx, s.UserID)
		if err != nil {
			return err
		}
		if len(w.creds) == 0 {
			return domain.ErrSessionState
		}
		assertion, data, err := c.wa.BeginLogin(w, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return fmt.Errorf("begin passkey login: %w", err)
		}
		if out, err = json.Marshal(assertion); err != nil {
			return fmt.Errorf("encode passkey options: %w", err)
		}
		return c.saveCeremony(ctx, s, ceremony{Kind: "passkey_login", WebAuthn: data})
	})
	return out, err
}

// FinishPasskeyLogin verifies a passkey assertion: completing sign-in, or re-authenticating.
func (c *Console) FinishPasskeyLogin(ctx context.Context, s Session, response []byte) error {
	if err := requireState(s, StateMFARequired, StateActive); err != nil {
		return err
	}
	if err := c.throttleMFA(ctx, s); err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return domain.ErrMFAInvalid
	}
	return c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		cer, err := c.takeCeremony(ctx, s, "passkey_login")
		if err != nil || cer.WebAuthn == nil {
			return errors.Join(err, domain.ErrSessionState)
		}
		w, err := c.loadWAUser(ctx, s.UserID)
		if err != nil {
			return err
		}
		cred, err := c.wa.ValidateLogin(w, *cer.WebAuthn, parsed)
		if err != nil {
			return c.failMFA(ctx, s, "passkey")
		}
		if cred.Authenticator.CloneWarning {
			// A sign count that went backwards suggests a cloned authenticator.
			return c.failMFA(ctx, s, "passkey_clone_warning")
		}
		flags, err := json.Marshal(cred.Flags)
		if err != nil {
			return fmt.Errorf("encode passkey flags: %w", err)
		}
		if err := c.store.UpdateCredentialUse(ctx, cred.ID, int64(cred.Authenticator.SignCount), flags, c.clock.Now()); err != nil {
			return err
		}
		return c.complete(ctx, s, "passkey")
	})
}

// Logout ends a session.
func (c *Console) Logout(ctx context.Context, s Session) error {
	return c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		if err := c.store.RevokeSession(ctx, s.ID, c.clock.Now()); err != nil {
			return err
		}
		return c.record(ctx, "user.logout", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, nil)
	})
}

// Profile is the signed-in user, their factors and their session.
type Profile struct {
	User     store.User
	Passkeys []store.Credential
	Session  Session
}

// Profile returns the session's user and factors.
func (c *Console) Profile(ctx context.Context, s Session) (Profile, error) {
	p := Profile{Session: s}
	err := c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		var err error
		if p.User, err = c.store.GetUser(ctx, s.UserID, false); err != nil {
			return err
		}
		p.Passkeys, err = c.store.Credentials(ctx, s.UserID)
		return err
	})
	return p, err
}

// DeletePasskey removes one of the user's passkeys, keeping at least one second factor.
func (c *Console) DeletePasskey(ctx context.Context, s Session, id []byte) error {
	if err := requireState(s, StateActive); err != nil {
		return err
	}
	return c.runner.Run(c.sessionCtx(ctx, s), func(ctx context.Context) error {
		p, err := c.store.GetUser(ctx, s.UserID, true)
		if err != nil {
			return err
		}
		creds, err := c.store.Credentials(ctx, s.UserID)
		if err != nil {
			return err
		}
		if p.TotpSecretEnc == nil && len(creds) <= 1 {
			return fmt.Errorf("%w: keep at least one second factor; add another first", domain.ErrConflict)
		}
		deleted, err := c.store.DeleteCredential(ctx, s.UserID, id)
		if err != nil {
			return err
		}
		if !deleted {
			return domain.ErrUserNotFound
		}
		return c.record(ctx, "user.passkey_removed", "user", ids.Format(ids.PrefixUser, s.UserID), audit.OutcomeSuccess, nil)
	})
}

// Invite is a created invite and its link token (shown once).
type Invite struct {
	ID        uuid.UUID
	Token     string
	Email     string
	Role      authz.Role
	ExpiresAt time.Time
	// Emailed is set when an email with the link was queued.
	Emailed bool
}

// InviteEmailArgs is the job that emails an invite's link. It carries no token and no address:
// the worker derives the token and reads the invite. Kind is stable across releases.
type InviteEmailArgs struct {
	TenantID uuid.UUID `json:"tenant_id"`
	InviteID uuid.UUID `json:"invite_id"`
}

// Kind implements jobs.Args.
func (InviteEmailArgs) Kind() string { return "auth_invite_email" }

// PendingInvite is an invite ready to be emailed.
type PendingInvite struct {
	Token      string
	Email      string
	Role       authz.Role
	ExpiresAt  time.Time
	TenantName string
}

func validRole(r authz.Role) bool {
	switch r {
	case authz.RoleCustomerAdmin, authz.RoleDeveloper, authz.RoleRiskManager, authz.RoleAnalyst:
		return true
	default:
		return false
	}
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		return "", fmt.Errorf("%w: email is not a valid address", domain.ErrInvalidInput)
	}
	return email, nil
}

// CreateInvite invites a person to the caller's tenant with a role. Requires team:write. The
// token is returned once; only its hash is stored.
func (c *Console) CreateInvite(ctx context.Context, rawEmail string, role authz.Role) (Invite, error) {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return Invite{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return Invite{}, err
	}
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return Invite{}, err
	}
	if !validRole(role) {
		return Invite{}, fmt.Errorf("%w: unknown role", domain.ErrInvalidInput)
	}
	taken, err := c.store.EmailTaken(ctx, email)
	if err != nil {
		return Invite{}, err
	}
	if taken {
		return Invite{}, domain.ErrEmailTaken
	}
	now := c.clock.Now()
	id := ids.NewV7()
	token := console.InviteToken(c.inviteKey, id)
	hash := console.HashToken(token)
	inv := Invite{ID: id, Token: token, Email: email, Role: role, ExpiresAt: now.Add(c.cfg.InviteTTL), Emailed: c.emails != nil}
	tctx := tenantCtx(ctx, scope.TenantID, scope.Principal)
	err = c.runner.Run(tctx, func(ctx context.Context) error {
		if err := c.store.InsertInvite(ctx, sqlcgen.InsertInviteParams{
			ID: inv.ID, Email: email, Role: string(role), TokenHash: hash, Now: now, CreatedBy: scope.Principal.ID, ExpiresAt: inv.ExpiresAt,
		}); err != nil {
			return err
		}
		if c.emails != nil {
			// Queued in the same transaction: the email goes out only if the invite exists.
			if err := c.emails.Enqueue(ctx, InviteEmailArgs{TenantID: scope.TenantID, InviteID: inv.ID}); err != nil {
				return err
			}
		}
		return c.record(ctx, "user.invited", "invite", ids.Format(ids.PrefixInvite, inv.ID), audit.OutcomeSuccess,
			map[string]any{"role": string(role), "emailed": inv.Emailed})
	})
	if err != nil {
		return Invite{}, err
	}
	return inv, nil
}

// emailActor names the invite email job in the audit log.
const emailActor = "system:email"

// PendingInviteForEmail returns the invite of a, or nil when it should not be emailed (accepted,
// revoked, expired, unknown, or its tenant is not active).
func (c *Console) PendingInviteForEmail(ctx context.Context, a InviteEmailArgs) (*PendingInvite, error) {
	token := console.InviteToken(c.inviteKey, a.InviteID)
	inv, err := c.store.InviteByToken(ctx, console.HashToken(token))
	if err != nil || inv == nil {
		return nil, err
	}
	if inv.TenantID != a.TenantID || inv.AcceptedAt != nil || inv.RevokedAt != nil ||
		!c.clock.Now().Before(inv.ExpiresAt) || inv.TenantStatus != "active" {
		return nil, nil //nolint:nilnil // nothing to send is a normal outcome
	}
	p := &PendingInvite{Token: token, Email: inv.Email, Role: authz.Role(inv.Role), ExpiresAt: inv.ExpiresAt}
	err = c.runner.Run(tenantCtx(ctx, inv.TenantID, emailPrincipal()), func(ctx context.Context) error {
		p.TenantName, err = c.store.TenantName(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// InviteEmailed records that the invite's email went out, in its tenant's audit log.
func (c *Console) InviteEmailed(ctx context.Context, a InviteEmailArgs) error {
	return c.runner.Run(tenantCtx(ctx, a.TenantID, emailPrincipal()), func(ctx context.Context) error {
		if err := c.store.MarkInviteEmailed(ctx, a.InviteID, c.clock.Now()); err != nil {
			return err
		}
		return c.record(ctx, "user.invite_emailed", "invite", ids.Format(ids.PrefixInvite, a.InviteID), audit.OutcomeSuccess, nil)
	})
}

func emailPrincipal() tenancy.Principal {
	return tenancy.Principal{Type: tenancy.PrincipalSystem, ID: emailActor}
}

// ListInvites returns the tenant's invites. Requires team:read.
func (c *Console) ListInvites(ctx context.Context) ([]store.Invite, error) {
	if err := authz.Require(ctx, authz.TeamRead); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.Invite
	err = c.runner.Run(tenantCtx(ctx, scope.TenantID, scope.Principal), func(ctx context.Context) error {
		var err error
		out, err = c.store.ListInvites(ctx)
		return err
	})
	return out, err
}

// RevokeInvite revokes a pending invite. Requires team:write.
func (c *Console) RevokeInvite(ctx context.Context, id uuid.UUID) error {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	return c.runner.Run(tenantCtx(ctx, scope.TenantID, scope.Principal), func(ctx context.Context) error {
		ok, err := c.store.RevokeInvite(ctx, id, c.clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrUserNotFound
		}
		return c.record(ctx, "user.invite_revoked", "invite", ids.Format(ids.PrefixInvite, id), audit.OutcomeSuccess, nil)
	})
}

// AcceptInvite creates the invited user and signs them in to enroll a second factor.
func (c *Console) AcceptInvite(ctx context.Context, token, name, password string, cl Client) (SignIn, error) {
	if err := c.throttle(ctx, ratelimit.Rule{Key: limitKey("invite", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 20.0 / 900, Burst: 20}}); err != nil {
		return SignIn{}, err
	}
	inv, err := c.store.InviteByToken(ctx, console.HashToken(token))
	if err != nil {
		return SignIn{}, err
	}
	now := c.clock.Now()
	if inv == nil || inv.AcceptedAt != nil || inv.RevokedAt != nil || !now.Before(inv.ExpiresAt) || inv.TenantStatus != tenantStatusActive {
		return SignIn{}, domain.ErrInviteInvalid
	}
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return SignIn{}, fmt.Errorf("%w: name must be 1-100 characters", domain.ErrInvalidInput)
	}
	if err := console.ValidatePassword(password, inv.Email); err != nil {
		return SignIn{}, fmt.Errorf("%w: %s", domain.ErrInvalidInput, strings.TrimPrefix(err.Error(), console.ErrWeakPassword.Error()+": "))
	}
	if err := c.checkBreached(ctx, password); err != nil {
		return SignIn{}, err
	}
	taken, err := c.store.EmailTaken(ctx, inv.Email)
	if err != nil {
		return SignIn{}, err
	}
	if taken {
		return SignIn{}, domain.ErrEmailTaken
	}
	hash, err := console.HashPassword(password)
	if err != nil {
		return SignIn{}, err
	}
	userID := ids.NewV7()
	actor := tenancy.Principal{Type: tenancy.PrincipalUser, ID: ids.Format(ids.PrefixUser, userID)}
	var out SignIn
	err = c.runner.Run(tenantCtx(ctx, inv.TenantID, actor), func(ctx context.Context) error {
		ok, err := c.store.AcceptInvite(ctx, inv.ID, now)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrInviteInvalid
		}
		if err := c.store.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID: userID, Email: inv.Email, Name: name, Role: inv.Role, PasswordHash: &hash, Now: now,
			CreatedBy: ids.Format(ids.PrefixInvite, inv.ID),
		}); err != nil {
			return err
		}
		if err := c.record(ctx, "user.created", "user", actor.ID, audit.OutcomeSuccess,
			map[string]any{"role": inv.Role, "invite_id": ids.Format(ids.PrefixInvite, inv.ID)}); err != nil {
			return err
		}
		out, err = c.openSession(ctx, userID, StateEnrollmentRequired, cl)
		return err
	})
	return out, err
}

// checkBreached rejects a password found in known breaches (NIST SP 800-63B §5.1.1.2). When the
// breach service cannot be reached the password is accepted and the failure logged: an outside
// outage must not stop people from joining their team.
func (c *Console) checkBreached(ctx context.Context, password string) error {
	if c.cfg.Breach == nil {
		return nil
	}
	breached, err := c.cfg.Breach.Breached(ctx, password)
	if err != nil {
		c.logger.WarnContext(ctx, "breached-password check unavailable; password accepted", slog.Any("error", err))
		return nil
	}
	if breached {
		return fmt.Errorf("%w: this password has appeared in a data breach; choose a different one", domain.ErrInvalidInput)
	}
	return nil
}

// Member is a user as the team page shows them.
type Member struct {
	User       store.User
	MFAEnabled bool
}

// ListMembers returns the tenant's users. Requires team:read.
func (c *Console) ListMembers(ctx context.Context) ([]Member, error) {
	if err := authz.Require(ctx, authz.TeamRead); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out []Member
	err = c.runner.Run(tenantCtx(ctx, scope.TenantID, scope.Principal), func(ctx context.Context) error {
		users, err := c.store.ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			creds, err := c.store.Credentials(ctx, u.ID)
			if err != nil {
				return err
			}
			out = append(out, Member{User: u, MFAEnabled: u.TotpSecretEnc != nil || len(creds) > 0})
		}
		return nil
	})
	return out, err
}

// UpdateMember changes another user's role or status. Disabling ends their sessions. The tenant
// always keeps an active administrator. Requires team:write.
func (c *Console) UpdateMember(ctx context.Context, id uuid.UUID, role *authz.Role, status *string) (Member, error) {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return Member{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return Member{}, err
	}
	if scope.Principal.ID == ids.Format(ids.PrefixUser, id) {
		return Member{}, domain.ErrSelfChange
	}
	if role != nil && !validRole(*role) {
		return Member{}, fmt.Errorf("%w: unknown role", domain.ErrInvalidInput)
	}
	if status != nil && *status != "active" && *status != "disabled" {
		return Member{}, fmt.Errorf("%w: status must be active or disabled", domain.ErrInvalidInput)
	}
	var out Member
	err = c.runner.Run(tenantCtx(ctx, scope.TenantID, scope.Principal), func(ctx context.Context) error {
		u, err := c.store.GetUser(ctx, id, true)
		if errors.Is(err, store.ErrNotFound) {
			return domain.ErrUserNotFound
		}
		if err != nil {
			return err
		}
		newRole, newStatus := u.Role, u.Status
		if role != nil {
			newRole = string(*role)
		}
		if status != nil {
			newStatus = *status
		}
		if newRole == u.Role && newStatus == u.Status {
			out = Member{User: u}
			return nil
		}
		wasAdmin := u.Role == string(authz.RoleCustomerAdmin) && u.Status == "active"
		staysAdmin := newRole == string(authz.RoleCustomerAdmin) && newStatus == "active"
		if wasAdmin && !staysAdmin {
			n, err := c.store.CountActiveAdmins(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return domain.ErrLastAdmin
			}
		}
		now := c.clock.Now()
		if err := c.store.UpdateUserAccess(ctx, id, newRole, newStatus, now); err != nil {
			return err
		}
		// Any change of access ends the user's sessions, so new permissions apply at once.
		if err := c.store.RevokeUserSessions(ctx, id, uuid.Nil, now); err != nil {
			return err
		}
		if err := c.record(ctx, "user.updated", "user", ids.Format(ids.PrefixUser, id), audit.OutcomeSuccess, map[string]any{
			"role_before": u.Role, "role_after": newRole, "status_before": u.Status, "status_after": newStatus,
		}); err != nil {
			return err
		}
		u.Role, u.Status, u.UpdatedAt = newRole, newStatus, now
		out = Member{User: u}
		return nil
	})
	return out, err
}
