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

// Sign-ups and Google requests are read and written before any tenant exists (no RLS): each is
// found only by the hash of a token held in the browser, or by its ID from the email job.

// Signup is a sign-up in flight.
type Signup struct {
	ID              uuid.UUID
	Email           string
	Name            string
	BusinessName    *string
	PasswordHash    *string
	GoogleSubject   *string
	ExistingAccount bool
	EmailVerifiedAt *time.Time
	CodeSentCount   int
	CodeSentAt      *time.Time
	FailedAttempts  int
	CreatedAt       time.Time
	ExpiresAt       time.Time
	CompletedAt     *time.Time
}

// NewSignup describes a sign-up to record.
type NewSignup struct {
	ID              uuid.UUID
	TokenHash       []byte
	Email           string
	Name            string
	BusinessName    *string
	PasswordHash    *string
	GoogleSubject   *string
	ExistingAccount bool
	EmailVerifiedAt *time.Time
	// CodeSent records that the first code goes out now.
	CodeSent  bool
	Now       time.Time
	ExpiresAt time.Time
}

const signupColumns = `id, email, name, business_name, password_hash, google_subject, existing_account,
	email_verified_at, code_sent_count, code_sent_at, failed_attempts, created_at, expires_at, completed_at`

func scanSignup(row pgx.Row) (Signup, error) {
	var s Signup
	err := row.Scan(&s.ID, &s.Email, &s.Name, &s.BusinessName, &s.PasswordHash, &s.GoogleSubject, &s.ExistingAccount,
		&s.EmailVerifiedAt, &s.CodeSentCount, &s.CodeSentAt, &s.FailedAttempts, &s.CreatedAt, &s.ExpiresAt, &s.CompletedAt)
	return s, err
}

// InsertSignup records a sign-up, purging expired ones.
func (s Store) InsertSignup(ctx context.Context, n NewSignup) error {
	count := 0
	var sentAt *time.Time
	if n.CodeSent {
		count, sentAt = 1, &n.Now
	}
	return preAuth(ctx, "record sign-up", func(ctx context.Context) error {
		var one int
		return s.pool.QueryRow(ctx,
			`WITH purged AS (DELETE FROM signups WHERE expires_at < $13)
			 INSERT INTO signups (id, token_hash, email, name, business_name, password_hash, google_subject,
			                      existing_account, email_verified_at, code_sent_count, code_sent_at, created_at, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING 1`,
			n.ID, n.TokenHash, n.Email, n.Name, n.BusinessName, n.PasswordHash, n.GoogleSubject,
			n.ExistingAccount, n.EmailVerifiedAt, count, sentAt, n.Now, n.ExpiresAt,
		).Scan(&one)
	})
}

