package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/console"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/sso"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Self-serve sign-up (docs/CONSOLE_AUTH.md#sign-up): with email, business name and password, the
// email proven by a 6-digit code; or with Google. The workspace is created only when the email is
// proven, in one transaction with its first administrator and session, which must enroll a second
// factor next like every other account.

// Sign-up limits.
const (
	signupTTL         = time.Hour
	signupResendAfter = time.Minute
	// signupMaxCodes is how many codes one sign-up may send; signupMaxFailures how many wrong
	// codes it tolerates (per code) before it must start again.
	signupMaxCodes    = 5
	signupMaxFailures = 5
	googleLoginTTL    = 10 * time.Minute
)

// GoogleIssuer is Google's OpenID Connect issuer.
const GoogleIssuer = "https://accounts.google.com"

// TenantCreator creates a self-serve tenant (the tenants module, wired by the composition root):
// with locked production and a compliance onboarding, in one transaction. then runs inside that
// transaction scoped to the new tenant.
type TenantCreator interface {
	CreateSelfServe(ctx context.Context, name, actor string, now time.Time, then func(ctx context.Context, tenantID uuid.UUID) error) (uuid.UUID, error)
}

// GoogleConfig registers RiskOS with Google (an OAuth client of type "web application").
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is the console's Google callback page, registered with Google.
	RedirectURL string
	// Issuer defaults to GoogleIssuer (tests point it at a mock provider).
	Issuer string
}

// SignupConfig configures self-serve sign-up.
type SignupConfig struct {
	// Tenants creates workspaces; nil switches sign-up off entirely.
	Tenants TenantCreator
	// Google enables signing in and up with Google; nil disables it.
	Google *GoogleConfig
}

// SignupEmailArgs is the job that emails a sign-up's code (or, when the email already has an
// account, says so). Count picks the code; a newer code supersedes the job.
type SignupEmailArgs struct {
	SignupID uuid.UUID `json:"signup_id"`
	Count    int       `json:"count"`
}

// Kind implements jobs.Args.
func (SignupEmailArgs) Kind() string { return "auth_signup_email" }

// SetSignup enables self-serve sign-up.
func (c *Console) SetSignup(cfg SignupConfig) {
	if cfg.Google != nil && cfg.Google.Issuer == "" {
		cfg.Google.Issuer = GoogleIssuer
	}
	c.signup = cfg
}

// SignupOptions reports which ways to sign up are available: email needs sign-up on and email
// delivery configured; Google needs its client.
func (c *Console) SignupOptions() (email, google bool) {
	return c.signup.Tenants != nil && c.emails != nil, c.signup.Tenants != nil && c.signup.Google != nil
}

// signupCode derives code number count of sign-up id: six digits, never stored.
func (c *Console) signupCode(id uuid.UUID, count int) string {
	m := hmac.New(sha256.New, c.signupKey)
	m.Write(id[:])
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(count)) //nolint:gosec // count is 1..signupMaxCodes
	m.Write(n[:])
	sum := m.Sum(nil)
	v := binary.BigEndian.Uint32(sum[:4]) % 1_000_000
	return fmt.Sprintf("%06d", v)
}

// SignupInput is a sign-up with email and password.
type SignupInput struct {
	Name         string
	Email        string
	BusinessName string
	Password     string
}

// SignupPending is a sign-up waiting for its code, with the token for the browser's cookie.
type SignupPending struct {
	Token       string
	Email       string
	ResendAfter time.Duration
	ExpiresAt   time.Time
}

func validName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		return "", fmt.Errorf("%w: your name must be 1-100 characters", domain.ErrInvalidInput)
	}
	return name, nil
}

func validBusiness(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if n := utf8.RuneCountInString(name); n < 2 || n > 200 {
		return "", fmt.Errorf("%w: the business name must be 2-200 characters", domain.ErrInvalidInput)
	}
	return name, nil
}

