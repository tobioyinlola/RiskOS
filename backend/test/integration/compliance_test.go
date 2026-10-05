//go:build integration

package integration

import (
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

// The rules library serves the Nigeria pack loaded by the migrate step: 19 controls, 52
// risk-scoring rules and 12 governance presets, with engineering logic only for engineers.
func TestComplianceLibrary(t *testing.T) {
	s := newStack(t, 0)
	a := s.createTenant(t, "Library")
	// The library without a profile: applicability is unknown.
	withoutCompliance(t, a.id)

	r := s.call(t, http.MethodGet, "/api/v1/compliance/pack", a.testKey, nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	p := decode[apigen.CompliancePack](t, r)
	assert.Equal(t, "ng", p.Id)
	assert.Equal(t, "NG", p.Jurisdiction)
	assert.Len(t, p.Checksum, 64)
	assert.Equal(t, apigen.ComplianceCounts{Controls: 19, Signals: 52, Governance: 12}, p.Counts)
	total := 0
	for _, g := range p.Groups {
		total += g.Rules
	}
	assert.Equal(t, 83, total, "every rule is in a listed group")

	r = s.call(t, http.MethodGet, "/api/v1/compliance/rules", a.testKey, nil)
	require.Equal(t, http.StatusOK, r.status, r.String())
	list := decode[apigen.ComplianceRuleList](t, r).Data
	require.Len(t, list, 83)
	for _, rule := range list {
		assert.Nil(t, rule.EngineeringLogic, "%s: lists never carry engineering logic", rule.Id)
		assert.Nil(t, rule.Definition, rule.Id)
		assert.Nil(t, rule.Applicable, "%s: no profile, so applicability is unknown", rule.Id)
		assert.NotEmpty(t, rule.Sources, "%s traces to a source", rule.Id)
		assert.NotEmpty(t, rule.Effective.Text, "%s has an effective date", rule.Id)
		unconfirmedLaw := rule.Kind == "control" && rule.Verification != "confirmed"
		if unconfirmedLaw || rule.Verification == "conflicting" || rule.Verification == "awaiting_regulator" {
			assert.NotNil(t, rule.Caveat, "%s: unconfirmed or conflicting facts carry a caveat", rule.Id)
		}
	}

	t.Run("filters", func(t *testing.T) {
		for query, want := range map[string]int{"kind=control": 19, "kind=signal": 52, "kind=governance": 12, "q=vel-01": 1} {
			r := s.call(t, http.MethodGet, "/api/v1/compliance/rules?"+query, a.testKey, nil)
			require.Equal(t, http.StatusOK, r.status, r.String())
			assert.Len(t, decode[apigen.ComplianceRuleList](t, r).Data, want, query)
		}
		conflicting := decode[apigen.ComplianceRuleList](t, s.call(t, http.MethodGet, "/api/v1/compliance/rules?verification=conflicting", a.testKey, nil)).Data
		var ids []string
		for _, rule := range conflicting {
			ids = append(ids, rule.Id)
		}
		assert.Subset(t, ids, []string{"REG-13", "REG-14", "REG-17", "GOV-08"}, "conflicts stay visible")
		r := s.call(t, http.MethodGet, "/api/v1/compliance/rules?applicable=true", a.testKey, nil)
		assert.Equal(t, http.StatusBadRequest, r.status, "applicability needs a profile: %s", r.String())
		r = s.call(t, http.MethodGet, "/api/v1/compliance/rules?kind=nope", a.testKey, nil)
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	})

	t.Run("wording is the library's, verbatim", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/compliance/rules/REG-01", a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		rule := decode[apigen.ComplianceRule](t, r)
		assert.Equal(t, apigen.ComplianceRuleKind("control"), rule.Kind)
		require.NotNil(t, rule.Action)
		require.NotNil(t, rule.Caveat)
		assert.Contains(t, *rule.Caveat, "summaries of the CBN circular")
		require.NotNil(t, rule.EngineeringLogic, "the bootstrap key holds compliance:engineering")
		assert.True(t, strings.HasPrefix(*rule.EngineeringLogic, "RULE REG-01 |"), *rule.EngineeringLogic)
		require.NotNil(t, rule.Definition)
		assert.Equal(t, "REG-01", (*rule.Definition)["id"])
		for _, prm := range rule.Parameters {
			assert.LessOrEqual(t, prm.Min, prm.Max, prm.Name)
		}
		r = s.call(t, http.MethodGet, "/api/v1/compliance/rules/ZZZ-99", a.testKey, nil)
		assert.Equal(t, http.StatusNotFound, r.status, r.String())
	})

	t.Run("engineering logic needs compliance:engineering", func(t *testing.T) {
		_, reader := s.createKey(t, a.testKey, "reader", authz.ComplianceRead)
		r := s.call(t, http.MethodGet, "/api/v1/compliance/rules/VEL-01", reader, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		rule := decode[apigen.ComplianceRule](t, r)
		assert.Nil(t, rule.EngineeringLogic)
		assert.Nil(t, rule.Definition)
		assert.NotEmpty(t, rule.Description)

		_, none := s.createKey(t, a.testKey, "events", authz.EventsRead)
		for _, path := range []string{"pack", "rules", "rules/VEL-01", "clocks", "gaps", "glossary", "profile"} {
			r := s.call(t, http.MethodGet, "/api/v1/compliance/"+path, none, nil)
			assert.Equal(t, http.StatusForbidden, r.status, "%s: %s", path, r.String())
		}
	})

	t.Run("clocks, gaps and glossary", func(t *testing.T) {
		clocks := decode[apigen.ComplianceClockList](t, s.call(t, http.MethodGet, "/api/v1/compliance/clocks", a.testKey, nil)).Data
		assert.NotEmpty(t, clocks)
		for _, c := range clocks {
			require.NotNil(t, c.Hours, c.Id)
			assert.Positive(t, *c.Hours, c.Id)
			assert.Equal(t, apigen.ComplianceClockKind("legal"), c.Kind, c.Id)
		}
		assert.NotEmpty(t, decode[apigen.ComplianceGapList](t, s.call(t, http.MethodGet, "/api/v1/compliance/gaps", a.testKey, nil)).Data)
		assert.NotEmpty(t, decode[apigen.ComplianceTermList](t, s.call(t, http.MethodGet, "/api/v1/compliance/glossary", a.testKey, nil)).Data)
	})
}

// The compliance profile decides which rules apply. Only people set it, it is audited, and it
// is the tenant's own: another tenant never sees it.
func TestComplianceProfile(t *testing.T) {
	s := newStackWith(t, 0, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Profile A")
	b := s.createTenant(t, "Profile B")
	profile := map[string]any{"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{"instant_payments"}}

	r := s.call(t, http.MethodGet, "/api/v1/compliance/profile", a.testKey, nil)
	require.Equal(t, http.StatusOK, r.status, "every workspace has a profile from the start: %s", r.String())
	auto := decode[apigen.ComplianceProfile](t, r)
	assert.False(t, auto.Confirmed, "nobody has confirmed the institution type yet")
	assert.Equal(t, "other", auto.LicenceType)
	assert.Equal(t, "system:default", auto.UpdatedBy)
	for id, want := range map[string]apigen.PackRuleMode{"REG-01": "live", "REG-16": "live", "ATO-01": "off", "VEL-01": "off"} {
		got := decode[apigen.ComplianceRule](t, s.call(t, http.MethodGet, "/api/v1/compliance/rules/"+id, a.testKey, nil))
		assert.Equal(t, want, got.Mode, "%s: legal controls start on, risk signals off until activated", id)
	}
	r = s.call(t, http.MethodPut, "/api/v1/compliance/profile", a.testKey, profile)
	assert.Equal(t, http.StatusForbidden, r.status, "an API key cannot set the profile: %s", r.String())

	admin := s.onboard(t, a, "admin@profile-a.example", authz.RoleCustomerAdmin)
	r = admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile", profile)
	require.Equal(t, http.StatusOK, r.status, r.String())
	saved := decode[apigen.ComplianceProfile](t, r)
	assert.Equal(t, "NG", saved.Jurisdiction)
	assert.Equal(t, "ng", saved.PackId)
	assert.Equal(t, []string{"instant_payments"}, saved.Capabilities)
	assert.True(t, strings.HasPrefix(saved.UpdatedBy, "usr_"), saved.UpdatedBy)

	assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs?limit=100", a.testKey, nil)),
		"compliance_profile.updated")

	t.Run("applicability follows the profile", func(t *testing.T) {
		rules := decode[apigen.ComplianceRuleList](t, s.call(t, http.MethodGet, "/api/v1/compliance/rules", a.testKey, nil)).Data
		byID := map[string]apigen.ComplianceRule{}
		for _, rule := range rules {
			require.NotNil(t, rule.Applicable, rule.Id)
			byID[rule.Id] = rule
		}
		assert.True(t, *byID["REG-01"].Applicable, "banks with instant payments get the NIP limits")
		assert.False(t, *byID["REG-08"].Applicable, "the cash fee needs the cash_fee capability")
		assert.False(t, *byID["CRY-01"].Applicable, "crypto rules are for VASPs")
		assert.True(t, *byID["VEL-01"].Applicable, "risk-scoring rules apply to everyone")

		applicable := decode[apigen.ComplianceRuleList](t, s.call(t, http.MethodGet, "/api/v1/compliance/rules?applicable=true", a.testKey, nil)).Data
		assert.Len(t, applicable, 77)
	})

	t.Run("invalid profiles are refused", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"unknown licence":    {"jurisdiction": "NG", "licence_type": "casino", "capabilities": []string{}},
			"unknown capability": {"jurisdiction": "NG", "licence_type": "bank", "capabilities": []string{"teleport"}},
			"no pack":            {"jurisdiction": "KE", "licence_type": "bank", "capabilities": []string{}},
			"bad jurisdiction":   {"jurisdiction": "Nigeria", "licence_type": "bank", "capabilities": []string{}},
		} {
			r := admin.b.do(t, http.MethodPut, "/api/v1/compliance/profile", body)
			assert.Equal(t, http.StatusBadRequest, r.status, "%s: %s", name, r.String())
		}
	})

	t.Run("another tenant never sees it", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/compliance/profile", b.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		own := decode[apigen.ComplianceProfile](t, r)
		assert.Equal(t, "other", own.LicenceType, "B sees its own automatic profile, not A's")
		assert.False(t, own.Confirmed)
	})

	t.Run("roles", func(t *testing.T) {
		officer := s.onboard(t, a, "officer@profile-a.example", authz.RoleComplianceOfficer)
		r := officer.b.do(t, http.MethodPut, "/api/v1/compliance/profile",
			map[string]any{"jurisdiction": "NG", "licence_type": "mfb", "capabilities": []string{"instant_payments", "cash_fee"}})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, []string{"cash_fee", "instant_payments"}, decode[apigen.ComplianceProfile](t, r).Capabilities)
		r = officer.b.do(t, http.MethodGet, "/api/v1/compliance/rules/REG-01", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Nil(t, decode[apigen.ComplianceRule](t, r).EngineeringLogic, "compliance officers read the plain wording")

		auditor := s.onboard(t, a, "auditor@profile-a.example", authz.RoleAuditor)
		r = auditor.b.do(t, http.MethodGet, "/api/v1/compliance/rules", nil)
		assert.Equal(t, http.StatusOK, r.status, r.String())
		r = auditor.b.do(t, http.MethodPut, "/api/v1/compliance/profile", profile)
		assert.Equal(t, http.StatusForbidden, r.status, "auditors only read: %s", r.String())

		dev := s.onboard(t, a, "dev@profile-a.example", authz.RoleDeveloper)
		r = dev.b.do(t, http.MethodGet, "/api/v1/compliance/rules/REG-01", nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.NotNil(t, decode[apigen.ComplianceRule](t, r).EngineeringLogic, "engineers see the logic")
	})
}

// Loading is idempotent, a loaded version never changes, and the application cannot write packs.
func TestRulePackLoading(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, rules.LoadPacks(ctx, shared.owner, rules.BuiltInPacks(), shared.logger), "loading again is a no-op")

	changed := fstest.MapFS{}
	require.NoError(t, fs.WalkDir(ng.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(ng.Files, path)
		changed[path] = &fstest.MapFile{Data: raw}
		return err
	}))
	changed["vel.json"].Data = []byte(strings.Replace(string(changed["vel.json"].Data), `"severity": "medium"`, `"severity": "high"`, 1))
	err := rules.LoadPacks(ctx, shared.owner, []fs.FS{changed}, shared.logger)
	require.ErrorIs(t, err, rules.ErrChecksumChanged, "the same version with different content is refused")

	_, err = shared.owner.Exec(ctx, "UPDATE pack_rules SET wording = '{}' WHERE rule_id = 'REG-01'")
	require.ErrorContains(t, err, "cannot change", "even the owner cannot rewrite a loaded version")
	_, err = shared.pool.Exec(ctx, "INSERT INTO rule_packs (id, version, jurisdiction, name, checksum, meta) VALUES ('xx', '2026.1.0', 'XX', 'x', repeat('0', 64), '{}')")
	require.ErrorContains(t, err, "permission denied", "the application only reads packs")
}
