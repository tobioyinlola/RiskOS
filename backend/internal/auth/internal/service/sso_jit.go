package service

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/sso"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// jitWorkspace finds the workspace that lets the person with email in just in time: it has
// verified the email's domain, single sign-on is on and so is account creation, and it is active.
// Nil when there is none, which callers treat like an unknown email.
func (c *Console) jitWorkspace(ctx context.Context, email string) (*store.SSODomainLookup, error) {
	at := strings.LastIndex(email, "@")
	if at < 1 || at == len(email)-1 || len(email) > 254 {
		return nil, nil //nolint:nilnil // not an email address: nothing to find
	}
	d, err := c.store.SSOForDomain(ctx, email[at+1:])
	if err != nil || d == nil {
		return nil, err
	}
	if !d.Enabled || !d.JITEnabled || d.TenantStatus != tenantStatusActive {
		return nil, nil //nolint:nilnil // the workspace does not let people in this way
	}
	return d, nil
}

// finishJIT completes a sign-in for someone who had no account when it began. The provider must
// vouch for the email asked for, the domain must still be verified and account creation still on;
// then the account is created with the workspace's fixed role, no password, the provider subject
// bound, and the session needs RiskOS's second factor like any other.
func (c *Console) finishJIT(ctx context.Context, login *store.SSOLogin, state, code string, cl Client) (SignIn, error) {
	userID := ids.NewV7()
	actor := tenancy.Principal{Type: tenancy.PrincipalUser, ID: ids.Format(ids.PrefixUser, userID)}
	sctx := tenantCtx(ctx, login.TenantID, ssoPrincipal())
	fail := func(reason string, cause error) (SignIn, error) {
		if err := c.runner.Run(sctx, func(ctx context.Context) error {
			return c.record(ctx, "user.sso_failed", "sso_login", login.Email, audit.OutcomeFailure,
				map[string]any{"reason": reason, "just_in_time": true})
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
	if err := c.runner.Run(sctx, func(ctx context.Context) error {
		var err error
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
	taken, err := c.store.EmailTaken(ctx, login.Email)
	if err != nil {
		return SignIn{}, err
	}
	if taken {
		return fail("email_taken", nil)
	}

	now := c.clock.Now()
	var out SignIn
	var reason string
	err = c.runner.Run(tenantCtx(ctx, login.TenantID, actor), func(ctx context.Context) error {
		cfg, err := c.store.SSOConnection(ctx)
		if err != nil {
			return err
		}
		switch {
		case cfg == nil || !cfg.Enabled || !cfg.JitEnabled:
			reason = "jit_disabled"
			return nil
		case !slices.Contains(jitRoles, cfg.JitRole):
			reason = "role_invalid"
			return nil
		case !ident.EmailVerified || !strings.EqualFold(strings.TrimSpace(ident.Email), login.Email):
			// The provider must vouch for the very address the person asked to sign in with.
			reason = "email_mismatch"
			return nil
		}
		claim, err := c.store.SSODomain(ctx, login.Email[strings.LastIndex(login.Email, "@")+1:])
		if err != nil {
			return err
		}
		if claim == nil || claim.VerifiedAt == nil {
			reason = "domain_unverified"
			return nil
		}
		if err := c.store.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID: userID, Email: login.Email, Name: jitName(ident.Name, login.Email), Role: cfg.JitRole,
			Now: now, CreatedBy: "sso:jit",
		}); err != nil {
			return err
		}
		// A unique violation aborts the transaction: it is returned, and handled after rollback.
		if ok, err := c.store.BindSSOSubject(ctx, userID, ident.Subject); err != nil {
			return err
		} else if !ok {
			reason = "subject_mismatch"
			return nil
		}
		if out, err = c.openSession(ctx, userID, StateEnrollmentRequired, cl); err != nil {
			return err
		}
		if err := c.record(ctx, "user.created", "user", actor.ID, audit.OutcomeSuccess,
			map[string]any{"role": cfg.JitRole, "via": "sso_jit", "issuer": conn.Issuer}); err != nil {
			return err
		}
		return c.record(ctx, "user.sso_authenticated", "user", actor.ID, audit.OutcomeSuccess,
			map[string]any{"issuer": conn.Issuer, "just_in_time": true})
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

// jitName is the new account's name: the provider's, or the part of the email before the @.
func jitName(name, email string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	if utf8.RuneCountInString(name) > 100 {
		name = string([]rune(name)[:100])
	}
	if name == "" {
		return "RiskOS user"
	}
	return name
}
