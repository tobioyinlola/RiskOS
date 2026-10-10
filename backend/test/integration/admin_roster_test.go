//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// TestAdminStaffRoster covers ADR-0010's roster: security administrators see accounts and can
// disable and enable them (never their own), disabling ends sessions at once, and nothing in
// the response is a credential.
func TestAdminStaffRoster(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
	tokenOps, err := s.admin.CreateStaff(t.Context(), "roster-ops@riskos.example", "Roster Ops", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	tokenSec, err := s.admin.CreateStaff(t.Context(), "roster-sec@riskos.example", "Roster Sec", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	_, err = s.admin.CreateStaff(t.Context(), "roster-new@riskos.example", "Roster New", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	ops, opsKey := enroll(t, s, tokenOps)
	sec, _ := enroll(t, s, tokenSec)

	roster := func(br *staffBrowser) response { return br.do(t, http.MethodGet, "/api/admin/v1/staff", nil) }
	find := func(list adminapi.StaffRosterList, email string) *adminapi.StaffRosterEntry {
		for i := range list.Data {
			if list.Data[i].Email == email {
				return &list.Data[i]
			}
		}
		return nil
	}

	t.Run("only security administrators see it", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, roster(&staffBrowser{s: s}).status)
		assert.Equal(t, http.StatusForbidden, roster(ops).status, "operations administrators do not")
		r := roster(sec)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.NotContains(t, r.String(), "enroll_token", "never a credential")
	})

	var opsID string
	t.Run("accounts show their state", func(t *testing.T) {
		list := decode[adminapi.StaffRosterList](t, roster(sec))
		o := find(list, "roster-ops@riskos.example")
		require.NotNil(t, o)
		opsID = o.Id
		assert.Equal(t, adminapi.StaffRosterEntryStatus("active"), o.Status)
		assert.Equal(t, int64(1), o.Passkeys)
		assert.False(t, o.EnrollmentPending)
		assert.NotNil(t, o.LastSignIn)

		n := find(list, "roster-new@riskos.example")
		require.NotNil(t, n)
		assert.Equal(t, int64(0), n.Passkeys, "not enrolled yet")
		assert.True(t, n.EnrollmentPending, "the CLI's link is outstanding")
		assert.Nil(t, n.LastSignIn)
	})

	t.Run("disabling ends the person's sessions at once", func(t *testing.T) {
		require.Equal(t, http.StatusOK, ops.do(t, http.MethodGet, "/api/admin/v1/overview", nil).status)
		r := sec.do(t, http.MethodPost, "/api/admin/v1/staff/"+opsID+"/disable", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, adminapi.StaffRosterEntryStatus("disabled"), decode[adminapi.StaffRosterEntry](t, r).Status)
		assert.Equal(t, http.StatusUnauthorized, ops.do(t, http.MethodGet, "/api/admin/v1/overview", nil).status)
		assert.Equal(t, http.StatusUnauthorized, staffLogin(t, s, opsKey).status, "and they cannot sign in again")
	})

	t.Run("enabling lets them sign in again", func(t *testing.T) {
		r := sec.do(t, http.MethodPost, "/api/admin/v1/staff/"+opsID+"/enable", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, http.StatusOK, staffLogin(t, s, opsKey).status)
	})

	t.Run("never your own account, and the actions are audited", func(t *testing.T) {
		list := decode[adminapi.StaffRosterList](t, roster(sec))
		self := find(list, "roster-sec@riskos.example")
		require.NotNil(t, self)
		assert.Equal(t, http.StatusForbidden, sec.do(t, http.MethodPost, "/api/admin/v1/staff/"+self.Id+"/disable", nil).status)
		assert.Equal(t, http.StatusNotFound, sec.do(t, http.MethodPost, "/api/admin/v1/staff/stf_"+"00000000000000000000000000"+"/disable", nil).status)

		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM staff_audit_log WHERE action IN ('staff.disabled', 'staff.enabled') AND resource_id = $1`, opsID).Scan(&n))
		assert.Equal(t, 2, n)
	})
}
