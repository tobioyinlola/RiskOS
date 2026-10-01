//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// actorTxn is a transaction by user from device for amount minor units.
func actorTxn(key, user, account, device string, amount int) map[string]any {
	e := txn(key)
	e["actor"] = map[string]any{"user_id": user, "account_id": account, "device_id": device}
	data, _ := e["data"].(map[string]any) //nolint:errcheck // txn builds this map
	data["amount"] = amount
	return e
}

func TestEntitiesFiltersAndAnalytics(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Entities")
	b := s.createTenant(t, "Entities other")

	// u_ada uses two devices; dev_shared is also used by u_bo.
	for i, e := range []map[string]any{
		actorTxn("e1", "u_ada", "acc_ada", "dev_phone", 120_000),
		actorTxn("e2", "u_ada", "acc_ada", "dev_shared", 250_000),
		actorTxn("e3", "u_bo", "acc_bo", "dev_shared", 80_000),
	} {
		r := s.ingest(t, a.testKey, e)
		require.Equal(t, http.StatusOK, r.status, "event %d: %s", i, r)
	}
	login := map[string]any{
		"event_id": "l1", "type": "login", "occurred_at": time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		"actor": map[string]any{"user_id": "u_ada", "device_id": "dev_phone"},
		"data":  map[string]any{"outcome": "success", "method": "password"},
	}
	require.Equal(t, http.StatusOK, s.ingest(t, a.testKey, login).status)

	list := func(key, query string) apigen.EventList {
		r := s.call(t, http.MethodGet, "/api/v1/events?"+query, key, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.EventList](t, r)
	}

	t.Run("filters", func(t *testing.T) {
		assert.Len(t, list(a.testKey, "user=u_ada").Data, 3)
		assert.Len(t, list(a.testKey, "device=dev_shared").Data, 2)
		assert.Len(t, list(a.testKey, "account=acc_bo").Data, 1)
		txns := list(a.testKey, "type=transaction&user=u_ada").Data
		require.Len(t, txns, 2)
		require.NotNil(t, txns[0].Transaction)
		assert.Equal(t, int64(250_000), txns[0].Transaction.Amount, "newest first")
		assert.Equal(t, "NGN", txns[0].Transaction.Currency)
		require.NotNil(t, txns[0].Actor)
		assert.Equal(t, "u_ada", txns[0].Actor.UserId)
		logins := list(a.testKey, "type=login").Data
		require.Len(t, logins, 1)
		assert.Nil(t, logins[0].Transaction)

		all := list(a.testKey, "").Data
		newest := all[0].Id
		assert.Empty(t, list(a.testKey, "after="+newest).Data, "nothing newer than the newest")
		assert.Len(t, list(a.testKey, "after="+all[len(all)-1].Id).Data, len(all)-1)
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodGet, "/api/v1/events?after=nope", a.testKey, nil).status)
		assert.Empty(t, list(b.testKey, "user=u_ada").Data, "another tenant sees nothing")
	})

	t.Run("profiles", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/entities/user/u_ada", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		p := decode[apigen.EntityProfile](t, r)
		assert.Equal(t, 3, p.EventCount)
		assert.Len(t, p.Linked.Devices, 2)
		assert.Len(t, p.Linked.Accounts, 1)
		assert.Equal(t, 3, p.Decisions.Allow+p.Decisions.StepUp+p.Decisions.Review+p.Decisions.Block)

		dev := decode[apigen.EntityProfile](t, s.call(t, http.MethodGet, "/api/v1/entities/device/dev_shared", a.testKey, nil))
		users := map[string]bool{}
		for _, u := range dev.Linked.Users {
			users[u.Ref] = true
		}
		assert.Equal(t, map[string]bool{"u_ada": true, "u_bo": true}, users, "a shared device links both users")

		esc := url.PathEscape("u/../weird id")
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, "/api/v1/entities/user/"+esc, a.testKey, nil).status)
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, "/api/v1/entities/user/u_ada", b.testKey, nil).status,
			"another tenant's entity does not exist")
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodGet, "/api/v1/entities/card/x", a.testKey, nil).status)
	})

	t.Run("analytics", func(t *testing.T) {
		for _, w := range []string{"24h", "7d", "30d"} {
			r := s.call(t, http.MethodGet, "/api/v1/analytics?window="+w, a.testKey, nil)
			require.Equal(t, http.StatusOK, r.status, r.String())
			an := decode[apigen.Analytics](t, r)
			total := 0
			for _, p := range an.Series {
				total += p.Events
				assert.Equal(t, p.Events, p.Bands.Minimal+p.Bands.Low+p.Bands.Elevated+p.Bands.High+p.Bands.Critical)
			}
			assert.Equal(t, 4, total, "window %s", w)
			require.NotEmpty(t, an.TopUsers)
			assert.NotEmpty(t, an.TopSignals)
			assert.Equal(t, fmt.Sprint(map[bool]string{true: "day", false: "hour"}[w == "30d"]), string(an.Bucket))
		}
		other := decode[apigen.Analytics](t, s.call(t, http.MethodGet, "/api/v1/analytics", b.testKey, nil))
		assert.Empty(t, other.Series)
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodGet, "/api/v1/analytics?window=1y", a.testKey, nil).status)
	})
}
