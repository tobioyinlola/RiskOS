package librarytest

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

// Expect is what the library says about one country's pack (section 1).
type Expect struct {
	Country string
	// Families counts rules by family: REG, TM, SEC, GOV.
	Families map[string]int
	// Provenance counts rules by tag.
	Provenance map[pack.Provenance]int
	// Off lists the rules off by default.
	Off []string
}

func read(t *testing.T, s Section) map[string]pack.Text {
	t.Helper()
	f, err := os.Open(Path)
	require.NoError(t, err)
	defer f.Close()
	lib, err := Parse(f, s)
	require.NoError(t, err)
	return lib
}

// Check holds a country pack to the library: wording, rule IDs in both directions, inherited
// rules, tags, weights and templates. With update it rewrites text.json from the library.
func Check(t *testing.T, dir string, files, parent fs.FS, e Expect, update bool) {
	t.Helper()
	lib := read(t, For(e.Country))
	t.Run("text matches the library", func(t *testing.T) {
		want, err := json.MarshalIndent(lib, "", "  ")
		require.NoError(t, err)
		want = append(want, '\n')
		path := filepath.Join(dir, pack.TextFile)
		if update {
			require.NoError(t, os.WriteFile(path, want, 0o600))
		}
		got, err := os.ReadFile(path) //nolint:gosec // the pack's own text.json, named by its test
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "text.json differs from the library; run go test with -update")
	})
	ng, err := pack.Load(parent)
	require.NoError(t, err)
	p, err := pack.Load(files, ng)
	require.NoError(t, err)
	own := map[string]pack.Rule{}
	inherited := map[string]pack.Rule{}
	for _, r := range p.Rules {
		if r.InheritedFrom != "" {
			inherited[r.ID] = r
		} else {
			own[r.ID] = r
		}
	}
	t.Run("seed and library list the same rules", func(t *testing.T) {
		libIDs, seedIDs := slices.Sorted(maps.Keys(lib)), slices.Sorted(maps.Keys(own))
		for _, id := range libIDs {
			assert.Contains(t, seedIDs, id, "%s is in the library but not in the seed", id)
		}
		for _, id := range seedIDs {
			assert.Contains(t, libIDs, id, "%s is in the seed but not in the library", id)
		}
		families := map[string]int{}
		total := 0
		for id := range own {
			families[strings.Split(id, "-")[1]]++
			total++
		}
		assert.Equal(t, e.Families, families)
		want := 0
		for _, n := range e.Families {
			want += n
		}
		assert.Equal(t, want, total)
	})
	t.Run("inherited rules are the ones section 1 lists", func(t *testing.T) {
		f, err := os.Open(Path)
		require.NoError(t, err)
		defer f.Close()
		list, err := Inherited(f)
		require.NoError(t, err)
		assert.ElementsMatch(t, list[e.Country], slices.Collect(maps.Keys(inherited)))
		for id, r := range inherited {
			assert.Equal(t, "ng", r.InheritedFrom, id)
			for _, prm := range r.Parameters {
				if prm.Type == pack.ParamMoney {
					assert.NotEqual(t, "NGN", prm.Currency, "%s: %s keeps a naira amount", id, prm.Name)
				}
			}
		}
	})
	t.Run("tags follow the library", func(t *testing.T) {
		prov := map[pack.Provenance]int{}
		var off []string
		for id, r := range own {
			tags, err := ParseTags(r.Text.Basis)
			require.NoError(t, err, id)
			assert.Equal(t, tags.Provenance, r.Provenance, "%s: provenance", id)
			require.NotNil(t, r.Lineage, id)
			assert.Equal(t, tags.Lineage, r.Lineage.Kind, "%s: lineage", id)
			assert.ElementsMatch(t, tags.Of, r.Lineage.Of, "%s: lineage names", id)
			assert.Equal(t, tags.Enabled, r.Defaults.Enabled, "%s: default", id)
			assert.Equal(t, tags.OffReason, r.OffReason, "%s: off reason", id)
			prov[r.Provenance]++
			if !r.Defaults.Enabled {
				off = append(off, id)
			}
			switch r.Kind {
			case pack.KindSignal:
				assert.Equal(t, pack.ModeShadow, r.Defaults.Mode, "%s: risk signals start in shadow for backtesting", id)
			default:
				assert.Equal(t, pack.ModeLive, r.Defaults.Mode, "%s: controls and presets run live (ADR-0006)", id)
			}
		}
		assert.Equal(t, e.Provenance, prov)
		assert.ElementsMatch(t, e.Off, off)
	})
	t.Run("weights match the library's points", func(t *testing.T) {
		number := regexp.MustCompile(`[0-9]+`)
		for id, r := range own {
			if r.Kind != pack.KindSignal {
				assert.Empty(t, r.Text.Points, "%s: only risk signals have points", id)
				continue
			}
			lead, _, _ := strings.Cut(r.Text.Points, " points")
			var want []float64
			for _, n := range number.FindAllString(lead, -1) {
				v, err := strconv.ParseFloat(n, 64)
				require.NoError(t, err)
				want = append(want, v)
			}
			var got []float64
			for _, prm := range r.Parameters {
				if prm.Type == pack.ParamPoints {
					got = append(got, *prm.Default)
				}
			}
			slices.Sort(got)
			assert.Equal(t, want, got, "%s: points differ from the library (%q)", id, r.Text.Points)
		}
	})
	t.Run("a parameterised rule shares its parent's template", func(t *testing.T) {
		n := 0
		for id, r := range own {
			if r.Template == nil {
				continue
			}
			n++
			i := slices.IndexFunc(ng.Rules, func(x pack.Rule) bool { return x.ID == r.Template.Rule })
			require.GreaterOrEqual(t, i, 0, id)
			parent := ng.Rules[i]
			for name, pair := range map[string][2]any{
				"trigger": {r.Trigger, parent.Trigger}, "condition": {r.Condition, parent.Condition},
				"branches": {r.Branches, parent.Branches}, "inputs": {r.Inputs, parent.Inputs},
				"governance": {r.Governance, parent.Governance},
			} {
				a, err := json.Marshal(pair[0])
				require.NoError(t, err)
				b, err := json.Marshal(pair[1])
				require.NoError(t, err)
				assert.JSONEq(t, string(b), string(a), "%s: %s differs from %s's", id, name, parent.ID)
			}
			for _, prm := range r.Parameters {
				if _, overridden := r.Overrides[prm.Name]; !overridden {
					j := slices.IndexFunc(parent.Parameters, func(x pack.Parameter) bool { return x.Name == prm.Name })
					require.GreaterOrEqual(t, j, 0)
					assert.Equal(t, parent.Parameters[j], prm, "%s: %s changed without an override", id, prm.Name)
				}
			}
		}
		assert.Positive(t, n, "the pack has parameterised rules")
	})
}
