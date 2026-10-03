package store

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
)

// ErrNotFound is returned when a console record does not exist.
var ErrNotFound = errors.New("not found")

// UserLogin is what signing in needs about a user, found before the tenant is known.
type UserLogin struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Status       string
	TenantStatus string
	// PasswordHash is nil for people who only sign in with Google.
	PasswordHash *string
}

// UserByEmail finds a user through the auth_user_by_email function (pre-authentication).
func (s Store) UserByEmail(ctx context.Context, email string) (*UserLogin, error) {
	var u UserLogin
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, status, tenant_status, password_hash FROM auth_user_by_email($1)`, email,
	).Scan(&u.ID, &u.TenantID, &u.Status, &u.TenantStatus, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown email is a normal outcome
	}
	if err != nil {
		return nil, fmt.Errorf("look up user: %w", err)
	}
	return &u, nil
}

// SessionLookup is a session with what authorizing a request needs.
type SessionLookup struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	UserID          uuid.UUID
	State           string
	CreatedAt       time.Time
	LastSeenAt      time.Time
	ExpiresAt       time.Time
	ReauthAt        *time.Time
	RevokedAt       *time.Time
	UserStatus      string
	UserRole        string
	TenantStatus    string
	TenantName      string
	TestEnvID       uuid.UUID
	ProductionEnvID uuid.UUID
	// ProductionEnabled is false while a self-serve tenant's onboarding is not approved.
	ProductionEnabled bool
}

// SessionByToken finds a session by its token hash (pre-authentication).
func (s Store) SessionByToken(ctx context.Context, tokenHash []byte) (*SessionLookup, error) {
	var r SessionLookup
	err := preAuth(ctx, "look up session", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`SELECT id, tenant_id, user_id, state, created_at, last_seen_at, expires_at, reauth_at, revoked_at,
			        user_status, user_role, tenant_status, tenant_name, test_env_id, production_env_id, production_enabled
			 FROM auth_session_by_token($1)`, tokenHash,
		).Scan(&r.ID, &r.TenantID, &r.UserID, &r.State, &r.CreatedAt, &r.LastSeenAt, &r.ExpiresAt, &r.ReauthAt,
			&r.RevokedAt, &r.UserStatus, &r.UserRole, &r.TenantStatus, &r.TenantName, &r.TestEnvID, &r.ProductionEnvID, &r.ProductionEnabled)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown session is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// InviteLookup is an invite found by its token (pre-authentication).
type InviteLookup struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Email        string
	Role         string
	ExpiresAt    time.Time
	AcceptedAt   *time.Time
	RevokedAt    *time.Time
	TenantStatus string
}

// InviteByToken finds an invite by its token hash.
func (s Store) InviteByToken(ctx context.Context, tokenHash []byte) (*InviteLookup, error) {
	var r InviteLookup
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, role, expires_at, accepted_at, revoked_at, tenant_status
		 FROM auth_invite_by_token($1)`, tokenHash,
	).Scan(&r.ID, &r.TenantID, &r.Email, &r.Role, &r.ExpiresAt, &r.AcceptedAt, &r.RevokedAt, &r.TenantStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown invite is a normal outcome
	}
	if err != nil {
		return nil, fmt.Errorf("look up invite: %w", err)
	}
	return &r, nil
}

// EmailTaken reports whether any tenant has a user with email.
func (s Store) EmailTaken(ctx context.Context, email string) (bool, error) {
	var taken bool
	if err := s.pool.QueryRow(ctx, `SELECT auth_email_taken($1)`, email).Scan(&taken); err != nil {
		return false, fmt.Errorf("check email: %w", err)
	}
	return taken, nil
}

// User is a console user.
type User = sqlcgen.User

// Credential is a stored passkey.
type Credential = sqlcgen.WebauthnCredential

// Invite is an invite as listed.
type Invite = sqlcgen.ListInvitesRow

// InsertUser creates an active user.
func (Store) InsertUser(ctx context.Context, p sqlcgen.InsertUserParams) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	p.TenantID = scope.TenantID
	if err := q.InsertUser(ctx, p); err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetUser returns a user of the tenant; forUpdate locks it.
