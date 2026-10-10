package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/console"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ratelimit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/sso"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// ssoLoginTTL bounds an authorization request: the person signs in at their provider within it.
const ssoLoginTTL = 10 * time.Minute

// SSOConnection is a tenant's identity provider registration, without its secret.
type SSOConnection struct {
	Issuer    string
	ClientID  string
	Enabled   bool
	UpdatedAt time.Time
	UpdatedBy string
}

// SSOInput changes the connection. A nil ClientSecret keeps the stored one.
type SSOInput struct {
	Issuer       string
	ClientID     string
	ClientSecret *string
	Enabled      bool
}

// SSOStart is where to send the browser, and the token binding the request to it (a cookie).
type SSOStart struct {
	RedirectURL  string
	BrowserToken string
}

func secretAAD(tenantID [16]byte) []byte {
	return append([]byte("riskos-sso-client-secret\x00"), tenantID[:]...)
}

// derive returns a value derived from an SSO state token: the PKCE verifier or the nonce. They
// are recomputed at the callback instead of being stored.
func (c *Console) derive(purpose, state string) string {
	m := hmac.New(sha256.New, c.ssoKey)
	m.Write([]byte(purpose + "\x00" + state))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// SSORedirectURL is the callback address to register at the provider.
func (c *Console) SSORedirectURL() string { return c.cfg.SSORedirectURL }

// GetSSO returns the tenant's connection, or nil. Requires team:read.
func (c *Console) GetSSO(ctx context.Context) (*SSOConnection, error) {
	if err := authz.Require(ctx, authz.TeamRead); err != nil {
		return nil, err
	}
	var out *SSOConnection
	err := c.runner.Run(ctx, func(ctx context.Context) error {
		conn, err := c.store.SSOConnection(ctx)
		if err != nil || conn == nil {
			return err
		}
		out = &SSOConnection{Issuer: conn.Issuer, ClientID: conn.ClientID, Enabled: conn.Enabled, UpdatedAt: conn.UpdatedAt, UpdatedBy: conn.UpdatedBy}
		return nil
	})
	return out, err
}

// PutSSO creates or changes the tenant's connection, after checking the issuer publishes an
// OpenID Connect discovery document. Changing the issuer forgets every user's bound subject.
// Requires team:write; audited.
func (c *Console) PutSSO(ctx context.Context, in SSOInput) (SSOConnection, error) {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return SSOConnection{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return SSOConnection{}, err
	}
	in.Issuer, in.ClientID = strings.TrimSpace(in.Issuer), strings.TrimSpace(in.ClientID)
	if err := c.sso.CheckIssuer(in.Issuer); err != nil || len(in.Issuer) > 512 {
		return SSOConnection{}, fmt.Errorf("%w: the issuer must be an https URL", domain.ErrInvalidInput)
	}
	if in.ClientID == "" || len(in.ClientID) > 256 {
		return SSOConnection{}, fmt.Errorf("%w: the client ID is required (at most 256 characters)", domain.ErrInvalidInput)
	}
	if in.ClientSecret != nil && (*in.ClientSecret == "" || len(*in.ClientSecret) > 1024) {
		return SSOConnection{}, fmt.Errorf("%w: the client secret must be 1 to 1024 characters", domain.ErrInvalidInput)
	}
	// Network call: outside the transaction.
	if err := c.sso.Discover(ctx, in.Issuer); err != nil {
		c.logger.InfoContext(ctx, "sso discovery failed", slog.Any("error", err))
		return SSOConnection{}, fmt.Errorf("%w: RiskOS could not read the provider's OpenID configuration at this issuer", domain.ErrInvalidInput)
	}
	now := c.clock.Now()
	out := SSOConnection{Issuer: in.Issuer, ClientID: in.ClientID, Enabled: in.Enabled, UpdatedAt: now, UpdatedBy: scope.Principal.ID}
	err = c.runner.Run(ctx, func(ctx context.Context) error {
		old, err := c.store.SSOConnection(ctx)
		if err != nil {
			return err
		}
		var sealed []byte
		switch {
		case in.ClientSecret != nil:
			if sealed, err = c.sealer.Seal([]byte(*in.ClientSecret), secretAAD(scope.TenantID)); err != nil {
				return err
			}
		case old != nil:
			sealed = old.ClientSecretEnc
		default:
			return fmt.Errorf("%w: the client secret is required", domain.ErrInvalidInput)
		}
		if old != nil && old.Issuer != in.Issuer {
			if err := c.store.ClearSSOSubjects(ctx); err != nil {
				return err
			}
		}
		if err := c.store.PutSSOConnection(ctx, sqlcgen.UpsertSSOConnectionParams{
			Issuer: in.Issuer, ClientID: in.ClientID, ClientSecretEnc: sealed, Enabled: in.Enabled, Now: now, UpdatedBy: scope.Principal.ID,
		}); err != nil {
			return err
		}
		return c.record(ctx, "sso.configured", "sso_connection", ids.Format(ids.PrefixTenant, scope.TenantID), audit.OutcomeSuccess,
			map[string]any{"issuer": in.Issuer, "client_id": in.ClientID, "enabled": in.Enabled, "secret_changed": in.ClientSecret != nil})
	})
	if err != nil {
		return SSOConnection{}, err
	}
	return out, nil
}

// DeleteSSO removes the tenant's connection and every user's bound subject. Requires
// team:write; audited.
func (c *Console) DeleteSSO(ctx context.Context) error {
	if err := authz.Require(ctx, authz.TeamWrite); err != nil {
		return err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	return c.runner.Run(ctx, func(ctx context.Context) error {
		ok, err := c.store.DeleteSSOConnection(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrSSONotFound
		}
		if err := c.store.ClearSSOSubjects(ctx); err != nil {
			return err
		}
		return c.record(ctx, "sso.removed", "sso_connection", ids.Format(ids.PrefixTenant, scope.TenantID), audit.OutcomeSuccess, nil)
	})
}

// connection loads and decrypts the connection of the tenant in ctx.
func (c *Console) connection(ctx context.Context, tenantID [16]byte) (*sso.Connection, bool, error) {
	conn, err := c.store.SSOConnection(ctx)
	if err != nil || conn == nil {
		return nil, false, err
	}
	secret, err := c.sealer.Open(conn.ClientSecretEnc, secretAAD(tenantID))
	if err != nil {
		return nil, false, fmt.Errorf("open sso client secret: %w", err)
	}
	return &sso.Connection{
		Issuer: conn.Issuer, ClientID: conn.ClientID, ClientSecret: string(secret), RedirectURL: c.cfg.SSORedirectURL,
	}, conn.Enabled, nil
}

// StartSSO begins signing in the person with email through their tenant's provider. Unknown
// emails, disabled users and tenants without SSO all get domain.ErrSSONotConfigured.
func (c *Console) StartSSO(ctx context.Context, email string, cl Client) (SSOStart, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("sso-email", email), Limit: ratelimit.Limit{PerSecond: 10.0 / 900, Burst: 10}},
		ratelimit.Rule{Key: limitKey("sso-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 60.0 / 900, Burst: 60}},
	); err != nil {
		return SSOStart{}, err
	}
	u, err := c.store.SSOForEmail(ctx, email)
	if err != nil {
		return SSOStart{}, err
	}
	if u == nil || !u.Enabled || u.UserStatus != "active" || u.TenantStatus != tenantStatusActive {
		return SSOStart{}, domain.ErrSSONotConfigured
	}
	var conn *sso.Connection
	err = c.runner.Run(tenantCtx(ctx, u.TenantID, ssoPrincipal()), func(ctx context.Context) error {
		var err error
		conn, _, err = c.connection(ctx, u.TenantID)
		return err
	})
	if err != nil {
		return SSOStart{}, err
	}
	if conn == nil {
		return SSOStart{}, domain.ErrSSONotConfigured
	}
	state, stateHash, err := console.NewToken()
	if err != nil {
		return SSOStart{}, err
	}
	browser, browserHash, err := console.NewToken()
	if err != nil {
		return SSOStart{}, err
	}
	authURL, err := c.sso.AuthURL(ctx, *conn, state, c.derive("nonce", state), c.derive("pkce", state))
	if err != nil {
		c.logger.WarnContext(ctx, "sso provider unavailable", slog.Any("error", err))
		return SSOStart{}, domain.ErrSSOProvider
	}
	now := c.clock.Now()
	if err := c.store.InsertSSOLogin(ctx, store.SSOLogin{
		StateHash: stateHash, BrowserHash: browserHash, TenantID: u.TenantID, UserID: u.UserID,
	}, now, now.Add(ssoLoginTTL)); err != nil {
		return SSOStart{}, err
	}
	return SSOStart{RedirectURL: authURL, BrowserToken: browser}, nil
}

// FinishSSO completes sign-in at the callback: the request must be in flight, started in this
// browser, and the provider's ID token must verify. The provider's subject is bound to the user
// at the first SSO sign-in (which also needs a verified email matching the account); later
// sign-ins must present the same subject. The session still needs RiskOS's second factor.
func (c *Console) FinishSSO(ctx context.Context, state, code, browser string, cl Client) (SignIn, error) {
	if err := c.throttle(ctx,
		ratelimit.Rule{Key: limitKey("sso-ip", cl.ipString()), Limit: ratelimit.Limit{PerSecond: 60.0 / 900, Burst: 60}},
	); err != nil {
		return SignIn{}, err
	}
	if state == "" || code == "" || browser == "" {
		return SignIn{}, domain.ErrSSOFailed
	}
	login, err := c.store.TakeSSOLogin(ctx, console.HashToken(state), c.clock.Now())
	if err != nil {
		return SignIn{}, err
	}
	if login == nil || subtle.ConstantTimeCompare(login.BrowserHash, console.HashToken(browser)) != 1 {
		return SignIn{}, domain.ErrSSOFailed
	}
	userRef := ids.Format(ids.PrefixUser, login.UserID)
	tctx := tenantCtx(ctx, login.TenantID, tenancy.Principal{Type: tenancy.PrincipalUser, ID: userRef})
	fail := func(reason string, cause error) (SignIn, error) {
		if err := c.runner.Run(tctx, func(ctx context.Context) error {
			return c.record(ctx, "user.sso_failed", "user", userRef, audit.OutcomeFailure, map[string]any{"reason": reason})
		}); err != nil {
			return SignIn{}, err
		}
		if cause != nil {
			c.logger.InfoContext(ctx, "sso sign-in failed", slog.String("reason", reason), slog.Any("error", cause))
		}
		return SignIn{}, domain.ErrSSOFailed
	}

	var conn *sso.Connection
	var enabled bool
	if err := c.runner.Run(tctx, func(ctx context.Context) error {
		conn, enabled, err = c.connection(ctx, login.TenantID)
		return err
	}); err != nil {
		return SignIn{}, err
	}
	if conn == nil || !enabled {
		return fail("sso_disabled", nil)
	}
	// Network call: outside any transaction.
	ident, err := c.sso.Exchange(ctx, *conn, code, c.derive("nonce", state), c.derive("pkce", state))
	if err != nil {
		if errors.Is(err, sso.ErrIssuer) {
			c.logger.WarnContext(ctx, "sso provider unavailable", slog.Any("error", err))
			return SignIn{}, domain.ErrSSOProvider
		}
		return fail("token_rejected", err)
	}

	var out SignIn
	var reason string
	err = c.runner.Run(tctx, func(ctx context.Context) error {
		user, err := c.store.GetUser(ctx, login.UserID, true)
		if err != nil {
			return err
		}
		switch {
		case user.Status != "active":
			reason = "user_disabled"
			return nil
		case user.SsoSubject == nil && (!ident.EmailVerified || ident.Email != user.Email):
			// First SSO sign-in: the provider must vouch for the account's email.
			reason = "email_mismatch"
			return nil
		}
		// A unique violation aborts the transaction: it is returned, and handled after rollback.
		ok, err := c.store.BindSSOSubject(ctx, user.ID, ident.Subject)
		if err != nil {
			return err
		}
		if !ok {
			reason = "subject_mismatch"
			return nil
		}
		creds, err := c.store.Credentials(ctx, user.ID)
		if err != nil {
			return err
		}
		next := StateEnrollmentRequired
		if user.TotpSecretEnc != nil || len(creds) > 0 {
			next = StateMFARequired
		}
		if out, err = c.openSession(ctx, user.ID, next, cl); err != nil {
			return err
		}
		return c.record(ctx, "user.sso_authenticated", "user", userRef, audit.OutcomeSuccess, map[string]any{"issuer": conn.Issuer})
	})
	if errors.Is(err, store.ErrSubjectTaken) {
		return fail("subject_taken", nil)
	}
	if err != nil {
		return SignIn{}, err
	}
	if reason != "" {
		return fail(reason, nil)
	}
	return out, nil
}

func ssoPrincipal() tenancy.Principal {
	return tenancy.Principal{Type: tenancy.PrincipalSystem, ID: "system:sso"}
}