// StartSignup records a sign-up with email and password and emails its first code. The outcome
// is the same whether or not the email has an account.
func (c *Console) StartSignup(ctx context.Context, in SignupInput, cl Client) (SignupPending, error) {
	if email, _ := c.SignupOptions(); !email {
		return SignupPending{}, domain.ErrSignupDisabled
	}
	emailAddr, err := normalizeEmail(in.Email)
	if err != nil {
		return SignupPending{}, err
	}
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("signup-email", emailAddr), Limit: ratelimit.Limit{PerSecond: 5.0 / 3600, Burst: 5}},
		ratelimit.Rule{Key: limitKey("signup-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 10.0 / 3600, Burst: 10}},
	); err != nil {
		return SignupPending{}, err
	}
	name, err := validName(in.Name)
	if err != nil {
		return SignupPending{}, err
	}
	business, err := validBusiness(in.BusinessName)
	if err != nil {
		return SignupPending{}, err
	}
	if err := console.ValidatePassword(in.Password, emailAddr); err != nil {
		return SignupPending{}, fmt.Errorf("%w: %s", domain.ErrInvalidInput, strings.TrimPrefix(err.Error(), console.ErrWeakPassword.Error()+": "))
	}
	if err := c.checkBreached(ctx, in.Password); err != nil {
		return SignupPending{}, err
	}
	existing, err := c.store.EmailTaken(ctx, emailAddr)
	if err != nil {
		return SignupPending{}, err
	}
	// Hashed even for an existing account, so the response takes the same time.
	hash, err := console.HashPassword(in.Password)
	if err != nil {
		return SignupPending{}, err
	}
	token, tokenHash, err := console.NewToken()
	if err != nil {
		return SignupPending{}, err
	}
	now := c.clock.Now()
	id := ids.NewV7()
	if err := c.store.InsertSignup(ctx, store.NewSignup{
		ID: id, TokenHash: tokenHash, Email: emailAddr, Name: name, BusinessName: &business, PasswordHash: &hash,
		ExistingAccount: existing, CodeSent: true, Now: now, ExpiresAt: now.Add(signupTTL),
	}); err != nil {
		return SignupPending{}, err
	}
	if err := c.emails.Enqueue(ctx, SignupEmailArgs{SignupID: id, Count: 1}); err != nil {
		return SignupPending{}, err
	}
	c.logger.InfoContext(ctx, "sign-up started", slog.String("signup_id", id.String()))
	return SignupPending{Token: token, Email: emailAddr, ResendAfter: signupResendAfter, ExpiresAt: now.Add(signupTTL)}, nil
}

// pending finds the sign-up of a browser token, still open (not expired or finished).
func (c *Console) pending(ctx context.Context, token string) (*store.Signup, error) {
	if token == "" {
		return nil, domain.ErrSignupInvalid
	}
	sg, err := c.store.SignupByToken(ctx, console.HashToken(token))
	if err != nil {
		return nil, err
	}
	if sg == nil || sg.CompletedAt != nil || !c.clock.Now().Before(sg.ExpiresAt) {
		return nil, domain.ErrSignupInvalid
	}
	return sg, nil
}

// ResendSignupCode sends a new code (the previous one stops working): once a minute, at most
// signupMaxCodes per sign-up.
func (c *Console) ResendSignupCode(ctx context.Context, token string) (SignupPending, error) {
	sg, err := c.pending(ctx, token)
	if err != nil {
		return SignupPending{}, err
	}
	if sg.PasswordHash == nil || sg.EmailVerifiedAt != nil || c.emails == nil {
		return SignupPending{}, domain.ErrSignupInvalid
	}
	now := c.clock.Now()
	if sg.CodeSentAt != nil && now.Sub(*sg.CodeSentAt) < signupResendAfter {
		return SignupPending{}, &domain.RateLimitedError{RetryAfter: signupResendAfter - now.Sub(*sg.CodeSentAt)}
	}
	if sg.CodeSentCount >= signupMaxCodes {
		return SignupPending{}, fmt.Errorf("%w: too many codes", domain.ErrSignupInvalid)
	}
	ok, err := c.store.NextSignupCode(ctx, sg.ID, sg.CodeSentCount, now)
	if err != nil {
		return SignupPending{}, err
	}
	if !ok {
		return SignupPending{}, &domain.RateLimitedError{RetryAfter: signupResendAfter}
	}
	if err := c.emails.Enqueue(ctx, SignupEmailArgs{SignupID: sg.ID, Count: sg.CodeSentCount + 1}); err != nil {
		return SignupPending{}, err
	}
	return SignupPending{Email: sg.Email, ResendAfter: signupResendAfter, ExpiresAt: sg.ExpiresAt}, nil
}

