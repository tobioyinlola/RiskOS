//go:build integration

package integration

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/dbmigrate"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

// TestMigrationsOnPopulatedDatabase rehearses the phase-8 migrations (00033 to 00039) the way a
// deployment meets them: on a database that already holds data. It seeds a schema at version 32,
// migrates to head, checks what the migrations were supposed to do to existing rows, rolls back
// to 32 to prove the downs, and migrates forward again.
func TestMigrationsOnPopulatedDatabase(t *testing.T) {
	const baseline = 32
	ctx := t.Context()

	_, err := shared.owner.Exec(ctx, `DROP DATABASE IF EXISTS rehearsal`)
	require.NoError(t, err)
	_, err = shared.owner.Exec(ctx, `CREATE DATABASE rehearsal`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = shared.owner.Exec(t.Context(), `DROP DATABASE IF EXISTS rehearsal WITH (FORCE)`) }) //nolint:errcheck // best effort

	dsn, err := shared.postgres.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/rehearsal"
	pool, err := db.NewPool(ctx, poolConfig(u.String()))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, dbmigrate.UpTo(ctx, pool, baseline))
	v, err := dbmigrate.Version(ctx, pool)
	require.NoError(t, err)
	require.EqualValues(t, baseline, v)

	// Data a deployment would already hold at version 32.
	tenant, env, endpoint, staffA, staffB, access := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	seed := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, name, region) VALUES ($1, 'Rehearsal Co', 'eu-west-1')`, []any{tenant}},
		{`INSERT INTO environments (id, tenant_id, kind) VALUES ($1, $2, 'test')`, []any{env, tenant}},
		{`INSERT INTO webhook_endpoints (tenant_id, environment_id, id, url, description, event_types, status,
			secret_version, previous_secret_version, previous_secret_valid_until, created_at, created_by, updated_at)
		  VALUES ($1, $2, $3, 'https://hooks.example/x', '', ARRAY['alert.created'], 'active',
			2, 1, now() + interval '1 day', now(), 'seed', now())`, []any{tenant, env, endpoint}},
		{`INSERT INTO staff (id, email, name, role, status, created_at, created_by)
		  VALUES ($1, 'sec@rehearsal.example', 'Sec', 'security_admin', 'active', now(), 'seed')`, []any{staffA}},
		{`INSERT INTO staff (id, email, name, role, status, created_at, created_by)
		  VALUES ($1, 'ops@rehearsal.example', 'Ops', 'operations_admin', 'active', now(), 'seed')`, []any{staffB}},
		{`INSERT INTO support_access (id, tenant_id, requested_by, reason, duration_minutes, status, created_at)
		  VALUES ($1, $2, $3, 'Seeded before the migrations', 30, 'pending', now())`, []any{access, tenant, staffB}},
	}
	for _, s := range seed {
		_, err := pool.Exec(ctx, s.sql, s.args...)
		require.NoError(t, err, s.sql)
	}

	// Forward to head.
	head, err := dbmigrate.Up(ctx, pool, shared.logger)
	require.NoError(t, err)
	require.Greater(t, head, int64(baseline))

	t.Run("existing rows are intact and take sensible defaults", func(t *testing.T) {
		var keyID, prevKeyID int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT secret_key_id, previous_secret_key_id FROM webhook_endpoints WHERE id = $1`, endpoint).Scan(&keyID, &prevKeyID))
		assert.Equal(t, 1, keyID, "endpoints issued before key ids are key 1")
		assert.Equal(t, 1, prevKeyID, "and so is their previous secret")

		var status string
		var reason string
		require.NoError(t, pool.QueryRow(ctx, `SELECT status, reason FROM support_access WHERE id = $1`, access).Scan(&status, &reason))
		assert.Equal(t, "pending", status)
		assert.Equal(t, "Seeded before the migrations", reason)

		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT endpoints FROM webhook_key_usage() WHERE key_id = 1`).Scan(&n))
		assert.Equal(t, 2, n, "the current secret and the one still in its overlap")
	})

	t.Run("the new functions work on that data", func(t *testing.T) {
		var tenants, staffActive, support int64
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT tenants_total, staff_active, support_pending FROM admin_overview($1)`, staffA).Scan(&tenants, &staffActive, &support))
		assert.EqualValues(t, 1, tenants)
		assert.EqualValues(t, 2, staffActive)
		assert.EqualValues(t, 1, support, "the seeded request waits for the other staff member")

		var name string
		var users int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT name, active_users FROM admin_tenant_directory() WHERE id = $1`, tenant).Scan(&name, &users))
		assert.Equal(t, "Rehearsal Co", name)

		var outcome string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT outcome FROM admin_emergency_suspend($1, $2, 'Rehearsal incident: INC-1', now())`, tenant, staffA).Scan(&outcome))
		assert.Equal(t, "applied", outcome)
		var tenantStatus string
		require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM tenants WHERE id = $1`, tenant).Scan(&tenantStatus))
		assert.Equal(t, "suspended", tenantStatus)
		// An operations administrator cannot use the emergency path, whatever the caller did.
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT outcome FROM admin_emergency_suspend($1, $2, 'Not allowed for this role', now())`, tenant, staffB).Scan(&outcome))
		assert.Equal(t, "forbidden", outcome)
	})

	t.Run("rolling back to the baseline keeps what existed before", func(t *testing.T) {
		// 00033 to 00039 and the later ones, down to the baseline.
		for range int(head) - baseline {
			require.NoError(t, dbmigrate.Down(ctx, pool, shared.logger))
		}
		v, err := dbmigrate.Version(ctx, pool)
		require.NoError(t, err)
		require.EqualValues(t, baseline, v)

		var url string
		require.NoError(t, pool.QueryRow(ctx, `SELECT url FROM webhook_endpoints WHERE id = $1`, endpoint).Scan(&url))
		assert.Equal(t, "https://hooks.example/x", url)
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM support_access`).Scan(&n))
		assert.Equal(t, 1, n)
		var exists bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('tenant_status_requests') IS NOT NULL`).Scan(&exists))
		assert.False(t, exists, "the new tables are gone")
	})

	t.Run("and forward again", func(t *testing.T) {
		again, err := dbmigrate.Up(ctx, pool, shared.logger)
		require.NoError(t, err)
		assert.Equal(t, head, again)
		var keyID int
		require.NoError(t, pool.QueryRow(ctx, `SELECT secret_key_id FROM webhook_endpoints WHERE id = $1`, endpoint).Scan(&keyID))
		assert.Equal(t, 1, keyID)
	})
}
