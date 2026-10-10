//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// TestAssigneesAreWorkspaceUsers checks work can only be assigned to an active dashboard user of
// the workspace, named by email, and that the stored value is the user's own (lowercase) email.
func TestAssigneesAreWorkspaceUsers(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withAssigneeValidation())
	s.startWorker(t)
	a := s.createTenant(t, "Assignees")
	other := s.createTenant(t, "Assignees elsewhere")
	s.onboard(t, a, "analyst@bank.example", authz.RoleAnalyst)
	s.onboard(t, other, "outsider@other.example", authz.RoleAnalyst)

	s.mustRule(t, a.testKey, alwaysReview())
	s.decide(t, a.testKey, debit(actor{"u_assign", "acc_assign", "dev_assign"}, "ben", "NG", "NGN", 5000))
	al := s.waitForAlerts(t, a.testKey, "kind=risk", 1)[0]

	t.Run("an alert", func(t *testing.T) {
		path := "/api/v1/alerts/" + al.Id
		for _, ref := range []string{"nobody@bank.example", "outsider@other.example", "just a name"} {
			r := s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"assignee": ref})
			assert.Equal(t, http.StatusBadRequest, r.status, "%q is not a user of this workspace: %s", ref, r.String())
		}
		r := s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"assignee": " Analyst@Bank.Example "})
		require.Equal(t, http.StatusOK, r.status, r.String())
		got := decode[apigen.Alert](t, r).Assignee
		require.NotNil(t, got)
		assert.Equal(t, "analyst@bank.example", *got, "the user's own email, whatever the case sent")

		r = s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"assignee": ""})
		require.Equal(t, http.StatusOK, r.status, "unassigning needs no user: %s", r.String())
		assert.Nil(t, decode[apigen.Alert](t, r).Assignee)
	})

	t.Run("a case", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/cases", a.testKey,
			map[string]any{"title": "Unknown owner", "assignee": "nobody@bank.example"})
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())

		r = s.call(t, http.MethodPost, "/api/v1/cases", a.testKey,
			map[string]any{"title": "Known owner", "assignee": "ANALYST@bank.example"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		c := decode[apigen.Case](t, r)
		require.NotNil(t, c.Assignee)
		assert.Equal(t, "analyst@bank.example", *c.Assignee)

		path := fmt.Sprintf("/api/v1/cases/%s", c.Id)
		r = s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"assignee": "outsider@other.example"})
		assert.Equal(t, http.StatusBadRequest, r.status, "another workspace's user: %s", r.String())

		r = s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"assignee": "analyst@bank.example", "priority": "high"})
		require.Equal(t, http.StatusOK, r.status, "keeping the same assignee while changing something else: %s", r.String())
	})
}