// VerifySignup checks the emailed code and, when it is right, creates the workspace and signs
// its administrator in to enroll a second factor.
func (c *Console) VerifySignup(ctx context.Context, token, code string, cl Client) (SignIn, error) {
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("signup-verify-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 30.0 / 900, Burst: 30}},
	); err != nil {
		return SignIn{}, err
	}
	sg, err := c.pending(ctx, token)
	if err != nil {
		return SignIn{}, err
	}
	if sg.PasswordHash == nil || sg.CodeSentCount == 0 || sg.FailedAttempts >= signupMaxFailures {
		return SignIn{}, domain.ErrSignupInvalid
	}
	want := c.signupCode(sg.ID, sg.CodeSentCount)
	ok := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(code)), []byte(want)) == 1
	if !ok || sg.ExistingAccount {
		n, err := c.store.SignupFailedAttempt(ctx, sg.ID)
		if err != nil {
			return SignIn{}, err
		}
		if n >= signupMaxFailures {
			return SignIn{}, fmt.Errorf("%w: too many wrong codes", domain.ErrSignupInvalid)
		}
		return SignIn{}, domain.ErrCodeInvalid
	}
	if err := c.store.VerifySignupEmail(ctx, sg.ID, c.clock.Now()); err != nil {
		return SignIn{}, err
	}
	return c.createWorkspace(ctx, sg, *sg.BusinessName, cl)
}

// CompleteSignup names the workspace of a Google sign-up (whose email Google verified) and
// creates it.
func (c *Console) CompleteSignup(ctx context.Context, token, businessName string, cl Client) (SignIn, error) {
	sg, err := c.pending(ctx, token)
	if err != nil {
		return SignIn{}, err
	}
	if sg.GoogleSubject == nil || sg.EmailVerifiedAt == nil {
		return SignIn{}, domain.ErrSignupInvalid
	}
	business, err := validBusiness(businessName)
	if err != nil {
		return SignIn{}, err
	}
	if err := c.store.SetSignupBusiness(ctx, sg.ID, business); err != nil {
		return SignIn{}, err
	}
	return c.createWorkspace(ctx, sg, business, cl)
}

// createWorkspace creates the tenant, its first administrator and a session that must enroll a
// second factor, in one transaction.
func (c *Console) createWorkspace(ctx context.Context, sg *store.Signup, business string, cl Client) (SignIn, error) {
	method := "password"
	if sg.GoogleSubject != nil {
		method = "google"
	}
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("workspace-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 5.0 / 86400, Burst: 5}},
	); err != nil {
		return SignIn{}, err
	}
	taken, err := c.store.EmailTaken(ctx, sg.Email)
	if err != nil {
		return SignIn{}, err
	}
	if taken {
		return SignIn{}, domain.ErrEmailTaken
	}
	now := c.clock.Now()
	userID := ids.NewV7()
	userRef := ids.Format(ids.PrefixUser, userID)
	var out SignIn
	_, err = c.signup.Tenants.CreateSelfServe(ctx, business, "signup:"+userRef, now, func(ctx context.Context, tenantID uuid.UUID) error {
		// Act as the new user from here on, in the new tenant.
		ctx = tenancy.WithScope(ctx, tenancy.Scope{TenantID: tenantID, Principal: tenancy.Principal{Type: tenancy.PrincipalUser, ID: userRef}})
		if err := c.store.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID: userID, Email: sg.Email, Name: sg.Name, Role: string(authz.RoleCustomerAdmin), PasswordHash: sg.PasswordHash,
			Now: now, CreatedBy: "signup",
		}); err != nil {
			return err
		}
		if sg.GoogleSubject != nil {
			if _, err := c.store.BindGoogleSubject(ctx, userID, *sg.GoogleSubject, now); err != nil {
				return err
			}
		}
		if err := c.record(ctx, "user.created", "user", userRef, audit.OutcomeSuccess,
			map[string]any{"role": string(authz.RoleCustomerAdmin), "source": "signup", "method": method}); err != nil {
			return err
		}
		var err error
		out, err = c.openSession(ctx, userID, StateEnrollmentRequired, cl)
		return err
	})
	if store.IsUniqueViolation(err) {
		// A concurrent sign-up took the email (or the Google account) first.
		return SignIn{}, domain.ErrEmailTaken
	}
	if err != nil {
		return SignIn{}, err
	}
	if _, err := c.store.CompleteSignup(ctx, sg.ID, now); err != nil {
		// The workspace exists; a sign-up left open only lapses.
		c.logger.WarnContext(ctx, "could not mark sign-up complete", slog.Any("error", err))
	}
	return out, nil
}

