package store

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
)

// Querier runs queries outside a tenant transaction. *pgxpool.Pool connected as the application
// role satisfies it.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// KeyUsage returns how many endpoint secrets, across tenants, were derived with each signing key
// id and are still in use. It calls the counts-only function the application role may execute.
func KeyUsage(ctx context.Context, q Querier) (map[int]int, error) {
	rows, err := q.Query(ctx, `SELECT key_id, endpoints FROM webhook_key_usage()`)
	if err != nil {
		return nil, fmt.Errorf("webhook key usage: %w", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id int
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan webhook key usage: %w", err)
		}
		out[id] = int(n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("webhook key usage: %w", err)
	}
	return out, nil
}

// keyID16 converts a configured key id (validated to 1..32000 at start-up) for the smallint
// column, refusing anything outside it.
func keyID16(id int) (int16, error) {
	if id < 1 || id > math.MaxInt16 {
		return 0, fmt.Errorf("webhook signing key id %d is outside 1..%d", id, math.MaxInt16)
	}
	return int16(id), nil //nolint:gosec // range checked on the line above
}
