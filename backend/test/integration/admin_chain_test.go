//go:build integration

package integration

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// TestStaffAuditChain covers the staff audit log's hash chain: appends chain even when they race,
// the table cannot be changed (even by the owner), and verification finds a modified entry and a
// truncated tail.
func TestStaffAuditChain(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	token, err := s.admin.CreateStaff(t.Context(), "chain-ops@riskos.example", "Chain Ops", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	a, key := enroll(t, s, token)

	t.Run("entries chain, including ones written at the same moment", func(t *testing.T) {
		// One forged key per goroutine: a software key's sign counter is not safe to share.
		codes := make([]int, 12)
		forged := make([]*softKey, len(codes))
		for i := range forged {
			forged[i] = newSoftKey(t)
			forged[i].id, forged[i].user, forged[i].rpHash = key.id, key.user, key.rpHash
		}
		var wg sync.WaitGroup
		for i := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = staffLogin(t, s, forged[i]).status
			}()
		}
		wg.Wait()
		for _, c := range codes {
			assert.Equal(t, http.StatusUnauthorized, c)
		}
		require.Equal(t, http.StatusOK, a.do(t, http.MethodGet, "/api/admin/v1/overview", nil).status)

		res, err := admin.VerifyStaffAudit(t.Context(), shared.owner)
		require.NoError(t, err)
		assert.True(t, res.Valid, res.Reason)
		assert.GreaterOrEqual(t, res.Entries, int64(13))
		assert.Equal(t, res.Entries, res.HeadSeq)
	})

	t.Run("nobody can change it, the owner included", func(t *testing.T) {
		for _, stmt := range []string{
			`UPDATE staff_audit_log SET action = 'x' WHERE seq IS NOT NULL`,
			`DELETE FROM staff_audit_log WHERE seq IS NOT NULL`,
			`TRUNCATE staff_audit_log`,
		} {
			_, err := shared.owner.Exec(t.Context(), stmt)
			require.Error(t, err, stmt)
			assert.Contains(t, err.Error(), "append-only")
		}
	})

	// Tampering needs the trigger out of the way; every case runs in a transaction that is rolled
	// back, so the log is untouched afterwards.
	tamper := func(t *testing.T, statements ...string) admin.ChainResult {
		t.Helper()
		tx, err := shared.owner.Begin(t.Context())
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(t.Context()) }() //nolint:errcheck // the point is to discard it
		_, err = tx.Exec(t.Context(), `ALTER TABLE staff_audit_log DISABLE TRIGGER staff_audit_no_update_delete`)
		require.NoError(t, err)
		for _, stmt := range statements {
			_, err := tx.Exec(t.Context(), stmt)
			require.NoError(t, err, stmt)
		}
		res, err := admin.VerifyStaffAudit(t.Context(), tx)
		require.NoError(t, err)
		return res
	}

	t.Run("a modified entry is found", func(t *testing.T) {
		res := tamper(t, `UPDATE staff_audit_log SET action = 'staff.nothing_happened'
			WHERE seq = (SELECT min(seq) + 3 FROM staff_audit_log)`)
		assert.False(t, res.Valid)
		assert.Contains(t, res.Reason, "modified")
		assert.Positive(t, res.FirstInvalidSeq)
	})

	t.Run("a removed entry is found", func(t *testing.T) {
		res := tamper(t, `DELETE FROM staff_audit_log WHERE seq = (SELECT min(seq) + 3 FROM staff_audit_log)`)
		assert.False(t, res.Valid)
		assert.Contains(t, res.Reason, "removed or reordered")
	})

	t.Run("a truncated tail is found by the head", func(t *testing.T) {
		res := tamper(t, `DELETE FROM staff_audit_log WHERE seq = (SELECT max(seq) FROM staff_audit_log)`)
		assert.False(t, res.Valid)
		assert.True(t, strings.Contains(res.Reason, "head"), res.Reason)
	})

	t.Run("a rewritten entry with a recomputed hash still breaks the next link", func(t *testing.T) {
		// Changing metadata_text alone is caught by the hash; the chain also pins each entry to
		// the one before, so rewriting history needs every later hash too.
		res := tamper(t, `UPDATE staff_audit_log SET metadata_text = '{"forged":true}'
			WHERE seq = (SELECT min(seq) + 1 FROM staff_audit_log)`)
		assert.False(t, res.Valid)
	})

	t.Run("the untouched log still verifies", func(t *testing.T) {
		res, err := admin.VerifyStaffAudit(t.Context(), shared.owner)
		require.NoError(t, err)
		assert.True(t, res.Valid, res.Reason)
	})
}
