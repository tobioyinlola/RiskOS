package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// ceremonyTTL bounds a WebAuthn ceremony.
const ceremonyTTL = 2 * time.Minute

// touchEvery limits how often a session's last use is written.
const touchEvery = time.Minute

// MaxAge is the longest a session lasts (the cookie's lifetime).
func (s *Service) MaxAge() time.Duration { return s.cfg.MaxAge }

// Session is a signed-in staff member.
type Session struct {
	ID            uuid.UUID
	StaffID       uuid.UUID
	Email         string
	Name          string
	Role          string
	ExpiresAt     time.Time
	IdleExpiresAt time.Time
	CSRF          string
}

// Ref is the staff member's external ID.
func (s Session) Ref() string { return staffRef(s.StaffID) }

// waStaff adapts a staff member to WebAuthn.
type waStaff struct {
	s     sqlcgen.Staff
	creds []webauthn.Credential
}

func (w waStaff) WebAuthnID() []byte                         { return w.s.ID[:] }
func (w waStaff) WebAuthnName() string                       { return w.s.Email }
func (w waStaff) WebAuthnDisplayName() string                { return w.s.Name }
func (w waStaff) WebAuthnCredentials() []webauthn.Credential { return w.creds }

func loadWAStaff(ctx context.Context, q *sqlcgen.Queries, st sqlcgen.Staff) (waStaff, error) {
	stored, err := q.StaffPasskeys(ctx, st.ID)
	if err != nil {
		return waStaff{}, store.Classify(err, "list staff passkeys")
	}
	w := waStaff{s: st}
	for _, sc := range stored {
		cred := webauthn.Credential{
			ID: sc.ID, PublicKey: sc.PublicKey, AttestationType: sc.AttestationType,
			Authenticator: webauthn.Authenticator{AAGUID: sc.Aaguid, SignCount: uint32(sc.SignCount)}, //nolint:gosec // stored from uint32
		}
		for _, t := range sc.Transports {
			cred.Transport = append(cred.Transport, protocol.AuthenticatorTransport(t))
		}
		_ = json.Unmarshal(sc.Flags, &cred.Flags) //nolint:errcheck // flags we wrote ourselves; zero value on damage
		w.creds = append(w.creds, cred)
	}
	return w, nil
}

func (s *Service) csrfFor(sessionID uuid.UUID) string {
	m := hmac.New(sha256.New, s.csrfKey)
	m.Write(sessionID[:])
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// CheckCSRF verifies a presented CSRF token in constant time.
func (s *Service) CheckCSRF(sess Session, token string) bool {
	return token != "" && hmac.Equal([]byte(s.csrfFor(sess.ID)), []byte(token))
}

func (s *Service) throttle(ctx context.Context, cl Client) error {
	if s.limiter == nil {
		return nil
	}
	key := "unknown"
	if cl.IP != nil {
		key = cl.IP.String()
	}
	if !s.limiter.Allow(ctx, "rl:staff:"+key) {
		return ErrRateLimited
	}
	return nil
}

func (s *Service) saveCeremony(ctx context.Context, q *sqlcgen.Queries, kind string, staffID *uuid.UUID, data *webauthn.SessionData) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("encode ceremony: %w", err)
	}
	id, hash, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	if err := q.PurgeCeremonies(ctx, now); err != nil {
		return "", store.Classify(err, "purge ceremonies")
	}
	if err := q.InsertCeremony(ctx, sqlcgen.InsertCeremonyParams{
		IDHash: hash, Kind: kind, StaffID: staffID, Data: raw, ExpiresAt: now.Add(ceremonyTTL),
	}); err != nil {
		return "", store.Classify(err, "save ceremony")
	}
	return id, nil
}

func (s *Service) takeCeremony(ctx context.Context, q *sqlcgen.Queries, kind, id string) (*uuid.UUID, webauthn.SessionData, error) {
	row, err := q.TakeCeremony(ctx, sqlcgen.TakeCeremonyParams{IDHash: hashToken(id), Kind: kind, Now: s.clock.Now()})
	if err := store.Classify(err, "take ceremony"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, webauthn.SessionData{}, ErrPasskey
		}
		return nil, webauthn.SessionData{}, err
	}
	var sd webauthn.SessionData
	if err := json.Unmarshal(row.Data, &sd); err != nil {
		return nil, webauthn.SessionData{}, fmt.Errorf("decode ceremony: %w", err)
	}
	return row.StaffID, sd, nil
}

