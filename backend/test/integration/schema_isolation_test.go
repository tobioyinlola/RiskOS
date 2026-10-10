//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every table that holds tenant data is isolated by row-level security with a policy, so a table
// added later cannot skip TENANCY.md's second layer unnoticed. The exceptions are deliberate.
func TestEveryTenantTableIsIsolated(t *testing.T) {
	exceptions := map[string]string{
		"sso_logins":      "pre-authentication state with no tenant scope yet (00015_sso.sql)",
		"staff_audit_log": "staff realm; the application role cannot write or delete it (00016_admin_realm.sql)",
		"support_access":  "staff realm, four-eyes support access (00016_admin_realm.sql)",
		"tenant_status_requests": "staff realm, four-eyes suspension and reactivation; read across tenants by staff, " +
			"and decided only through admin_decide_tenant_status() (00036_tenant_status_requests.sql)",
	}
	rows, err := shared.owner.Query(t.Context(), `
		SELECT c.relname, c.relrowsecurity, (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		  AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped)`)
	require.NoError(t, err)
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var rls bool
		var policies int
		require.NoError(t, rows.Scan(&name, &rls, &policies))
		seen[name] = true
		if why, ok := exceptions[name]; ok {
			t.Logf("%s: no row-level security: %s", name, why)
			continue
		}
		assert.True(t, rls && policies > 0, "%s holds tenant data without row-level security and a policy", name)
	}
	require.NoError(t, rows.Err())
	for _, name := range []string{"compliance_tasks", "compliance_accounts", "pack_rule_versions", "pack_rule_changes", "compliance_fx_rates", "jurisdiction_activations", "activation_changes", "gap_verifications"} {
		assert.True(t, seen[name], "%s is checked", name)
	}

	// Compliance work and the audit trail are never deleted by the application.
	for _, table := range []string{"compliance_tasks", "audit_log", "pack_rule_changes", "pack_rule_versions", "activation_changes", "gap_verifications"} {
		var canDelete bool
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			"SELECT has_table_privilege('riskos_app', $1, 'DELETE')", table).Scan(&canDelete))
		assert.False(t, canDelete, "riskos_app can delete from %s", table)
	}
}
