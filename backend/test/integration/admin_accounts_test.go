//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/adminapi"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// TestAdminStaffAccountCreation covers creating staff accounts from the console (ADR-0010): two
// security administrators, the enrollment link emailed to the new person and shown to no one, and
// the CLI remaining the way when email is not configured.
func TestAdminStaffAccountCreation(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin(), withAdminEmail())
	tokenOps, err := s.admin.CreateStaff(t.Context(), "acct-ops@riskos.example", "Acct Ops", authz.RoleOperationsAdmin, "integration")
	require.NoError(t, err)
	tokenB, err := s.admin.CreateStaff(t.Context(), "acct-sec-b@riskos.example", "Acct Sec B", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	tokenC, err := s.admin.CreateStaff(t.Context(), "acct-sec-c@riskos.example", "Acct Sec C", authz.RoleSecurityAdmin, "integration")
	require.NoError(t, err)
	ops, _ := enroll(t, s, tokenOps)
	b, _ := enroll(t, s, tokenB)
	c, _ := enroll(t, s, tokenC)

	const newEmail = "new.person@riskos.example"
	const reason = "Joining the on-call rotation: HR-4410"
	request := func(br *staffBrowser, email, why string) response {
		return br.do(t, http.MethodPost, "/api/admin/v1/staff/account-requests",
			map[string]any{"email": email, "name": "New Person", "role": "operations_admin", "reason": why})
	}
	decide := func(br *staffBrowser, id, verb string) response {
		return br.do(t, http.MethodPost, "/api/admin/v1/staff/account-requests/"+id+"/"+verb, nil)
	}

	t.Run("only security administrators, with a real address and reason", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, request(ops, newEmail, reason).status)
		assert.Equal(t, http.StatusUnauthorized, request(&staffBrowser{s: s}, newEmail, reason).status)
		assert.Equal(t, http.StatusBadRequest, request(b, "not an address", reason).status)
		assert.Equal(t, http.StatusBadRequest, request(b, newEmail, "short").status)
		assert.Equal(t, http.StatusConflict, request(b, "acct-ops@riskos.example", reason).status, "an account exists")
		assert.Equal(t, http.StatusForbidden, ops.do(t, http.MethodGet, "/api/admin/v1/staff/account-requests", nil).status)
	})

	var id string
	t.Run("nothing is created until another security administrator approves", func(t *testing.T) {
		r := request(b, newEmail, reason)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		got := decode[adminapi.StaffAccountRequest](t, r)
		id = got.Id
		assert.Equal(t, adminapi.StaffAccountRequestStatus("pending"), got.Status)
		assert.Nil(t, got.StaffId)

		assert.Equal(t, http.StatusConflict, request(c, newEmail, reason).status, "one pending request per address")
		assert.Equal(t, http.StatusForbidden, decide(b, id, "approve").status, "not your own")
		assert.Equal(t, http.StatusForbidden, decide(ops, id, "approve").status, "not an operations administrator")
		assert.Equal(t, http.StatusForbidden, decide(c, id, "withdraw").status, "only the requester withdraws")

		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(), `SELECT count(*) FROM staff WHERE email = $1`, newEmail).Scan(&n))
		assert.Zero(t, n)
	})

	var link string
	t.Run("approval creates the account and the link is emailed, not shown", func(t *testing.T) {
		r := decide(c, id, "approve")
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[adminapi.StaffAccountRequest](t, r)
		assert.Equal(t, adminapi.StaffAccountRequestStatus("approved"), got.Status)
		require.NotNil(t, got.StaffId)
		assert.NotContains(t, r.String(), "token", "no session is shown the link")
		assert.Equal(t, http.StatusConflict, decide(c, id, "approve").status, "decided once")

		var raw []byte
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT args FROM river_job WHERE kind = 'admin_enrollment_email' ORDER BY id DESC LIMIT 1`).Scan(&raw))
		var args admin.EnrollmentEmailArgs
		require.NoError(t, json.Unmarshal(raw, &args))
		assert.NotEqual(t, uuid.Nil, args.StaffID)
		assert.NotContains(t, string(raw), "token", "the queued job never holds the token")
		assert.NotContains(t, string(raw), newEmail)

		box := &mailbox{}
		job := s.admin.EnrollmentEmailJob(box, "https://console.example/")
		require.NoError(t, job(t.Context(), args))
		require.Len(t, box.sent, 1)
		assert.Equal(t, newEmail, box.sent[0].To)
		i := strings.Index(box.sent[0].Text, "https://console.example/admin/enroll#token=")
		require.GreaterOrEqual(t, i, 0, box.sent[0].Text)
		link = strings.Fields(box.sent[0].Text[i:])[0]

		// At least once: a retry before the link is used sends the same one again.
		require.NoError(t, job(t.Context(), args))
		assert.Len(t, box.sent, 2)
	})

	t.Run("the emailed link enrolls the person, once", func(t *testing.T) {
		token := link[strings.Index(link, "#token=")+len("#token="):]
		person, _ := enroll(t, s, token)
		assert.Equal(t, http.StatusOK, person.do(t, http.MethodGet, "/api/admin/v1/overview", nil).status)

		var raw []byte
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT args FROM river_job WHERE kind = 'admin_enrollment_email' ORDER BY id DESC LIMIT 1`).Scan(&raw))
		var args admin.EnrollmentEmailArgs
		require.NoError(t, json.Unmarshal(raw, &args))
		box := &mailbox{}
		require.NoError(t, s.admin.EnrollmentEmailJob(box, "https://console.example")(t.Context(), args))
		assert.Empty(t, box.sent, "a used link is not re-sent")
	})

	t.Run("rejection and withdrawal create nothing", func(t *testing.T) {
		r := request(b, "second@riskos.example", reason)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		rej := decide(c, decode[adminapi.StaffAccountRequest](t, r).Id, "reject")
		require.Equal(t, http.StatusOK, rej.status, rej.String())
		assert.Equal(t, adminapi.StaffAccountRequestStatus("rejected"), decode[adminapi.StaffAccountRequest](t, rej).Status)

		r = request(b, "third@riskos.example", reason)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		w := decide(b, decode[adminapi.StaffAccountRequest](t, r).Id, "withdraw")
		require.Equal(t, http.StatusOK, w.status, w.String())

		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			`SELECT count(*) FROM staff WHERE email IN ('second@riskos.example', 'third@riskos.example')`).Scan(&n))
		assert.Zero(t, n)
	})

	t.Run("without email the console cannot create accounts", func(t *testing.T) {
		noMail := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAdmin())
		tok, err := noMail.admin.CreateStaff(t.Context(), "nomail-sec@riskos.example", "No Mail Sec", authz.RoleSecurityAdmin, "integration")
		require.NoError(t, err)
		sec, _ := enroll(t, noMail, tok)
		r := request(sec, "nomail-new@riskos.example", reason)
		assert.Equal(t, http.StatusConflict, r.status, r.String())
	})
}