// enrollee finds the staff member an enrollment token belongs to.
func (s *Service) enrollee(ctx context.Context, q *sqlcgen.Queries, token string) (sqlcgen.Staff, error) {
	st, err := q.StaffByEnrollToken(ctx, hashToken(token))
	if err := store.Classify(err, "get enrollee"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return sqlcgen.Staff{}, ErrUnauthenticated
		}
		return sqlcgen.Staff{}, err
	}
	if st.Status != "active" || st.EnrollExpiresAt == nil || !s.clock.Now().Before(*st.EnrollExpiresAt) {
		return sqlcgen.Staff{}, ErrUnauthenticated
	}
	return st, nil
}

// EnrollOptions starts registering a passkey with an enrollment token.
func (s *Service) EnrollOptions(ctx context.Context, token string, cl Client) (string, json.RawMessage, error) {
	if err := s.throttle(ctx, cl); err != nil {
		return "", nil, err
	}
	var ceremony string
	var options json.RawMessage
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		st, err := s.enrollee(ctx, q, token)
		if err != nil {
			return err
		}
		w, err := loadWAStaff(ctx, q, st)
		if err != nil {
			return err
		}
		required := true
		creation, sd, err := s.wa.BeginRegistration(w,
			webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
				ResidentKey: protocol.ResidentKeyRequirementRequired, RequireResidentKey: &required,
				UserVerification: protocol.VerificationRequired,
			}),
			webauthn.WithExclusions(webauthn.Credentials(w.creds).CredentialDescriptors()),
		)
		if err != nil {
			return fmt.Errorf("begin passkey registration: %w", err)
		}
		if options, err = json.Marshal(creation); err != nil {
			return fmt.Errorf("encode options: %w", err)
		}
		ceremony, err = s.saveCeremony(ctx, q, "enroll", &st.ID, sd)
		return err
	})
	return ceremony, options, err
}

// Enroll finishes registering the passkey, uses up the enrollment token and signs in.
func (s *Service) Enroll(ctx context.Context, token, ceremony, name string, credential []byte, cl Client) (Session, string, error) {
	if err := s.throttle(ctx, cl); err != nil {
		return Session{}, "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	if utf8.RuneCountInString(name) > 100 {
		return Session{}, "", fmt.Errorf("%w: the passkey name must be at most 100 characters", ErrInvalid)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(credential)
	if err != nil {
		return Session{}, "", ErrPasskey
	}
	var sess Session
	var sessionToken string
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		st, err := s.enrollee(ctx, q, token)
		if err != nil {
			return err
		}
		owner, sd, err := s.takeCeremony(ctx, q, "enroll", ceremony)
		if err != nil {
			return err
		}
		if owner == nil || *owner != st.ID {
			return ErrPasskey
		}
		w, err := loadWAStaff(ctx, q, st)
		if err != nil {
			return err
		}
		cred, err := s.wa.CreateCredential(w, sd, parsed)
		if err != nil {
			return ErrPasskey
		}
		flags, err := json.Marshal(cred.Flags)
		if err != nil {
			return fmt.Errorf("encode passkey flags: %w", err)
		}
		transports := make([]string, len(cred.Transport))
		for i, t := range cred.Transport {
			transports[i] = string(t)
		}
		if err := q.InsertStaffPasskey(ctx, sqlcgen.InsertStaffPasskeyParams{
			ID: cred.ID, StaffID: st.ID, Name: name, PublicKey: cred.PublicKey, AttestationType: cred.AttestationType,
			Aaguid: cred.Authenticator.AAGUID, SignCount: int64(cred.Authenticator.SignCount), Transports: transports,
			Flags: flags, Now: s.clock.Now(),
		}); err != nil {
			return store.Classify(err, "insert staff passkey")
		}
		if err := q.ClearEnrollToken(ctx, st.ID); err != nil {
			return store.Classify(err, "clear enrollment")
		}
		if err := s.record(ctx, q, staffRef(st.ID), "staff.passkey_enrolled", nil, staffRef(st.ID), "success", map[string]any{"name": name}); err != nil {
			return err
		}
		sess, sessionToken, err = s.openSession(ctx, q, st, cl)
		return err
	})
	if err != nil {
		return Session{}, "", err
	}
	return sess, sessionToken, nil
}

// LoginOptions starts a discoverable passkey sign-in.
func (s *Service) LoginOptions(ctx context.Context, cl Client) (string, json.RawMessage, error) {
	if err := s.throttle(ctx, cl); err != nil {
		return "", nil, err
	}
	assertion, sd, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, fmt.Errorf("begin passkey login: %w", err)
	}
	options, err := json.Marshal(assertion)
	if err != nil {
		return "", nil, fmt.Errorf("encode options: %w", err)
	}
	var ceremony string
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		var err error
		ceremony, err = s.saveCeremony(ctx, q, "login", nil, sd)
		return err
	})
	return ceremony, options, err
}

