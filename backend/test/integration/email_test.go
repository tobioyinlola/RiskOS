//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/mail"
)

// jobRecorder is a queue that records jobs instead of running them.
type jobRecorder struct {
	mu   sync.Mutex
	jobs []jobs.Args
}

func (q *jobRecorder) Enqueue(_ context.Context, a jobs.Args) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, a)
	return nil
}

func (q *jobRecorder) last(t *testing.T) auth.InviteEmailArgs {
	t.Helper()
	q.mu.Lock()
	defer q.mu.Unlock()
	require.NotEmpty(t, q.jobs)
	a, ok := q.jobs[len(q.jobs)-1].(auth.InviteEmailArgs)
	require.True(t, ok)
	return a
}

// mailbox is a mail.Sender that keeps messages.
type mailbox struct {
	sent []mail.Message
	fail error
}

func (m *mailbox) Send(_ context.Context, msg mail.Message) error {
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, msg)
	return nil
}

// TestInviteEmails covers emailed invites: the job carries no token or address, the emailed
// link works, and invites no longer pending are not emailed.
func TestInviteEmails(t *testing.T) {
	q := &jobRecorder{}
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withInviteEmails(q))
	tenant := s.createTenant(t, "Email Co")
	box := &mailbox{}
	send := s.auth.InviteEmailJob(box)

	token := s.inviteToken(t, tenant, "first@email.example", authz.RoleCustomerAdmin)
	args := q.last(t)
	assert.NotContains(t, mustJSON(t, args), token, "the queued job never holds the token")
	assert.NotContains(t, mustJSON(t, args), "first@email.example")

	// A failed send is retried by the queue; nothing is recorded.
	box.fail = errors.New("smtp down")
	require.Error(t, send(t.Context(), args))
	box.fail = nil
	require.NoError(t, send(t.Context(), args))
	require.Len(t, box.sent, 1)
	msg := box.sent[0]
	assert.Equal(t, "first@email.example", msg.To)
	assert.Equal(t, "You're invited to Email Co on RiskOS", msg.Subject)
	assert.Contains(t, msg.Text, "as customer admin")
	assert.Contains(t, msg.Text, "http://console.test/accept-invite#token="+token, "the emailed link is the invite's link")

	// The emailed link works once; afterwards the invite is not emailed again.
	b := s.browser()
	r := b.do(t, http.MethodPost, "/api/v1/auth/invites/accept", map[string]any{
		"token": token, "name": "First", "password": testPassword,
	})
	require.Equal(t, http.StatusOK, r.status, r.String())
	require.NoError(t, send(t.Context(), args))
	assert.Len(t, box.sent, 1, "an accepted invite is not emailed")

	// A revoked invite is not emailed either, and the console reports what was emailed.
	s.inviteToken(t, tenant, "second@email.example", authz.RoleAnalyst)
	second := q.last(t)
	var sentAt *time.Time
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT email_sent_at FROM user_invites WHERE id = $1`, args.InviteID).Scan(&sentAt))
	assert.NotNil(t, sentAt)
	_, err := shared.owner.Exec(t.Context(), `UPDATE user_invites SET revoked_at = now() WHERE id = $1`, second.InviteID)
	require.NoError(t, err)
	require.NoError(t, send(t.Context(), second))
	assert.Len(t, box.sent, 1, "a revoked invite is not emailed")

	actions := auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", tenant.testKey, nil))
	assert.Contains(t, actions, "user.invite_emailed")
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}
