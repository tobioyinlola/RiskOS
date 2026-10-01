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
	UserID      uuid.UUID
}

// InsertSSOLogin records an authorization request in flight, purging expired ones.
func (s Store) InsertSSOLogin(ctx context.Context, l SSOLogin, now, expiresAt time.Time) error {
	return preAuth(ctx, "record sso login", func(ctx context.Context) error {
		var one int
		return s.pool.QueryRow(ctx,
			`WITH purged AS (DELETE FROM sso_logins WHERE expires_at < $5)
			 INSERT INTO sso_logins (state_hash, browser_hash, tenant_id, user_id, created_at, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING 1`,
			l.StateHash, l.BrowserHash, l.TenantID, l.UserID, now, expiresAt,
		).Scan(&one)
	})
}

// TakeSSOLogin consumes the authorization request with stateHash; nil if unknown or expired.
func (s Store) TakeSSOLogin(ctx context.Context, stateHash []byte, now time.Time) (*SSOLogin, error) {
	l := SSOLogin{StateHash: stateHash}
	err := preAuth(ctx, "take sso login", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`DELETE FROM sso_logins WHERE state_hash = $1 AND expires_at > $2
			 RETURNING browser_hash, tenant_id, user_id`, stateHash, now,
		).Scan(&l.BrowserHash, &l.TenantID, &l.UserID)
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