// PendingSignupEmail is what a sign-up's email says: the code, or that the email already has an
// account (Code empty).
type PendingSignupEmail struct {
	Email     string
	Name      string
	Code      string
	ExpiresAt time.Time
}

// SignupEmailFor returns what to email for a, or nil when nothing should be sent (finished,
// expired, superseded by a newer code, or verified).
func (c *Console) SignupEmailFor(ctx context.Context, a SignupEmailArgs) (*PendingSignupEmail, error) {
	sg, err := c.store.SignupByID(ctx, a.SignupID)
	if err != nil || sg == nil {
		return nil, err
	}
	if sg.CompletedAt != nil || sg.EmailVerifiedAt != nil || sg.CodeSentCount != a.Count || !c.clock.Now().Before(sg.ExpiresAt) {
		return nil, nil //nolint:nilnil // nothing to send is a normal outcome
	}
	out := &PendingSignupEmail{Email: sg.Email, Name: sg.Name, ExpiresAt: sg.ExpiresAt}
	if !sg.ExistingAccount {
		out.Code = c.signupCode(sg.ID, a.Count)
	}
	return out, nil
}

// GoogleOutcome is the end of a Google sign-in: a session (the account exists), or a sign-up
// waiting for its business name.
type GoogleOutcome struct {
	SignIn *SignIn
	Signup *SignupPending
	Name   string
}

func (c *Console) googleConn() (sso.Connection, error) {
	g := c.signup.Google
	if g == nil || c.signup.Tenants == nil {
		return sso.Connection{}, domain.ErrGoogleNotConfigured
	}
	return sso.Connection{Issuer: g.Issuer, ClientID: g.ClientID, ClientSecret: g.ClientSecret, RedirectURL: g.RedirectURL, Scopes: []string{"profile"}}, nil
}

// StartGoogle begins signing in or up with Google.
func (c *Console) StartGoogle(ctx context.Context, cl Client) (SSOStart, error) {
	conn, err := c.googleConn()
	if err != nil {
		return SSOStart{}, err
	}
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("google-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 60.0 / 900, Burst: 60}},
	); err != nil {
		return SSOStart{}, err
	}
	state, stateHash, err := console.NewToken()
	if err != nil {
		return SSOStart{}, err
	}
	browser, browserHash, err := console.NewToken()
	if err != nil {
		return SSOStart{}, err
	}
	authURL, err := c.sso.AuthURL(ctx, conn, state, c.derive("google-nonce", state), c.derive("google-pkce", state))
	if err != nil {
		c.logger.WarnContext(ctx, "google unavailable", slog.Any("error", err))
		return SSOStart{}, domain.ErrSSOProvider
	}
	now := c.clock.Now()
	if err := c.store.InsertGoogleLogin(ctx, stateHash, browserHash, now, now.Add(googleLoginTTL)); err != nil {
		return SSOStart{}, err
	}
	return SSOStart{RedirectURL: authURL, BrowserToken: browser}, nil
}

