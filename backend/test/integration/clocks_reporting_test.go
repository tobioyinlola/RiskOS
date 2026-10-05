//go:build integration

package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// activity is a country-pack activity event (an FIC order, a breach) booked by an entity, at a
// time, with compliance facts.
func activity(id, kind, entity string, at time.Time, facts map[string]any) map[string]any {
	c := map[string]any{"kind": kind, "facts": facts}
	if entity != "" {
		c["booking_entity"] = entity
	}
	return map[string]any{
		"event_id": id, "type": "activity", "occurred_at": at.Format(time.RFC3339Nano),
		"actor":      map[string]any{"user_id": "u_" + id, "account_id": "acc_" + id},
		"compliance": c,
	}
}

// workingDaysEnd is the close of the nth weekday after at's local day, skipping holidays: the
// calendar the working-day clocks must follow, counted independently of the implementation.
func workingDaysEnd(at time.Time, n int, loc *time.Location, holidays ...string) time.Time {
	local := at.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for counted := 0; counted < n; {
		day = day.AddDate(0, 0, 1)
		if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday && !slices.Contains(holidays, day.Format(time.DateOnly)) {
			counted++
		}
	}
	return day.AddDate(0, 0, 1)
}

func tasksOf(t *testing.T, b *browser, query string) apigen.ComplianceQueue {
	t.Helper()
	r := b.do(t, http.MethodGet, "/api/v1/compliance/tasks"+query, nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	return decode[apigen.ComplianceQueue](t, r)
}

func taskOfRule(list []apigen.ComplianceTask, rule, kind string) *apigen.ComplianceTask {
	for i := range list {
		if list[i].Rule == rule && list[i].Kind == kind {
			return &list[i]
		}
	}
	return nil
}

// Phase 5 of ADR-0007: clocks counted in working days, duties that follow the data subject
// across borders, tasks that carry their country, the live load of every clock, and retention
// across countries.
func TestClocksAndReportingAcrossCountries(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	s.startWorker(t)
	a := s.createTenant(t, "Clocks across countries")
	officer := s.onboard(t, a, "officer@clocks.example", authz.RoleComplianceOfficer)
	analyst := s.onboard(t, a, "analyst@clocks.example", authz.RoleAnalyst)
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", nil)
	activate(t, a.id, "RW", "rw", "rw-emi", "emi", nil)
	accra, err := time.LoadLocation("Africa/Accra")
	require.NoError(t, err)
	decide := func(e map[string]any) apigen.RiskDecision {
		t.Helper()
		r := s.ingest(t, a.testKey, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}

	t.Run("readiness warns while no public holiday is entered for a working-day clock", func(t *testing.T) {
		r := officer.b.do(t, http.MethodGet, "/api/v1/compliance/activations", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		acts := decode[struct {
			Data []apigen.JurisdictionActivation `json:"data"`
		}](t, r).Data
		i := slices.IndexFunc(acts, func(x apigen.JurisdictionActivation) bool { return x.Jurisdiction == "GH" })
		require.GreaterOrEqual(t, i, 0)
		r = officer.b.do(t, http.MethodGet, "/api/v1/compliance/activations/"+acts[i].Id+"/readiness", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Contains(t, r.String(), `"kind":"holidays"`)
		assert.Contains(t, r.String(), "GH-REG-11")
	})

	// A public holiday three working days from today, inside the hold.
	at := time.Now().Add(-time.Minute)
	holiday := workingDaysEnd(at, 3, accra).AddDate(0, 0, -1).Format(time.DateOnly)
	t.Run("an officer enters a public holiday; nobody else can", func(t *testing.T) {
		path := "/api/v1/compliance/jurisdictions/GH/holidays"
		body := map[string]any{"date": holiday, "name": "Test holiday"}
		assert.Equal(t, http.StatusForbidden, analyst.b.do(t, http.MethodPost, path, body).status)
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPost, path, a.testKey, body).status, "people only")
		r := officer.b.do(t, http.MethodPost, path, body)
		require.Equal(t, http.StatusCreated, r.status, r.String())
		assert.Equal(t, http.StatusConflict, officer.b.do(t, http.MethodPost, path, body).status, "one holiday a day")
		assert.Equal(t, http.StatusNotFound, officer.b.do(t, http.MethodPost, "/api/v1/compliance/jurisdictions/ZZ/holidays", body).status)
		r = analyst.b.do(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Contains(t, r.String(), holiday)
	})

	t.Run("an FIC suspension holds for 7 working days, skipping weekends and the holiday, and tips nobody off", func(t *testing.T) {
		d := decide(activity("fic-1", "fic_order_received", "gh-bank", at, map[string]any{"order.source": "FIC", "order.type": "suspension"}))
		assert.Equal(t, apigen.Decision("hold"), d.Decision)
		c := controlOf(d, "compliance_review")
		require.NotNil(t, c, "the hold stands under a masked name: %v", ruleIDs(d))
		require.NotNil(t, c.HoldUntil)
		want := workingDaysEnd(at, 7, accra, holiday)
		assert.True(t, want.Equal(*c.HoldUntil), "hold until %s, got %s", want, c.HoldUntil.In(accra))
		require.NotNil(t, c.HoldSeconds)
		assert.InDelta(t, want.Sub(at).Seconds(), float64(*c.HoldSeconds), 1)
		assert.Empty(t, c.Tasks, "the case it opens is not named")
		assert.NotContains(t, ruleIDs(d), "GH-REG-11")
		raw := s.call(t, http.MethodGet, "/api/v1/events/"+*d.EventId, a.testKey, nil).String()
		assert.NotContains(t, raw, "GH-REG-11")

		var task *apigen.ComplianceTask
		require.Eventually(t, func() bool {
			task = taskOfRule(tasksOf(t, officer.b, "?jurisdiction=GH").Data, "GH-REG-11", "case.open")
			return task != nil
		}, 20*time.Second, 100*time.Millisecond)
		assert.True(t, task.Sensitive, "an FIC order's work is for compliance officers only")
		assert.Equal(t, "GH", task.Jurisdiction)
		assert.Nil(t, taskOfRule(tasksOf(t, analyst.b, "").Data, "GH-REG-11", "case.open"))
	})

	t.Run("a breach of a Rwandan data subject's data booked in Ghana starts Rwanda's 48-hour clock", func(t *testing.T) {
		e := activity("breach-1", "breach_confirmed", "gh-bank", at, map[string]any{"breach.personal_data": true, "breach.high_risk": true})
		c, _ := e["compliance"].(map[string]any)
		c["data_subject_country"] = "RW"
		d := decide(e)
		require.NotNil(t, d.Jurisdiction)
		assert.Equal(t, "GH", *d.Jurisdiction.Jurisdiction, "Ghana still governs the event")
		require.NotNil(t, d.Jurisdiction.DataSubjectCountry)
		assert.Equal(t, "RW", *d.Jurisdiction.DataSubjectCountry)
		require.NotNil(t, d.Jurisdiction.DataSubjectPack)
		assert.True(t, strings.HasPrefix(*d.Jurisdiction.DataSubjectPack, "rw@"))
		ctl := controlOf(d, "RW-SEC-04")
		require.NotNil(t, ctl, "%v", ruleIDs(d))
		require.NotNil(t, ctl.Jurisdiction)
		assert.Equal(t, "RW", *ctl.Jurisdiction)
		assert.Equal(t, "High risk to the people affected", *ctl.Branch)
		for _, id := range ruleIDs(d) {
			assert.False(t, strings.HasPrefix(id, "RW-") && id != "RW-SEC-04", "only Rwanda's data-subject duties run: %s", id)
		}

		var ncsa *apigen.ComplianceTask
		require.Eventually(t, func() bool {
			ncsa = taskOfRule(tasksOf(t, analyst.b, "?jurisdiction=RW").Data, "RW-SEC-04", "report.NCSA")
			return ncsa != nil
		}, 20*time.Second, 100*time.Millisecond)
		assert.Equal(t, "RW", ncsa.Jurisdiction)
		require.NotNil(t, ncsa.EventJurisdiction)
		assert.Equal(t, "GH", *ncsa.EventJurisdiction)
		require.NotNil(t, ncsa.Authority)
		assert.Equal(t, "NCSA", *ncsa.Authority)
		require.NotNil(t, ncsa.DueAt)
		assert.WithinDuration(t, at.Add(48*time.Hour), *ncsa.DueAt, time.Second, "48 hours from awareness")
		assert.False(t, ncsa.Sensitive)
		for _, task := range tasksOf(t, analyst.b, "?jurisdiction=RW").Data {
			assert.Equal(t, "RW", task.Jurisdiction, "the filter keeps one country")
		}

		r := analyst.b.do(t, http.MethodGet, "/api/v1/compliance/cross-border", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		events := decode[struct {
			Data []apigen.CrossBorderEvent `json:"data"`
		}](t, r).Data
		i := slices.IndexFunc(events, func(e apigen.CrossBorderEvent) bool { return e.EventId == *d.EventId })
		require.GreaterOrEqual(t, i, 0, "the event is in the cross-border view")
		assert.Equal(t, "GH", events[i].BookedIn)
		assert.Equal(t, []string{"GH", "RW"}, events[i].Countries)
		assert.NotNil(t, taskOfRule(events[i].Tasks, "RW-SEC-04", "report.NCSA"))
	})

	t.Run("an event booked in the data subject's own country runs its rule once", func(t *testing.T) {
		e := activity("breach-2", "breach_confirmed", "rw-emi", at, map[string]any{"breach.personal_data": true, "breach.high_risk": false})
		c, _ := e["compliance"].(map[string]any)
		c["data_subject_country"] = "RW"
		d := decide(e)
		assert.Nil(t, d.Jurisdiction.DataSubjectPack)
		n := 0
		for _, ctl := range d.Controls {
			if ctl.Rule == "RW-SEC-04" {
				n++
			}
		}
		assert.Equal(t, 1, n)
	})

	t.Run("without an activation in the data subject's country nothing is guessed", func(t *testing.T) {
		b := s.createTenant(t, "Ghana only")
		activate(t, b.id, "GH", "gh", "gh-bank", "bank", nil)
		e := activity("breach-3", "breach_confirmed", "gh-bank", at, map[string]any{"breach.personal_data": true})
		c, _ := e["compliance"].(map[string]any)
		c["data_subject_country"] = "RW"
		r := s.ingest(t, b.testKey, e)
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := decode[apigen.RiskDecision](t, r)
		assert.Equal(t, "RW", *d.Jurisdiction.DataSubjectCountry)
		assert.Nil(t, d.Jurisdiction.DataSubjectPack)
		assert.Nil(t, controlOf(d, "RW-SEC-04"))
	})

	t.Run("a suspicious transaction report to Ghana's FIC: 24 hours, no goAML check", func(t *testing.T) {
		r := s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Ghana suspicion"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		caseID := decode[apigen.Case](t, r).Id
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/suspicion", map[string]any{"case_id": caseID, "jurisdiction": "GH"})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		str := decode[apigen.ComplianceTask](t, r)
		assert.Equal(t, "GH-REG-06", str.Rule)
		assert.Equal(t, "GH", str.Jurisdiction)
		require.NotNil(t, str.Authority)
		assert.Equal(t, "FIC", *str.Authority)
		require.NotNil(t, str.DueAt)
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), *str.DueAt, time.Minute)
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/tasks/"+str.Id+"/approve", nil)
		require.Equal(t, http.StatusOK, r.status, "goAML's checks are Nigeria's (REG-19): %s", r.String())

		r = s.call(t, http.MethodPost, "/api/v1/cases", a.testKey, map[string]any{"title": "Nigeria suspicion"})
		ngCase := decode[apigen.Case](t, r).Id
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/suspicion", map[string]any{"case_id": ngCase})
		require.Equal(t, http.StatusCreated, r.status, r.String())
		ng := decode[apigen.ComplianceTask](t, r)
		assert.Equal(t, "REG-14", ng.Rule, "the fallback country's rule when none is named")
		assert.Equal(t, "NFIU", *ng.Authority)
		r = officer.b.do(t, http.MethodPost, "/api/v1/compliance/tasks/"+ng.Id+"/approve", nil)
		assert.Equal(t, http.StatusBadRequest, r.status, "goAML would reject it: %s", r.String())
	})

	t.Run("the live load of every clock, with suspicious reports counted for officers only", func(t *testing.T) {
		load := func(b *browser) apigen.ClockLoad {
			t.Helper()
			r := b.do(t, http.MethodGet, "/api/v1/compliance/clock-load", nil)
			require.Equal(t, http.StatusOK, r.status, r.String())
			return decode[apigen.ClockLoad](t, r)
		}
		row := func(l apigen.ClockLoad, country, clock string) apigen.ClockLoadRow {
			t.Helper()
			i := slices.IndexFunc(l.Data, func(r apigen.ClockLoadRow) bool { return r.Jurisdiction == country && r.Clock.Id == clock })
			require.GreaterOrEqual(t, i, 0, "%s %s", country, clock)
			return l.Data[i]
		}
		o := load(officer.b)
		assert.False(t, o.SensitiveHidden)
		assert.Equal(t, 1, row(o, "GH", "fic_suspension").Running)
		fic := row(o, "GH", "fic_suspension").Clock
		assert.Nil(t, fic.Hours, "a working-day clock has no fixed hours")
		require.NotNil(t, fic.WorkingDays)
		assert.Equal(t, 7, *fic.WorkingDays)
		assert.Equal(t, 1, row(o, "GH", "str_filing").Running, "the approved FIC report still runs until filed")
		// The Ghana-booked breach of a Rwandan's data, and the Rwanda-booked one.
		assert.Equal(t, 2, row(o, "RW", "ncsa_breach").Running)
		assert.GreaterOrEqual(t, row(o, "RW", "ncsa_breach").DueSoon, 0)

		an := load(analyst.b)
		assert.True(t, an.SensitiveHidden)
		assert.Zero(t, row(an, "GH", "fic_suspension").Running, "not even a count of compliance-only work")
		assert.Zero(t, row(an, "GH", "str_filing").Running)
		assert.Equal(t, 2, row(an, "RW", "ncsa_breach").Running)
	})

	t.Run("retention: the longest period of every activated country wins, and a refusal is logged", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/compliance/retention", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		periods := decode[struct {
			Data []apigen.RetentionPeriod `json:"data"`
		}](t, r).Data
		i := slices.IndexFunc(periods, func(p apigen.RetentionPeriod) bool { return p.RecordType == "customer_file" })
		require.GreaterOrEqual(t, i, 0)
		p := periods[i]
		assert.Equal(t, 3650, p.Days, "Rwanda's 10 years beat Nigeria's and Ghana's 5")
		require.NotNil(t, p.Rule)
		assert.Equal(t, "RW", p.Rule.Jurisdiction)
		assert.Equal(t, "RW-GOV-01", p.Rule.Rule)
		got := map[string]int{}
		for _, c := range p.Considered {
			got[c.Jurisdiction+" "+c.Rule] = c.Days
		}
		assert.Equal(t, 1825, got["NG GOV-08"])
		assert.Equal(t, 1825, got["GH GH-GOV-01"])
		assert.Equal(t, 3650, got["RW RW-GOV-10"])

		check := func(since time.Time) apigen.DeletionVerdict {
			t.Helper()
			r := s.call(t, http.MethodPost, "/api/v1/compliance/retention/check", a.testKey, map[string]any{
				"record_type": "customer_file", "record_ref": "cust_42", "since": since.Format(time.RFC3339), "reason": "data_subject_request",
			})
			require.Equal(t, http.StatusOK, r.status, r.String())
			return decode[apigen.DeletionVerdict](t, r)
		}
		v := check(time.Now().AddDate(-6, 0, 0))
		assert.False(t, v.Allowed, "six years is past Ghana's and Nigeria's five, not Rwanda's ten")
		assert.WithinDuration(t, time.Now().AddDate(-6, 0, 0).Add(3650*24*time.Hour), v.RetainUntil, time.Minute)
		assert.True(t, check(time.Now().AddDate(-11, 0, 0)).Allowed)
		logs := s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil).String()
		assert.Contains(t, logs, "compliance.deletion_denied")
		assert.Contains(t, logs, "compliance.deletion_allowed")
		assert.Contains(t, logs, `"rule":"RW-GOV-01"`)

		r = s.call(t, http.MethodPost, "/api/v1/compliance/retention/check", a.testKey, map[string]any{
			"record_type": "customer_file", "record_ref": "cust_42", "since": time.Now().Format(time.RFC3339), "reason": "because",
		})
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	})
}

