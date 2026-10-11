package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestQueriesNameTheirScope is the automated half of TENANCY.md layer 1: every sqlc query that
// touches a table with row-level security must name tenant_id (and environment_id, when the table
// has one) in its text, so a query that forgot its scope fails here rather than relying on RLS to
// return nothing. Queries that deliberately run across tenants are listed in crossTenant with the
// reason, and the list is checked for entries that no longer exist.
func TestQueriesNameTheirScope(t *testing.T) {
	migrations, err := filepath.Glob("../../../db/migrations/*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, migrations)

	rls := map[string]bool{}
	hasEnv := map[string]bool{}
	rlsRe := regexp.MustCompile(`(?i)ALTER TABLE\s+(\w+)\s+(?:ENABLE|FORCE)\s+ROW LEVEL SECURITY`)
	createRe := regexp.MustCompile(`(?is)CREATE TABLE\s+(?:IF NOT EXISTS\s+)?(\w+)\s*\((.*?)\n\);`)
	addColRe := regexp.MustCompile(`(?i)ALTER TABLE\s+(\w+)\s+ADD COLUMN\s+(?:IF NOT EXISTS\s+)?environment_id\b`)
	for _, m := range migrations {
		raw, err := os.ReadFile(m)
		require.NoError(t, err)
		up, _, _ := strings.Cut(string(raw), "-- +goose Down")
		for _, g := range rlsRe.FindAllStringSubmatch(up, -1) {
			rls[strings.ToLower(g[1])] = true
		}
		for _, g := range createRe.FindAllStringSubmatch(up, -1) {
			if regexp.MustCompile(`(?i)\benvironment_id\b`).MatchString(g[2]) {
				hasEnv[strings.ToLower(g[1])] = true
			}
		}
		for _, g := range addColRe.FindAllStringSubmatch(up, -1) {
			hasEnv[strings.ToLower(g[1])] = true
		}
	}
	require.NotEmpty(t, rls, "no row-level security tables found: the migration parser is broken")

	files, err := filepath.Glob("../../../db/queries/*/*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	nameRe := regexp.MustCompile(`(?m)^-- name:\s+(\w+)\s+:\w+`)
	tableRe := regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|UPDATE)\s+(\w+)`)
	commentRe := regexp.MustCompile(`(?m)--.*$`)
	tenantRe := regexp.MustCompile(`(?i)\btenant_id\b`)
	envRe := regexp.MustCompile(`(?i)\benvironment_id\b`)

	seen := map[string]bool{}
	var failures []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		text := string(raw)
		locs := nameRe.FindAllStringSubmatchIndex(text, -1)
		for i, loc := range locs {
			name := text[loc[2]:loc[3]]
			end := len(text)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			body := commentRe.ReplaceAllString(text[loc[1]:end], "")
			seen[name] = true

			var touched, needEnv []string
			for _, g := range tableRe.FindAllStringSubmatch(body, -1) {
				tbl := strings.ToLower(g[1])
				if rls[tbl] {
					touched = append(touched, tbl)
					if hasEnv[tbl] {
						needEnv = append(needEnv, tbl)
					}
				}
			}
			if len(touched) == 0 {
				continue
			}
			if _, ok := crossTenant[name]; ok {
				continue
			}
			switch {
			case !tenantRe.MatchString(body):
				failures = append(failures, filepath.Base(f)+": "+name+" touches "+strings.Join(touched, ", ")+" without tenant_id")
			case len(needEnv) > 0 && !envRe.MatchString(body):
				failures = append(failures, filepath.Base(f)+": "+name+" touches "+strings.Join(needEnv, ", ")+" without environment_id")
			}
		}
	}
	sort.Strings(failures)
	require.Empty(t, failures, "queries on row-level-security tables must name their scope; "+
		"if one is meant to run across tenants, add it to crossTenant with the reason")

	for name := range crossTenant {
		require.True(t, seen[name], "crossTenant lists %q, which is no longer a query", name)
	}
}

// crossTenant lists the queries that run outside a tenant scope on purpose, each with why.
var crossTenant = map[string]string{
	"InsertTenant": "creates the tenant row itself, before any scope exists; the tenant's id is the new row's key",
}
