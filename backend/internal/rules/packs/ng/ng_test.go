package ng_test

import (
	"bufio"
	"encoding/json"
	"flag"
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
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

var update = flag.Bool("update", false, "regenerate text.json from the library document")

// library is the source of truth the pack is checked against.
const library = "../../../../../docs/compliance/NIGERIA_RULES_LIBRARY.md"

var (
	rowID    = regexp.MustCompile(`^\| ((?:REG|ATO|VEL|MUL|BEH|CNP|LEN|SEC|INS|SIM|CRY|PON|GOV)-[0-9]{2}) (.+?) \| (.+) \|$`)
	ruleLine = regexp.MustCompile(`^RULE ((?:REG|ATO|VEL|MUL|BEH|CNP|LEN|SEC|INS|SIM|CRY|PON|GOV)-[0-9]{2}) \|`)
)

// parseLibrary reads section 7 (plain English) and section 7B (engineering logic).
func parseLibrary(t *testing.T) map[string]pack.Text {
	t.Helper()
	f, err := os.Open(library)
	require.NoError(t, err)
	defer f.Close()
	out := map[string]pack.Text{}
	section := ""
	var current string
	var block []string
	flush := func() {
		if current != "" {
			txt := out[current]
			txt.EngineeringLogic = strings.TrimRight(strings.Join(block, "\n"), "\n ")
			out[current] = txt
		}
		current, block = "", nil
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "### 7. "):
			section = "7"
		case strings.HasPrefix(line, "### 7B."):
			section = "7B"
		case strings.HasPrefix(line, "### 8."):
			flush()
			section = ""
		}
		switch section {
		case "7":
			m := rowID.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			id, name, rest := m[1], m[2], strings.Split(m[3], " | ")
			txt := pack.Text{Name: name, Description: rest[0]}
			switch {
			case strings.HasPrefix(id, "REG-"):
				require.Len(t, rest, 2, id)
				txt.Action = rest[1]
			case strings.HasPrefix(id, "GOV-"):
				require.Len(t, rest, 1, id)
			default:
				require.Len(t, rest, 2, id)
				txt.Points = rest[1]
			}
			require.NotContains(t, out, id, "%s appears twice in section 7", id)
			out[id] = txt
		case "7B":
			if m := ruleLine.FindStringSubmatch(line); m != nil {
				flush()
				current = m[1]
			}
			if strings.HasPrefix(line, "```") {
				flush()
				continue
			}
			if current != "" {
				block = append(block, line)
			}
		}
	}
	require.NoError(t, sc.Err())
	return out
}

func TestTextMatchesLibrary(t *testing.T) {
	lib := parseLibrary(t)
	require.Len(t, lib, 83, "19 controls, 52 risk-scoring rules and 12 governance presets")
	for id, txt := range lib {
		require.NotEmpty(t, txt.EngineeringLogic, "%s has no section 7B logic", id)
	}
	path := filepath.Join(".", pack.TextFile)
	want, err := json.MarshalIndent(lib, "", "  ")
	require.NoError(t, err)
	want = append(want, '\n')
	if *update {
		require.NoError(t, os.WriteFile(path, want, 0o600))
	}
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got),
		"text.json differs from the library; run go test ./internal/rules/packs/ng -update")
}

func TestPackLoadsAndValidates(t *testing.T) {
	p, err := pack.Load(ng.Files)
	require.NoError(t, err)
	assert.Equal(t, "ng", p.ID)
	assert.NotEmpty(t, p.Checksum)
}

// The seed and the library list exactly the same rules, in both directions.
func TestSeedMatchesLibraryRules(t *testing.T) {
	p, err := pack.Load(ng.Files)
	require.NoError(t, err)
	lib := parseLibrary(t)
	seed := map[string]pack.Rule{}
	for _, r := range p.Rules {
		seed[r.ID] = r
	}
	libIDs := slices.Sorted(maps.Keys(lib))
	seedIDs := slices.Sorted(maps.Keys(seed))
	for _, id := range libIDs {
		assert.Contains(t, seedIDs, id, "%s is in the library but not in the seed", id)
	}
	for _, id := range seedIDs {
		assert.Contains(t, libIDs, id, "%s is in the seed but not in the library", id)
	}
	counts := map[pack.Kind]int{}
	for _, r := range p.Rules {
		counts[r.Kind]++
	}
	assert.Equal(t, map[pack.Kind]int{pack.KindControl: 19, pack.KindSignal: 52, pack.KindGovernance: 12}, counts)
}

// Every risk-scoring rule's default points equal the library's Points column.
func TestWeightsMatchLibrary(t *testing.T) {
	p, err := pack.Load(ng.Files)
	require.NoError(t, err)
	for _, r := range p.Rules {
		if r.Kind != pack.KindSignal {
			continue
		}
		var got []string
		for _, prm := range r.Parameters {
			if prm.Type == pack.ParamPoints {
				got = append(got, strconv.FormatFloat(*prm.Default, 'f', -1, 64))
			}
		}
		assert.Equal(t, strings.Join(got, " or "), r.Text.Points, "%s: points differ from the library", r.ID)
	}
}

// What the library says about rules that must not run by default.
func TestDefaultsFollowTheLibrary(t *testing.T) {
	p, err := pack.Load(ng.Files)
	require.NoError(t, err)
	byID := map[string]pack.Rule{}
	for _, r := range p.Rules {
		byID[r.ID] = r
	}
	assert.False(t, byID["CRY-04"].Defaults.Enabled, "the official SEC threshold is awaited")
	assert.False(t, byID["MUL-04"].Defaults.Enabled, "KYC tier norms are set by the institution")
	for _, id := range []string{"REG-13", "REG-14", "REG-17", "GOV-08"} {
		assert.Equal(t, pack.VerificationConflicting, byID[id].Verification, id)
		assert.NotEmpty(t, byID[id].Caveat, id)
	}
	assert.Contains(t, byID["REG-01"].Caveat, "summaries of the CBN circular")
	for _, r := range p.Rules {
		switch r.Kind {
		case pack.KindControl, pack.KindGovernance:
			assert.Equal(t, pack.ModeLive, r.Defaults.Mode, "%s: controls run live by default (ADR-0006)", r.ID)
		case pack.KindSignal:
			assert.Equal(t, pack.ModeShadow, r.Defaults.Mode, "%s: recommended rules start in shadow for backtesting", r.ID)
		}
	}
}