// No tipping off across the Ghana and Rwanda packs (GH-GOV-02, RW-GOV-02): every compliance-only
// rule is fired, then every payload a non-compliance role, an API key or a webhook endpoint can
// read is walked for its ID, its name and its reason. Only compliance officers read them.
func TestNoTippingOffAcrossCountries(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	s.startWorker(t)
	a := s.createTenant(t, "No tipping off GH RW")
	admin := s.onboard(t, a, "admin@walk.example", authz.RoleCustomerAdmin)
	officer := s.onboard(t, a, "officer@walk.example", authz.RoleComplianceOfficer)
	analyst := s.onboard(t, a, "analyst@walk.example", authz.RoleAnalyst)
	activate(t, a.id, "GH", "gh", "gh-bank", "bank", nil)
	activate(t, a.id, "RW", "rw", "rw-emi", "emi", nil)
	configure(t, a.id, "gh", "GH-SEC-07", "live", `{}`)
	configure(t, a.id, "gh", "GH-TM-16", "live", `{"float_peer_multiple": 2, "cycle_max": 5, "new_wallet_share_max": 0.5}`)
	configure(t, a.id, "gh", "GH-TM-17", "live", `{}`)
	configure(t, a.id, "rw", "RW-TM-16", "live", `{"cluster_min": 5, "float_peer_multiple": 2}`)
	configure(t, a.id, "rw", "RW-TM-20", "live", `{}`)
	rcv := newReceiver(t)
	s.createWebhook(t, a.testKey, rcv.srv.URL+"/riskos", allEvents()...)
	s.mustRule(t, a.testKey, alwaysReview())

	at := time.Now().Add(-time.Minute)
	staff := activity("walk-staff", "staff_action", "gh-bank", at.Add(-10*time.Second), map[string]any{
		"staff.action": "view_contact", "staff.role": "teller", "staff.overrides_above_peer_p95": true,
	})
	staff["actor"] = map[string]any{"user_id": "u_walk"}
	transfer := booked("walk-transfer", "transfer_outbound", "gh-bank", 100_000, "GHS", nil)
	transfer["actor"] = map[string]any{"user_id": "u_walk", "account_id": "acc_walk", "device_id": "dev_walk"}
	fired := map[string]map[string]any{
		"GH-REG-11": activity("walk-fic", "fic_order_received", "gh-bank", at, map[string]any{"order.source": "FIC", "order.type": "freeze"}),
		"GH-TM-17":  staff,
		"GH-SEC-07": transfer,
		"GH-TM-16":  activity("walk-agent-gh", "agent_daily", "gh-bank", at, map[string]any{"agent.float_topup_to_peer_p95": 10.0}),
		"RW-TM-16":  activity("walk-agent-rw", "agent_daily", "rw-emi", at, map[string]any{"agent.float_topup_to_peer_p95": 10.0}),
		"RW-TM-20": booked("walk-insider", "transfer_outbound", "rw-emi", 1_000_000, "RWF",
			map[string]any{"account.linked_to_employee": true, "account.volume_above_peer_p95": true}),
	}
	rules := []string{"GH-TM-17", "GH-SEC-07", "GH-REG-11", "GH-TM-16", "RW-TM-16", "RW-TM-20"}
	var decisions, eventIDs []string
	for _, rule := range rules {
		r := s.ingest(t, a.testKey, fired[rule])
		require.Equal(t, http.StatusOK, r.status, "%s: %s", rule, r.String())
		decisions = append(decisions, r.String())
		eventIDs = append(eventIDs, *decode[apigen.RiskDecision](t, r).EventId)
	}

	// Officers see each rule as it fired.
	var restricted string
	require.Eventually(t, func() bool {
		r := officer.b.do(t, http.MethodGet, "/api/v1/compliance/restricted-signals", nil)
		restricted = r.String()
		return r.status == http.StatusOK && !slices.ContainsFunc(rules, func(id string) bool { return !strings.Contains(restricted, `"`+id+`"`) })
	}, 20*time.Second, 200*time.Millisecond, "every compliance-only rule fired: %s", restricted)

	// What would tip someone off: each rule's ID and name, and the FIC order's reason.
	words := []string{"fic_order"}
	for _, rule := range rules {
		j := rule[:2]
		r := officer.b.do(t, http.MethodGet, "/api/v1/compliance/rules/"+rule+"?jurisdiction="+j, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		words = append(words, rule, decode[apigen.ComplianceRule](t, r).Name)
	}
	leaks := func(where, body string) {
		t.Helper()
		for _, w := range words {
			assert.NotContains(t, strings.ToLower(body), strings.ToLower(w), "%s names %q", where, w)
		}
	}
	for i, d := range decisions {
		leaks("the decision on "+rules[i], d)
	}
	for _, id := range eventIDs {
		// The stored event is the institution's own payload (its kind and facts stay); the
		// evaluation stored with it must not name the rules.
		body := s.call(t, http.MethodGet, "/api/v1/events/"+id, a.testKey, nil).String()
		for _, w := range words[1:] {
			assert.NotContains(t, body, w, "the stored event names %q", w)
		}
	}
	// Every transaction raises an alert (always_review); activity events alert on their decision.
	alerts := s.waitForAlerts(t, a.testKey, "kind=risk", 2)
	for _, al := range alerts {
		leaks("an alert", s.call(t, http.MethodGet, "/api/v1/alerts/"+al.Id, a.testKey, nil).String())
	}
	leaks("analytics", s.call(t, http.MethodGet, "/api/v1/analytics?window=24h", a.testKey, nil).String())
	leaks("the analyst's work queue", analyst.b.do(t, http.MethodGet, "/api/v1/compliance/tasks", nil).String())
	// The clock load lists the library's clocks, which name their rules as the library does; its
	// counts leave out compliance-only work (TestClocksAndReportingAcrossCountries).
	leaks("the analyst's cross-border view", analyst.b.do(t, http.MethodGet, "/api/v1/compliance/cross-border", nil).String())
	leaks("the administrator's audit log", admin.b.do(t, http.MethodGet, "/api/v1/audit-logs?limit=200", nil).String())
	require.Eventually(t, func() bool {
		rcv.mu.Lock()
		defer rcv.mu.Unlock()
		return len(rcv.all) >= len(alerts)
	}, 20*time.Second, 100*time.Millisecond, "the alerts reached the webhook endpoint")
	rcv.mu.Lock()
	for _, rec := range rcv.all {
		leaks("a webhook ("+rec.event.Type+")", string(rec.body))
	}
	rcv.mu.Unlock()
	for name, b := range map[string]*browser{"an administrator": admin.b, "an analyst": analyst.b} {
		assert.Equal(t, http.StatusForbidden, b.do(t, http.MethodGet, "/api/v1/compliance/restricted-signals", nil).status, name)
	}
}
