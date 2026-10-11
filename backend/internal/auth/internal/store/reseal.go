package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB is what resealing needs: it runs as the schema owner, across tenants (operator tooling
// only, ADR-0008), so it is not the application role's tenant-scoped transaction.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Sealed is a ciphertext stored in a table, with the identifier it is bound to (the user, or the
// tenant for an SSO client secret).
type Sealed struct {
	ID         uuid.UUID
	Ciphertext []byte
}

// SealedKind names a table of sealed values.
type SealedKind string

// The tables that hold values sealed under the console secret.
const (
	SealedTOTP SealedKind = "totp"
	SealedSSO  SealedKind = "sso"
)

func (k SealedKind) queries() (list, update string, err error) {
	switch k {
	case SealedTOTP:
		return `SELECT id, totp_secret_enc FROM users WHERE totp_secret_enc IS NOT NULL ORDER BY id`,
			`UPDATE users SET totp_secret_enc = $3 WHERE id = $1 AND totp_secret_enc = $2`, nil
	case SealedSSO:
		return `SELECT tenant_id, client_secret_enc FROM sso_connections ORDER BY tenant_id`,
			`UPDATE sso_connections SET client_secret_enc = $3 WHERE tenant_id = $1 AND client_secret_enc = $2`, nil
	}
	return "", "", fmt.Errorf("unknown sealed kind %q", k)
}

// ListSealed returns every sealed value of a kind, across tenants.
func ListSealed(ctx context.Context, db DB, kind SealedKind) ([]Sealed, error) {
	list, _, err := kind.queries()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, list)
	if err != nil {
		return nil, fmt.Errorf("list sealed %s: %w", kind, err)
	}
	defer rows.Close()
	var out []Sealed
	for rows.Next() {
		var s Sealed
		if err := rows.Scan(&s.ID, &s.Ciphertext); err != nil {
			return nil, fmt.Errorf("scan sealed %s: %w", kind, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ReplaceSealed stores replacement for the value old of id, only if it is still old (compare and
// swap), and reports whether it did.
func ReplaceSealed(ctx context.Context, db DB, kind SealedKind, id uuid.UUID, old, replacement []byte) (bool, error) {
	_, update, err := kind.queries()
	if err != nil {
		return false, err
	}
	tag, err := db.Exec(ctx, update, id, old, replacement)
	if err != nil {
		return false, fmt.Errorf("replace sealed %s: %w", kind, err)
	}
	return tag.RowsAffected() == 1, nil
}