func (Store) GetUser(ctx context.Context, id uuid.UUID, forUpdate bool) (User, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return User{}, err
	}
	var u User
	if forUpdate {
		u, err = q.GetUserForUpdate(ctx, sqlcgen.GetUserForUpdateParams{TenantID: scope.TenantID, ID: id})
	} else {
		u, err = q.GetUser(ctx, sqlcgen.GetUserParams{TenantID: scope.TenantID, ID: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// ListUsers returns the tenant's users.
func (Store) ListUsers(ctx context.Context) ([]User, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	us, err := q.ListUsers(ctx, scope.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return us, nil
}

// CountActiveAdmins counts the tenant's active administrators.
func (Store) CountActiveAdmins(ctx context.Context) (int64, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountActiveAdmins(ctx, scope.TenantID)
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

// UpdateUserAccess changes a user's role and status.
func (Store) UpdateUserAccess(ctx context.Context, id uuid.UUID, role, status string, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.UpdateUserAccess(ctx, sqlcgen.UpdateUserAccessParams{TenantID: scope.TenantID, ID: id, Role: role, Status: status, Now: now}); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// SetTOTP stores (or clears, with nil) a user's encrypted TOTP secret.
func (Store) SetTOTP(ctx context.Context, id uuid.UUID, enc []byte, lastStep *int64, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetUserTOTP(ctx, sqlcgen.SetUserTOTPParams{TenantID: scope.TenantID, ID: id, TotpSecretEnc: enc, TotpLastStep: lastStep, Now: now}); err != nil {
		return fmt.Errorf("set totp: %w", err)
	}
	return nil
}

// SetTOTPStep records the last accepted TOTP step.
func (Store) SetTOTPStep(ctx context.Context, id uuid.UUID, step int64) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetTOTPStep(ctx, sqlcgen.SetTOTPStepParams{TenantID: scope.TenantID, ID: id, Step: &step}); err != nil {
		return fmt.Errorf("set totp step: %w", err)
	}
	return nil
}

// SetRecoveryHashes replaces a user's unused recovery code hashes.
func (Store) SetRecoveryHashes(ctx context.Context, id uuid.UUID, hashes [][]byte, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if hashes == nil {
		hashes = [][]byte{}
	}
	if err := q.SetRecoveryHashes(ctx, sqlcgen.SetRecoveryHashesParams{TenantID: scope.TenantID, ID: id, RecoveryHashes: hashes, Now: now}); err != nil {
		return fmt.Errorf("set recovery codes: %w", err)
	}
	return nil
}

// TouchLogin records a completed sign-in.
func (Store) TouchLogin(ctx context.Context, id uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.TouchLogin(ctx, sqlcgen.TouchLoginParams{TenantID: scope.TenantID, ID: id, Now: &now}); err != nil {
		return fmt.Errorf("touch login: %w", err)
	}
	return nil
}

// NewSession describes a session to create.
type NewSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	State     string
	Now       time.Time
	ExpiresAt time.Time
	IP        *netip.Addr
	UserAgent string
}

// InsertSession creates a session.
func (Store) InsertSession(ctx context.Context, n NewSession) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	ua := n.UserAgent
	if err := q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID: n.ID, TenantID: scope.TenantID, UserID: n.UserID, TokenHash: n.TokenHash, State: n.State,
		Now: n.Now, ExpiresAt: n.ExpiresAt, Ip: n.IP, UserAgent: &ua,
	}); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// SetSessionState moves a session to state, confirming a fresh authentication.
func (Store) SetSessionState(ctx context.Context, id uuid.UUID, state string, now, expiresAt time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetSessionState(ctx, sqlcgen.SetSessionStateParams{TenantID: scope.TenantID, ID: id, State: state, Now: &now, ExpiresAt: expiresAt}); err != nil {
		return fmt.Errorf("set session state: %w", err)
	}
	return nil
}

// TouchSession records activity on a session.
func (Store) TouchSession(ctx context.Context, id uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.TouchSession(ctx, sqlcgen.TouchSessionParams{TenantID: scope.TenantID, ID: id, Now: now}); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

// SetReauth records a fresh second-factor confirmation.
func (Store) SetReauth(ctx context.Context, id uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetSessionReauth(ctx, sqlcgen.SetSessionReauthParams{TenantID: scope.TenantID, ID: id, Now: &now}); err != nil {
		return fmt.Errorf("set reauth: %w", err)
	}
	return nil
}

// SetCeremony stores (or clears, with nil) an in-flight ceremony on a session.
func (Store) SetCeremony(ctx context.Context, id uuid.UUID, ceremony []byte) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetSessionCeremony(ctx, sqlcgen.SetSessionCeremonyParams{TenantID: scope.TenantID, ID: id, Ceremony: ceremony}); err != nil {
		return fmt.Errorf("set ceremony: %w", err)
	}
	return nil
}

// Ceremony returns a session's in-flight ceremony, or nil.
func (Store) Ceremony(ctx context.Context, id uuid.UUID) ([]byte, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	c, err := q.GetSessionCeremony(ctx, sqlcgen.GetSessionCeremonyParams{TenantID: scope.TenantID, ID: id})
	if err != nil {
		return nil, fmt.Errorf("get ceremony: %w", err)
	}
	return c, nil
}

// RevokeSession ends a session.
func (Store) RevokeSession(ctx context.Context, id uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.RevokeSession(ctx, sqlcgen.RevokeSessionParams{TenantID: scope.TenantID, ID: id, Now: &now}); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// RevokeUserSessions ends every session of a user except one (uuid.Nil for none).
func (Store) RevokeUserSessions(ctx context.Context, userID, except uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.RevokeUserSessions(ctx, sqlcgen.RevokeUserSessionsParams{TenantID: scope.TenantID, UserID: userID, ExceptID: except, Now: &now}); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// InsertInvite creates an invite.
func (Store) InsertInvite(ctx context.Context, p sqlcgen.InsertInviteParams) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	p.TenantID = scope.TenantID
	if err := q.InsertInvite(ctx, p); err != nil {
		return fmt.Errorf("insert invite: %w", err)
	}
	return nil
}

// ListInvites returns the tenant's invites, newest first.
func (Store) ListInvites(ctx context.Context) ([]Invite, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListInvites(ctx, scope.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	return out, nil
}

// AcceptInvite marks a pending, unexpired invite accepted; false if it no longer was.
func (Store) AcceptInvite(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.AcceptInvite(ctx, sqlcgen.AcceptInviteParams{TenantID: scope.TenantID, ID: id, Now: &now})
	if err != nil {
		return false, fmt.Errorf("accept invite: %w", err)
	}
	return n == 1, nil
}

// MarkInviteEmailed records that the invite's email was sent.
func (Store) MarkInviteEmailed(ctx context.Context, id uuid.UUID, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.MarkInviteEmailed(ctx, sqlcgen.MarkInviteEmailedParams{TenantID: scope.TenantID, ID: id, Now: now}); err != nil {
		return fmt.Errorf("mark invite emailed: %w", err)
	}
	return nil
}

// TenantName returns the name of the scope's tenant.
func (Store) TenantName(ctx context.Context) (string, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return "", err
	}
	name, err := q.TenantName(ctx, scope.TenantID)
	if err != nil {
		return "", fmt.Errorf("tenant name: %w", err)
	}
	return name, nil
}

// RevokeInvite revokes a pending invite; false if it was not pending.
func (Store) RevokeInvite(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.RevokeInvite(ctx, sqlcgen.RevokeInviteParams{TenantID: scope.TenantID, ID: id, Now: &now})
	if err != nil {
		return false, fmt.Errorf("revoke invite: %w", err)
	}
	return n == 1, nil
}

// InsertCredential stores a passkey.
func (Store) InsertCredential(ctx context.Context, p sqlcgen.InsertCredentialParams) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	p.TenantID = scope.TenantID
	if err := q.InsertCredential(ctx, p); err != nil {
		return fmt.Errorf("insert passkey: %w", err)
	}
	return nil
}

// Credentials returns a user's passkeys.
func (Store) Credentials(ctx context.Context, userID uuid.UUID) ([]Credential, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListCredentials(ctx, sqlcgen.ListCredentialsParams{TenantID: scope.TenantID, UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("list passkeys: %w", err)
	}
	return out, nil
}

// UpdateCredentialUse records a passkey sign-in.
func (Store) UpdateCredentialUse(ctx context.Context, id []byte, signCount int64, flags []byte, now time.Time) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.UpdateCredentialUse(ctx, sqlcgen.UpdateCredentialUseParams{TenantID: scope.TenantID, ID: id, SignCount: signCount, Flags: flags, Now: &now}); err != nil {
		return fmt.Errorf("update passkey: %w", err)
	}
	return nil
}

// DeleteCredential removes one of a user's passkeys; false if it did not exist.
func (Store) DeleteCredential(ctx context.Context, userID uuid.UUID, id []byte) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteCredential(ctx, sqlcgen.DeleteCredentialParams{TenantID: scope.TenantID, UserID: userID, ID: id})
	if err != nil {
		return false, fmt.Errorf("delete passkey: %w", err)
	}
	return n == 1, nil
}