// Login finishes a passkey sign-in.
func (s *Service) Login(ctx context.Context, ceremony string, credential []byte, cl Client) (Session, string, error) {
	if err := s.throttle(ctx, cl); err != nil {
		return Session{}, "", err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return Session{}, "", ErrPasskey
	}
	var sess Session
	var sessionToken string
	var failed string
	err = s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		_, sd, err := s.takeCeremony(ctx, q, "login", ceremony)
		if err != nil {
			return err
		}
		var who sqlcgen.Staff
		user, cred, err := s.wa.ValidatePasskeyLogin(func(_, userHandle []byte) (webauthn.User, error) {
			id, err := uuid.FromBytes(userHandle)
			if err != nil {
				return nil, err
			}
			st, err := q.GetStaff(ctx, id)
			if err != nil {
				return nil, err
			}
			if st.Status != "active" {
				return nil, ErrUnauthenticated
			}
			who = st
			return loadWAStaff(ctx, q, st)
		}, sd, parsed)
		if err != nil || user == nil {
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
		sess, sessionToken, err = s.openSession(ctx, q, who, cl)
		return err
	})
	if err != nil {
		return Session{}, "", err
	}
	if failed != "" {
		if err := s.record(ctx, s.store.Q(), "unknown", "staff.sign_in_failed", nil, "-", "failure", map[string]any{"reason": failed}); err != nil {
			return Session{}, "", err
		}
		return Session{}, "", ErrPasskey
	}
	return sess, sessionToken, nil
}

func (s *Service) openSession(ctx context.Context, q *sqlcgen.Queries, st sqlcgen.Staff, cl Client) (Session, string, error) {
	token, hash, err := newToken()
	if err != nil {
		return Session{}, "", err
	}
	now := s.clock.Now()
	id := ids.NewV7()
	ua := cl.UserAgent
	if utf8.RuneCountInString(ua) > 256 {
		ua = string([]rune(ua)[:256])
	}
	if err := q.InsertStaffSession(ctx, sqlcgen.InsertStaffSessionParams{
		ID: id, StaffID: st.ID, TokenHash: hash, Now: now, ExpiresAt: now.Add(s.cfg.MaxAge), Ip: cl.IP, UserAgent: ua,
	}); err != nil {
		return Session{}, "", store.Classify(err, "insert staff session")
	}
	meta := map[string]any{}
	if cl.IP != nil {
		meta["ip"] = cl.IP.String()
	}
	if err := s.record(ctx, q, staffRef(st.ID), "staff.signed_in", nil, staffRef(st.ID), "success", meta); err != nil {
		return Session{}, "", err
	}
	return s.session(id, st.ID, st.Email, st.Name, st.Role, now, now.Add(s.cfg.MaxAge)), token, nil
}

func (s *Service) session(id, staffID uuid.UUID, email, name, role string, lastSeen, expires time.Time) Session {
	idle := lastSeen.Add(s.cfg.IdleTimeout)
	if idle.After(expires) {
		idle = expires
	}
	return Session{
		ID: id, StaffID: staffID, Email: email, Name: name, Role: role,
		ExpiresAt: expires, IdleExpiresAt: idle, CSRF: s.csrfFor(id),
	}
}

// ResolveSession checks a session cookie: not revoked, not past its absolute or idle expiry,
// staff member active. It records the use (at most once a minute).
func (s *Service) ResolveSession(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrUnauthenticated
	}
	q := s.store.Q()
	r, err := q.StaffSessionByToken(ctx, hashToken(token))
	if err := store.Classify(err, "get staff session"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Session{}, ErrUnauthenticated
		}
		return Session{}, err
	}
	now := s.clock.Now()
	if r.RevokedAt != nil || !now.Before(r.ExpiresAt) || !now.Before(r.LastSeenAt.Add(s.cfg.IdleTimeout)) || r.Status != "active" {
		return Session{}, ErrUnauthenticated
	}
	last := r.LastSeenAt
	if now.Sub(r.LastSeenAt) >= touchEvery {
		if err := q.TouchStaffSession(ctx, sqlcgen.TouchStaffSessionParams{ID: r.ID, Now: now}); err != nil {
			return Session{}, store.Classify(err, "touch staff session")
		}
		last = now
	}
	return s.session(r.ID, r.StaffID, r.Email, r.Name, r.Role, last, r.ExpiresAt), nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sess Session) error {
	return s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		if err := q.RevokeStaffSession(ctx, sqlcgen.RevokeStaffSessionParams{ID: sess.ID, Now: s.clock.Now()}); err != nil {
			return store.Classify(err, "revoke staff session")
		}
		return s.record(ctx, q, sess.Ref(), "staff.signed_out", nil, sess.Ref(), "success", nil)
	})
}
