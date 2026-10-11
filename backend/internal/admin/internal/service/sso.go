package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/sso"
)

// Single sign-on errors.
var (
	// ErrSSONotConfigured means the deployment has no identity provider for staff.
	ErrSSONotConfigured = errors.New("staff single sign-on is not configured")
	// ErrSSOFailed means the sign-in did not complete; the reason is audited, not returned.
	ErrSSOFailed = errors.New("single sign-on did not complete")
	// ErrSSOProvider means the identity provider could not be reached.
	ErrSSOProvider = errors.New("identity provider unavailable")
	// ErrSSORequired means this deployment requires single sign-on before the passkey.
	ErrSSORequired = errors.New("single sign-on is required")
)

// SSOConfig is the platform's identity provider for staff. It is deployment configuration, not
// data: staff identities are the roster, and the provider only vouches for the person.
type SSOConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is the console page that finishes the sign-in, registered at the provider.
	RedirectURL string
	// Required makes single sign-on the first step of every sign-in; the direct passkey
	// ceremony is refused, and a passkey enrolled from a link no longer opens a session.
	Required bool
	// AllowPrivate permits http providers and private addresses (local development and tests).
	AllowPrivate bool
}

// Enabled says an identity provider is configured.
func (c SSOConfig) Enabled() bool { return c.Issuer != "" }

const (
	ssoTimeout  = 5 * time.Second
	ssoStartTTL = 10 * time.Minute
)

// ssoRequest is the data kept with an SSO request in flight: which browser started it.
type ssoRequest struct {
	Browser []byte `json:"browser"`
}

// AuthMethods are the ways staff can sign in here.
type AuthMethods struct {
	SSOEnabled  bool
	SSORequired bool
}

// Methods reports the sign-in methods.
func (s *Service) Methods() AuthMethods {
	return AuthMethods{SSOEnabled: s.sso != nil, SSORequired: s.sso != nil && s.cfg.SSO.Required}
}

func (s *Service) ssoRequired() bool { return s.sso != nil && s.cfg.SSO.Required }

// derive returns the PKCE verifier or the nonce of an SSO request, recomputed at the callback
// from its state instead of being stored.
func (s *Service) derive(purpose, state string) string {
	m := hmac.New(sha256.New, s.ssoKey)
	m.Write([]byte(purpose + "\x00" + state))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Service) connection() sso.Connection {
	return sso.Connection{
		Issuer: s.cfg.SSO.Issuer, ClientID: s.cfg.SSO.ClientID, ClientSecret: s.cfg.SSO.ClientSecret,
		RedirectURL: s.cfg.SSO.RedirectURL,
	}
}

// StartSSO begins a sign-in: where to send the browser, and the token that binds the request to
// it (a cookie).
func (s *Service) StartSSO(ctx context.Context, cl Client) (redirect, browser string, err error) {
	if s.sso == nil {
		return "", "", ErrSSONotConfigured
	}
	if err := s.throttle(ctx, cl); err != nil {
		return "", "", err
	}
	browser, browserHash, err := newToken()
	if err != nil {
		return "", "", err
	}
	raw, err := json.Marshal(ssoRequest{Browser: browserHash})
	if err != nil {
		return "", "", fmt.Errorf("encode sso request: %w", err)
	}
	var state string
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		var err error
		state, err = s.saveCeremonyRaw(ctx, q, "sso_start", nil, raw, ssoStartTTL)
		return err
	})
	if err != nil {
		return "", "", err
	}
	redirect, err = s.sso.AuthURL(ctx, s.connection(), state, s.derive("nonce", state), s.derive("pkce", state))
	if err != nil {
		s.logger.WarnContext(ctx, "staff sso provider unavailable", slog.Any("error", err))
		return "", "", ErrSSOProvider
	}
	return redirect, browser, nil
}

