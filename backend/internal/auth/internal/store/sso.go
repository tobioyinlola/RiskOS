package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store/sqlcgen"
)

// ErrSubjectTaken is returned when the provider's subject is bound to another user.
var ErrSubjectTaken = errors.New("sso subject bound to another user")

// SSOConnection is a tenant's identity provider registration.
type SSOConnection = sqlcgen.SsoConnection

// SSOLookup is what starting SSO for an email needs (pre-authentication).
type SSOLookup struct {
	UserID       uuid.UUID
	TenantID     uuid.UUID
	UserStatus   string
	TenantStatus string
	Enabled      bool
}

// SSOForEmail finds the user with email and whether their tenant uses SSO.
func (s Store) SSOForEmail(ctx context.Context, email string) (*SSOLookup, error) {
	var r SSOLookup
	err := preAuth(ctx, "look up sso", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`SELECT user_id, tenant_id, user_status, tenant_status, sso_enabled FROM auth_sso_for_email($1)`, email,
		).Scan(&r.UserID, &r.TenantID, &r.UserStatus, &r.TenantStatus, &r.Enabled)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown email is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ErrDomainTaken is returned when another workspace has verified the domain.
var ErrDomainTaken = errors.New("sso domain verified by another workspace")

// SSODomain is a domain a workspace claims.
type SSODomain = sqlcgen.SsoDomain

// SSODomainLookup is what signing in a person with no account needs (pre-authentication): the
// workspace that has verified their email's domain, and whether it lets people in just in time.
type SSODomainLookup struct {
	TenantID     uuid.UUID
	TenantStatus string
	Enabled      bool
	JITEnabled   bool
	JITRole      string
}

// SSOForDomain finds the workspace that has verified domain; nil if none has.
func (s Store) SSOForDomain(ctx context.Context, domain string) (*SSODomainLookup, error) {
	var r SSODomainLookup
	err := preAuth(ctx, "look up sso domain", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`SELECT tenant_id, tenant_status, sso_enabled, jit_enabled, jit_role FROM auth_sso_for_domain($1)`, domain,
		).Scan(&r.TenantID, &r.TenantStatus, &r.Enabled, &r.JITEnabled, &r.JITRole)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // an unclaimed domain is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SSODomains lists the scope's claimed domains.
func (Store) SSODomains(ctx context.Context) ([]SSODomain, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListSSODomains(ctx, scope.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list sso domains: %w", err)
	}
	return out, nil
}

// SSODomain returns one claimed domain, or nil.
func (Store) SSODomain(ctx context.Context, domain string) (*SSODomain, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	d, err := q.GetSSODomain(ctx, sqlcgen.GetSSODomainParams{TenantID: scope.TenantID, Domain: domain})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // not claimed is a normal outcome
	}
	if err != nil {
		return nil, fmt.Errorf("get sso domain: %w", err)
	}
	return &d, nil
}

// InsertSSODomain claims domain for the scope; false if it is already claimed.
func (Store) InsertSSODomain(ctx context.Context, domain string, now time.Time, by string) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	err = q.InsertSSODomain(ctx, sqlcgen.InsertSSODomainParams{TenantID: scope.TenantID, Domain: domain, Now: now, CreatedBy: by})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim sso domain: %w", err)
	}
	return true, nil
}

// VerifySSODomain marks the scope's claim verified; false if there is no unverified claim, and
// ErrDomainTaken when another workspace holds the domain verified.
func (Store) VerifySSODomain(ctx context.Context, domain string, now time.Time) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.VerifySSODomain(ctx, sqlcgen.VerifySSODomainParams{TenantID: scope.TenantID, Domain: domain, Now: &now})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, ErrDomainTaken
	}
	if err != nil {
		return false, fmt.Errorf("verify sso domain: %w", err)
	}
	return n > 0, nil
}

// DeleteSSODomain removes the scope's claim; false if there was none.
func (Store) DeleteSSODomain(ctx context.Context, domain string) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteSSODomain(ctx, sqlcgen.DeleteSSODomainParams{TenantID: scope.TenantID, Domain: domain})
	if err != nil {
		return false, fmt.Errorf("delete sso domain: %w", err)
	}
	return n > 0, nil
}

// DeleteSSODomains removes every claim of the scope's tenant.
func (Store) DeleteSSODomains(ctx context.Context) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.DeleteSSODomains(ctx, scope.TenantID); err != nil {
		return fmt.Errorf("delete sso domains: %w", err)
	}
	return nil
}

// VerifiedSSODomains counts the scope's verified claims.
func (Store) VerifiedSSODomains(ctx context.Context) (int64, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountVerifiedSSODomains(ctx, scope.TenantID)
	if err != nil {
		return 0, fmt.Errorf("count sso domains: %w", err)
	}
	return n, nil
}

