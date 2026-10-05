//go:build integration

package integration

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
)

// The library reads per country (ADR-0007): clocks that may be unknown, open questions with how
// they are treated, glossaries, the side-by-side matrix and an audited export.
func TestLibraryByCountry(t *testing.T) {
	s := newStackWith(t, time.Second, generousLimits(), shared.rdb, withConsole(auth.ConsoleConfig{}, false))
	a := s.createTenant(t, "Library by country")
	get := func(path string) response {
		t.Helper()
		r := s.call(t, http.MethodGet, path, a.testKey, nil)
		require.Equal(t, http.StatusOK, r.status, r.String())
		return r
	}

	t.Run("a clock that was not found has no hours", func(t *testing.T) {
		clocks := decode[apigen.ComplianceClockList](t, get("/api/v1/compliance/clocks?jurisdiction=RW")).Data
		byID := map[string]apigen.ComplianceClock{}
		for _, c := range clocks {
			byID[c.Id] = c
		}
		assert.Nil(t, byID["bnr_incident"].Hours, "never shown as zero")
		assert.Equal(t, apigen.ComplianceClockKind("not_found"), byID["bnr_incident"].Kind)
		assert.Equal(t, apigen.ComplianceClockKind("design_placeholder"), byID["str_filing"].Kind, "Rwanda's STR deadline is a placeholder")
		require.NotNil(t, byID["rura_critical"].Hours)
		assert.Zero(t, *byID["rura_critical"].Hours, "immediately")
		assert.Equal(t, apigen.ComplianceClockKind("legal"), byID["ncsa_breach"].Kind)
	})
	t.Run("open questions say how they are treated", func(t *testing.T) {
		gaps := decode[apigen.ComplianceGapList](t, get("/api/v1/compliance/gaps?jurisdiction=GH")).Data
		i := slices.IndexFunc(gaps, func(g apigen.ComplianceGap) bool { return g.Id == "C4" })
		require.GreaterOrEqual(t, i, 0)
		assert.Equal(t, apigen.ComplianceGapKind("conflict"), *gaps[i].Kind)
		assert.Contains(t, *gaps[i].Treatment, "Official figures used")
		assert.Contains(t, gaps[i].Rules, "GH-REG-04")
	})
	t.Run("each country has its glossary and penalties", func(t *testing.T) {
		terms := decode[apigen.ComplianceTermList](t, get("/api/v1/compliance/glossary?jurisdiction=GH")).Data
		assert.True(t, slices.ContainsFunc(terms, func(x apigen.ComplianceTerm) bool { return x.Term == "Ghana Card" }))
		pk := decode[apigen.CompliancePack](t, get("/api/v1/compliance/pack?jurisdiction=GH"))
		assert.Equal(t, "GHS", *pk.Currency)
		assert.Equal(t, "Africa/Accra", *pk.Timezone)
		require.NotEmpty(t, *pk.Penalties)
		assert.True(t, (*pk.Penalties)[0].Inference, "cedi estimates at GHS 12 a unit are an inference")
	})
	t.Run("the matrix is the library's section 6", func(t *testing.T) {
		rows := decode[struct {
			Data []apigen.ClockMatrixRow `json:"data"`
		}](t, get("/api/v1/compliance/clock-matrix")).Data
		require.Len(t, rows, 14)
		assert.Equal(t, "STR clock", rows[0].Item)
		assert.Contains(t, rows[0].Rwanda, "numeric deadline not found")
	})
	t.Run("a country's library exports as CSV and PDF, audited", func(t *testing.T) {
		r := get("/api/v1/compliance/export?jurisdiction=RW&format=csv")
		assert.Contains(t, r.header.Get("Content-Disposition"), "riskos-rules-rw-")
		rows, err := csv.NewReader(bytes.NewReader(r.raw)).ReadAll()
		require.NoError(t, err)
		assert.Equal(t, "Status in test", rows[0][9])
		assert.Len(t, rows, 1+64+24, "a header, Rwanda's 64 rules and the 24 it inherits")
		i := slices.IndexFunc(rows, func(row []string) bool { return row[1] == "RW-REG-04" })
		require.GreaterOrEqual(t, i, 0)
		assert.Contains(t, rows[i][13], "design placeholder", "the caveat travels with the rule")
		assert.Contains(t, rows[i][11], "1 day", "settings are in words")
		j := slices.IndexFunc(rows, func(row []string) bool { return row[1] == "RW-REG-07" })
		assert.Contains(t, rows[j][11], "RWF 10,000,000")

		r = get("/api/v1/compliance/export?format=pdf")
		assert.Equal(t, "application/pdf", r.header.Get("Content-Type"))
		assert.True(t, bytes.HasPrefix(r.raw, []byte("%PDF-")))
		assert.True(t, strings.Contains(r.header.Get("Content-Disposition"), "all-countries"))

		var n int
		require.NoError(t, shared.owner.QueryRow(t.Context(),
			"SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'compliance_library.exported'", a.id).Scan(&n))
		assert.Equal(t, 2, n)
	})
	t.Run("an unknown format is refused", func(t *testing.T) {
		r := s.call(t, http.MethodGet, "/api/v1/compliance/export?format=docx", a.testKey, nil)
		assert.Equal(t, http.StatusBadRequest, r.status, r.String())
	})
}