// FinishGoogle completes a Google sign-in at the callback. Google must vouch for the email. An
// existing account is signed in (its first Google sign-in binds Google's subject; later ones must
// present it) and still needs RiskOS's second factor; a new email starts a sign-up.
func (c *Console) FinishGoogle(ctx context.Context, state, code, browser string, cl Client) (GoogleOutcome, error) {
	conn, err := c.googleConn()
	if err != nil {
		return GoogleOutcome{}, err
	}
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("google-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 60.0 / 900, Burst: 60}},
	); err != nil {
		return GoogleOutcome{}, err
	}
	if state == "" || code == "" || browser == "" {
		return GoogleOutcome{}, domain.ErrGoogleFailed
	}
	browserHash, err := c.store.TakeGoogleLogin(ctx, console.HashToken(state), c.clock.Now())
	if err != nil {
		return GoogleOutcome{}, err
	}
	if browserHash == nil || subtle.ConstantTimeCompare(browserHash, console.HashToken(browser)) != 1 {
		return GoogleOutcome{}, domain.ErrGoogleFailed
	}
	ident, err := c.sso.Exchange(ctx, conn, code, c.derive("google-nonce", state), c.derive("google-pkce", state))
	if err != nil {
		if errors.Is(err, sso.ErrIssuer) {
			c.logger.WarnContext(ctx, "google unavailable", slog.Any("error", err))
			return GoogleOutcome{}, domain.ErrSSOProvider
		}
		c.logger.InfoContext(ctx, "google sign-in rejected", slog.Any("error", err))
		return GoogleOutcome{}, domain.ErrGoogleFailed
	}
	if !ident.EmailVerified || ident.Email == "" || ident.Subject == "" {
		return GoogleOutcome{}, domain.ErrGoogleFailed
	}

	// An account bound to this Google subject, or one with this email.
	var userID, tenantID uuid.UUID
	var userStatus, tenantStatus string
	if u, err := c.store.UserByGoogleSubject(ctx, ident.Subject); err != nil {
		return GoogleOutcome{}, err
	} else if u != nil {
		userID, tenantID, userStatus, tenantStatus = u.ID, u.TenantID, u.Status, u.TenantStatus
	} else if u, err := c.store.UserByEmail(ctx, ident.Email); err != nil {
		return GoogleOutcome{}, err
	} else if u != nil {
		userID, tenantID, userStatus, tenantStatus = u.ID, u.TenantID, u.Status, u.TenantStatus
	}
	if userID != uuid.Nil {
		in, err := c.googleSignIn(ctx, userID, tenantID, userStatus, tenantStatus, ident.Subject, cl)
		if err != nil {
			return GoogleOutcome{}, err
		}
		return GoogleOutcome{SignIn: &in}, nil
	}

	// A new person: a sign-up with the verified email, waiting for the business name.
	name := ident.Name
	if name == "" {
		name = strings.SplitN(ident.Email, "@", 2)[0]
	}
	if n := []rune(name); len(n) > 100 {
		name = string(n[:100])
	}
	token, tokenHash, err := console.NewToken()
	if err != nil {
		return GoogleOutcome{}, err
	}
	now := c.clock.Now()
	subject := ident.Subject
	if err := c.store.InsertSignup(ctx, store.NewSignup{
		ID: ids.NewV7(), TokenHash: tokenHash, Email: ident.Email, Name: name, GoogleSubject: &subject,
		EmailVerifiedAt: &now, Now: now, ExpiresAt: now.Add(signupTTL),
	}); err != nil {
		return GoogleOutcome{}, err
	}
	return GoogleOutcome{
		Signup: &SignupPending{Token: token, Email: ident.Email, ExpiresAt: now.Add(signupTTL)},
		Name:   name,
	}, nil
}

func (c *Console) googleSignIn(ctx context.Context, userID, tenantID uuid.UUID, userStatus, tenantStatus, subject string, cl Client) (SignIn, error) {
	userRef := ids.Format(ids.PrefixUser, userID)
	tctx := tenantCtx(ctx, tenantID, tenancy.Principal{Type: tenancy.PrincipalUser, ID: userRef})
	fail := func(reason string) (SignIn, error) {
		if err := c.runner.Run(tctx, func(ctx context.Context) error {
			return c.record(ctx, "user.google_failed", "user", userRef, audit.OutcomeFailure, map[string]any{"reason": reason})
		}); err != nil {
			return SignIn{}, err
		}
		return SignIn{}, domain.ErrGoogleFailed
	}
	if userStatus != "active" {
		return fail("user_disabled")
	}
	if tenantStatus != tenantStatusActive {
		return SignIn{}, domain.ErrTenantSuspended
	}
	var out SignIn
	var reason string
	err := c.runner.Run(tctx, func(ctx context.Context) error {
		ok, err := c.store.BindGoogleSubject(ctx, userID, subject, c.clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			reason = "subject_mismatch"
			return nil
		}
		user, err := c.store.GetUser(ctx, userID, false)
		if err != nil {
			return err
		}
		creds, err := c.store.Credentials(ctx, userID)
		if err != nil {
			return err
		}
		next := StateEnrollmentRequired
		if user.TotpSecretEnc != nil || len(creds) > 0 {
			next = StateMFARequired
		}
		if out, err = c.openSession(ctx, userID, next, cl); err != nil {
			return err
		}
		return c.record(ctx, "user.google_authenticated", "user", userRef, audit.OutcomeSuccess, nil)
	})
	if errors.Is(err, store.ErrSubjectTaken) {
		return fail("subject_taken")
	}
	if err != nil {
		return SignIn{}, err
	}
	if reason != "" {
		return fail(reason)
	}
	return out, nil
}