// SignupByToken finds a sign-up by its token hash; nil if unknown.
func (s Store) SignupByToken(ctx context.Context, tokenHash []byte) (*Signup, error) {
	var out Signup
	err := preAuth(ctx, "look up sign-up", func(ctx context.Context) error {
		var err error
		out, err = scanSignup(s.pool.QueryRow(ctx, `SELECT `+signupColumns+` FROM signups WHERE token_hash = $1`, tokenHash))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown sign-up is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SignupByID finds a sign-up by ID (the email job); nil if unknown.
func (s Store) SignupByID(ctx context.Context, id uuid.UUID) (*Signup, error) {
	var out Signup
	err := preAuth(ctx, "look up sign-up", func(ctx context.Context) error {
		var err error
		out, err = scanSignup(s.pool.QueryRow(ctx, `SELECT `+signupColumns+` FROM signups WHERE id = $1`, id))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // unknown sign-up is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// signupUpdate runs an UPDATE ... RETURNING 1 on one sign-up and reports whether a row changed.
func (s Store) signupUpdate(ctx context.Context, what, sql string, args ...any) (bool, error) {
	err := preAuth(ctx, what, func(ctx context.Context) error {
		var one int
		return s.pool.QueryRow(ctx, sql+` RETURNING 1`, args...).Scan(&one)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// NextSignupCode moves a sign-up from code number from to from+1 (a new code goes out); false
// when another request moved it first.
func (s Store) NextSignupCode(ctx context.Context, id uuid.UUID, from int, now time.Time) (bool, error) {
	return s.signupUpdate(ctx, "record sign-up code",
		`UPDATE signups SET code_sent_count = code_sent_count + 1, code_sent_at = $3, failed_attempts = 0
		 WHERE id = $1 AND code_sent_count = $2 AND completed_at IS NULL`, id, from, now)
}

// SignupFailedAttempt counts a wrong code and returns the total so far.
func (s Store) SignupFailedAttempt(ctx context.Context, id uuid.UUID) (int, error) {
	var n int
	err := preAuth(ctx, "count sign-up attempt", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`UPDATE signups SET failed_attempts = failed_attempts + 1 WHERE id = $1 RETURNING failed_attempts`, id,
		).Scan(&n)
	})
	return n, err
}

// VerifySignupEmail records that the sign-up's email is proven.
func (s Store) VerifySignupEmail(ctx context.Context, id uuid.UUID, now time.Time) error {
	_, err := s.signupUpdate(ctx, "verify sign-up email",
		`UPDATE signups SET email_verified_at = coalesce(email_verified_at, $2) WHERE id = $1`, id, now)
	return err
}

// SetSignupBusiness names the workspace of a pending sign-up.
func (s Store) SetSignupBusiness(ctx context.Context, id uuid.UUID, name string) error {
	_, err := s.signupUpdate(ctx, "name sign-up workspace",
		`UPDATE signups SET business_name = $2 WHERE id = $1 AND completed_at IS NULL`, id, name)
	return err
}

// CompleteSignup marks a sign-up done; false when it already was.
func (s Store) CompleteSignup(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	return s.signupUpdate(ctx, "complete sign-up",
		`UPDATE signups SET completed_at = $2 WHERE id = $1 AND completed_at IS NULL`, id, now)
}

// InsertGoogleLogin records a Google authorization request in flight, purging expired ones.
func (s Store) InsertGoogleLogin(ctx context.Context, stateHash, browserHash []byte, now, expiresAt time.Time) error {
	return preAuth(ctx, "record google login", func(ctx context.Context) error {
		var one int
		return s.pool.QueryRow(ctx,
			`WITH purged AS (DELETE FROM google_logins WHERE expires_at < $3)
			 INSERT INTO google_logins (state_hash, browser_hash, created_at, expires_at)
			 VALUES ($1, $2, $3, $4) RETURNING 1`, stateHash, browserHash, now, expiresAt,
		).Scan(&one)
	})
}

// TakeGoogleLogin consumes the request with stateHash and returns its browser hash; nil if
// unknown or expired.
func (s Store) TakeGoogleLogin(ctx context.Context, stateHash []byte, now time.Time) ([]byte, error) {
	var browser []byte
	err := preAuth(ctx, "take google login", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`DELETE FROM google_logins WHERE state_hash = $1 AND expires_at > $2 RETURNING browser_hash`, stateHash, now,
		).Scan(&browser)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return browser, err
}

// GoogleUser is the user bound to a Google subject (pre-authentication).
type GoogleUser struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Status       string
	TenantStatus string
}

// UserByGoogleSubject finds the user bound to subject; nil if none.
func (s Store) UserByGoogleSubject(ctx context.Context, subject string) (*GoogleUser, error) {
	var u GoogleUser
	err := preAuth(ctx, "look up google user", func(ctx context.Context) error {
		return s.pool.QueryRow(ctx,
			`SELECT id, tenant_id, status, tenant_status FROM auth_user_by_google_subject($1)`, subject,
		).Scan(&u.ID, &u.TenantID, &u.Status, &u.TenantStatus)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no bound user is a normal outcome
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// BindGoogleSubject binds Google's subject to the user, or checks it matches the bound one. false
// when the user is bound to another subject; ErrSubjectTaken when another user has this subject.
func (Store) BindGoogleSubject(ctx context.Context, userID uuid.UUID, subject string, now time.Time) (bool, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return false, err
	}
	n, err := q.BindGoogleSubject(ctx, sqlcgen.BindGoogleSubjectParams{TenantID: scope.TenantID, ID: userID, Subject: &subject, Now: now})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, ErrSubjectTaken
	}
	if err != nil {
		return false, fmt.Errorf("bind google subject: %w", err)
	}
	return n > 0, nil
}

// IsUniqueViolation reports whether err is a unique constraint violation (an email taken by a
// concurrent sign-up).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