// SSOConnection returns the scope's connection, or nil if there is none.
func (Store) SSOConnection(ctx context.Context) (*SSOConnection, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	c, err := q.GetSSOConnection(ctx, scope.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no connection is a normal outcome
	}
	if err != nil {
		return nil, fmt.Errorf("get sso connection: %w", err)
	}
	return &c, nil
}

// PutSSOConnection creates or replaces the scope's connection.
func (Store) PutSSOConnection(ctx context.Context, p sqlcgen.UpsertSSOConnectionParams) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	p.TenantID = scope.TenantID
	if err := q.UpsertSSOConnection(ctx, p); err != nil {
		return fmt.Errorf("put sso connection: %w", err)
	}
	return nil
}

// DeleteSSOConnection removes the scope's connection; false if there was none.
func (Store) DeleteSSOConnection(ctx context.Context) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.DeleteSSOConnection(ctx, scope.TenantID)
	if err != nil {
		return false, fmt.Errorf("delete sso connection: %w", err)
	}
	return n > 0, nil
}

// SetSSOOptional stops requiring single sign-on for the scope's tenant; false if it was not
// required.
func (Store) SetSSOOptional(ctx context.Context, now time.Time, updatedBy string) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.SetSSORequired(ctx, sqlcgen.SetSSORequiredParams{TenantID: scope.TenantID, Now: now, UpdatedBy: updatedBy})
	if err != nil {
		return false, fmt.Errorf("lift sso requirement: %w", err)
	}
	return n > 0, nil
}

// ClearSSOSubjects forgets every user's provider subject in the scope's tenant.
func (Store) ClearSSOSubjects(ctx context.Context) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.ClearSSOSubjects(ctx, scope.TenantID); err != nil {
		return fmt.Errorf("clear sso subjects: %w", err)
	}
	return nil
}

// SSOLogin is an authorization request in flight.
type SSOLogin struct {
	StateHash   []byte
	BrowserHash []byte
	TenantID    uuid.UUID
	// UserID is the existing account being signed in, or uuid.Nil for someone with no account
	// yet, who is identified by Email until the provider vouches for it (just-in-time accounts).
	UserID uuid.UUID
	Email  string
}

// InsertSSOLogin records an authorization request in flight, purging expired ones.
func (s Store) InsertSSOLogin(ctx context.Context, l SSOLogin, now, expiresAt time.Time) error {
	var user *uuid.UUID
	var email *string
	if l.UserID != uuid.Nil {
		user = &l.UserID
	} else {
		email = &l.Email
	}
	return preAuth(ctx, "record sso login", func(ctx context.Context) error {
		var one int
		return s.pool.QueryRow(ctx,
			`WITH purged AS (DELETE FROM sso_logins WHERE expires_at < $5)
			 INSERT INTO sso_logins (state_hash, browser_hash, tenant_id, user_id, email, created_at, expires_at)
			 VALUES ($1, $2, $3, $4, $7, $5, $6) RETURNING 1`,
			l.StateHash, l.BrowserHash, l.TenantID, user, now, expiresAt, email,
		).Scan(&one)
	})
}

// TakeSSOLogin consumes the authorization request with stateHash; nil if unknown or expired.
func (s Store) TakeSSOLogin(ctx context.Context, stateHash []byte, now time.Time) (*SSOLogin, error) {
	l := SSOLogin{StateHash: stateHash}
	err := preAuth(ctx, "take sso login", func(ctx context.Context) error {
		var user *uuid.UUID
		var email *string
		if err := s.pool.QueryRow(ctx,
			`DELETE FROM sso_logins WHERE state_hash = $1 AND expires_at > $2
			 RETURNING browser_hash, tenant_id, user_id, email`, stateHash, now,
		).Scan(&l.BrowserHash, &l.TenantID, &user, &email); err != nil {
			return err
		}
		if user != nil {
			l.UserID = *user
		}
		if email != nil {
			l.Email = *email
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown request is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// BindSSOSubject binds subject to the user, or checks it matches the bound one. false when the
// user is bound to another subject; ErrSubjectTaken when another user has this subject.
func (Store) BindSSOSubject(ctx context.Context, userID uuid.UUID, subject string) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.BindSSOSubject(ctx, sqlcgen.BindSSOSubjectParams{TenantID: scope.TenantID, ID: userID, Subject: &subject})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, ErrSubjectTaken
	}
	if err != nil {
		return false, fmt.Errorf("bind sso subject: %w", err)
	}
	return n > 0, nil
}