// FinishSSO completes the provider step at the callback: the request must be in flight and
// started in this browser, and the ID token must verify. The provider's subject is bound to the
// staff member at the first sign-in (which also needs a verified email matching the account);
// later sign-ins must present the same subject. The staff member must be active and have a
// passkey. The answer is the passkey assertion to complete with SSOLogin: this step alone opens
// no session.
func (s *Service) FinishSSO(ctx context.Context, state, code, browser string, cl Client) (string, json.RawMessage, error) {
	if s.sso == nil {
		return "", nil, ErrSSONotConfigured
	}
	if err := s.throttle(ctx, cl); err != nil {
		return "", nil, err
	}
	if state == "" || code == "" || browser == "" {
		return "", nil, ErrSSOFailed
	}
	// The request is used up whatever follows: a code is never retried.
	var raw []byte
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.TakeCeremony(ctx, sqlcgen.TakeCeremonyParams{IDHash: hashToken(state), Kind: "sso_start", Now: s.clock.Now()})
		if err := store.Classify(err, "take sso request"); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrSSOFailed
			}
			return err
		}
		raw = row.Data
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	var req ssoRequest
	if err := json.Unmarshal(raw, &req); err != nil || subtle.ConstantTimeCompare(req.Browser, hashToken(browser)) != 1 {
		return "", nil, ErrSSOFailed
	}

	// Network call: outside any transaction.
	ident, err := s.sso.Exchange(ctx, s.connection(), code, s.derive("nonce", state), s.derive("pkce", state))
	if err != nil {
		if errors.Is(err, sso.ErrIssuer) {
			s.logger.WarnContext(ctx, "staff sso provider unavailable", slog.Any("error", err))
			return "", nil, ErrSSOProvider
		}
		s.logger.InfoContext(ctx, "staff sso token rejected", slog.Any("error", err))
		return "", nil, s.ssoFailed(ctx, "", "token_rejected")
	}

	var ceremony string
	var options json.RawMessage
	var reason, who string
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		st, err := s.ssoStaff(ctx, q, ident)
		if err != nil {
			return err
		}
		if st == nil {
			reason = "unknown_staff"
			return nil
		}
		who = staffRef(st.ID)
		w, err := loadWAStaff(ctx, q, *st)
		if err != nil {
			return err
		}
		switch {
		case st.Status != "active":
			reason = "staff_disabled"
			return nil
		case len(w.creds) == 0:
			// A passkey is the second factor: enrol one from the invitation link first.
			reason = "no_passkey"
			return nil
		}
		bound, err := q.StaffSSOIdentity(ctx, st.ID)
		switch {
		case err == nil:
			if bound.Issuer != s.cfg.SSO.Issuer || bound.Subject != ident.Subject {
				reason = "subject_mismatch"
				return nil
			}
		case errors.Is(store.Classify(err, "get staff sso identity"), store.ErrNotFound):
			if !ident.EmailVerified {
				reason = "email_unverified"
				return nil
			}
			if err := q.BindStaffSSOIdentity(ctx, sqlcgen.BindStaffSSOIdentityParams{
				StaffID: st.ID, Issuer: s.cfg.SSO.Issuer, Subject: ident.Subject, Now: s.clock.Now(),
			}); err != nil {
				return store.Classify(err, "bind staff sso identity")
			}
			if err := s.record(ctx, q, who, "staff.sso_bound", nil, who, "success", map[string]any{"issuer": s.cfg.SSO.Issuer}); err != nil {
				return err
			}
		default:
			return store.Classify(err, "get staff sso identity")
		}
		assertion, sd, err := s.wa.BeginLogin(w, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return fmt.Errorf("begin passkey login: %w", err)
		}
		if options, err = json.Marshal(assertion); err != nil {
			return fmt.Errorf("encode options: %w", err)
		}
		rawSD, err := json.Marshal(sd)
		if err != nil {
			return fmt.Errorf("encode ceremony: %w", err)
		}
		if ceremony, err = s.saveCeremonyRaw(ctx, q, "sso_login", &st.ID, rawSD, ceremonyTTL); err != nil {
			return err
		}
		return s.record(ctx, q, who, "staff.sso_authenticated", nil, who, "success", map[string]any{"issuer": s.cfg.SSO.Issuer})
	})
	if errors.Is(err, store.ErrDuplicate) {
		// The subject is already bound to another staff member.
		return "", nil, s.ssoFailed(ctx, who, "subject_taken")
	}
	if err != nil {
		return "", nil, err
	}
	if reason != "" {
		return "", nil, s.ssoFailed(ctx, who, reason)
	}
	return ceremony, options, nil
}

// ssoStaff finds the staff member the identity is: by bound subject, else by verified-or-not
// email (the caller checks verification before binding). Nil means none.
func (s *Service) ssoStaff(ctx context.Context, q *sqlcgen.Queries, ident sso.Identity) (*sqlcgen.Staff, error) {
	st, err := q.StaffBySSOSubject(ctx, sqlcgen.StaffBySSOSubjectParams{Issuer: s.cfg.SSO.Issuer, Subject: ident.Subject})
	if err := store.Classify(err, "get staff by sso subject"); err == nil {
		return &st, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if ident.Email == "" {
		return nil, nil //nolint:nilnil // nil means no such staff member
	}
	st, err = q.StaffByEmail(ctx, ident.Email)
	if err := store.Classify(err, "get staff by email"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil //nolint:nilnil // nil means no such staff member
		}
		return nil, err
	}
	return &st, nil
}

// ssoFailed audits a failed provider step and returns ErrSSOFailed.
func (s *Service) ssoFailed(ctx context.Context, staff, reason string) error {
	actor, resource := "unknown", "-"
	if staff != "" {
		actor, resource = staff, staff
	}
	if err := s.recordAlone(ctx, actor, "staff.sso_failed", nil, resource, "failure", map[string]any{"reason": reason}); err != nil {
		return err
	}
	return ErrSSOFailed
}

// SSOLogin finishes a sign-in after the provider step: the passkey assertion for the staff
// member the provider step identified.
func (s *Service) SSOLogin(ctx context.Context, ceremony string, credential []byte, cl Client) (Session, string, error) {
	if s.sso == nil {
		return Session{}, "", ErrSSONotConfigured
	}
	if err := s.throttle(ctx, cl); err != nil {
		return Session{}, "", err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return Session{}, "", ErrPasskey
	}
	var sess Session
	var sessionToken, failed string
	var who uuid.UUID
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.TakeCeremony(ctx, sqlcgen.TakeCeremonyParams{IDHash: hashToken(ceremony), Kind: "sso_login", Now: s.clock.Now()})
		if err := store.Classify(err, "take ceremony"); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrPasskey
			}
			return err
		}
		if row.StaffID == nil {
			return ErrPasskey
		}
		who = *row.StaffID
		var sd webauthn.SessionData
		if err := json.Unmarshal(row.Data, &sd); err != nil {
			return fmt.Errorf("decode ceremony: %w", err)
		}
		st, err := q.GetStaff(ctx, who)
		if err := store.Classify(err, "get staff"); err != nil {
			return err
		}
		if st.Status != "active" {
			failed = "staff_disabled"
			return nil
		}
		w, err := loadWAStaff(ctx, q, st)
		if err != nil {
			return err
		}
		cred, err := s.wa.ValidateLogin(w, sd, parsed)
		if err != nil || cred == nil {
			// Commit regardless, so the ceremony stays used up; the failure is recorded below.
			failed = "passkey_invalid"
			return nil //nolint:nilerr // see above
		}
		if cred.Authenticator.CloneWarning {
			failed = "passkey_clone_warning"
			return nil
		}
		flags, err := json.Marshal(cred.Flags)
		if err != nil {
			return fmt.Errorf("encode passkey flags: %w", err)
		}
		if err := q.UpdateStaffPasskeyUse(ctx, sqlcgen.UpdateStaffPasskeyUseParams{
			ID: cred.ID, SignCount: int64(cred.Authenticator.SignCount), Flags: flags, Now: s.clock.Now(),
		}); err != nil {
			return store.Classify(err, "update staff passkey")
		}
		sess, sessionToken, err = s.openSession(ctx, q, st, cl, "sso+passkey")
		return err
	})
	if err != nil {
		return Session{}, "", err
	}
	if failed != "" {
		if err := s.recordAlone(ctx, staffRef(who), "staff.sign_in_failed", nil, staffRef(who), "failure", map[string]any{"reason": failed, "method": "sso+passkey"}); err != nil {
			return Session{}, "", err
		}
		return Session{}, "", ErrPasskey
	}
	return sess, sessionToken, nil
}

// UnbindSSO forgets a staff member's provider subject, so the next SSO sign-in binds afresh
// (their account at the provider changed). Operator CLI.
func (s *Service) UnbindSSO(ctx context.Context, email, operator string) error {
	return s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		st, err := q.StaffByEmail(ctx, email)
		if err := store.Classify(err, "get staff"); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}
		n, err := q.UnbindStaffSSOIdentity(ctx, st.ID)
		if err != nil {
			return store.Classify(err, "unbind staff sso identity")
		}
		if n == 0 {
			return fmt.Errorf("%w: no single sign-on identity is bound", ErrConflict)
		}
		// Sessions opened through the old identity end with it.
		if err := q.RevokeStaffSessions(ctx, sqlcgen.RevokeStaffSessionsParams{StaffID: st.ID, Now: s.clock.Now()}); err != nil {
			return store.Classify(err, "revoke staff sessions")
		}
		return s.record(ctx, q, "riskosctl:"+operator, "staff.sso_unbound", nil, staffRef(st.ID), "success", nil)
	})
}
