// Package librarytest reads the Ghana and Rwanda rules library
// (docs/compliance/GHANA_RWANDA_RULES_LIBRARY.md, ADR-0007) so a pack's wording is copied from
// it, never retyped, and its tests can hold the seed to the library in both directions.
package librarytest

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

// Path is the library document, relative to a pack directory.
const Path = "../../../../../docs/compliance/GHANA_RWANDA_RULES_LIBRARY.md"

// Section names the headings that open a country's plain-English tables and its logic, and
// the heading that ends them.
type Section struct {
	Country     string // GH or RW
	Tables      string // "### 4.1"
	Logic       string // "### 4.2"
	End         string // "## 5."
	RulePattern *regexp.Regexp
}

// For returns the sections of a country's rules.
func For(country string) Section {
	s := Section{Country: country}
	switch country {
	case "GH":
		s.Tables, s.Logic, s.End = "### 4.1", "### 4.2", "## 5."
	case "RW":
		s.Tables, s.Logic, s.End = "### 5.1", "### 5.2", "## 6."
	}
	return s
}

var points = regexp.MustCompile(`^[0-9]+( or [0-9]+| to [0-9]+)? points`)

// Parse reads a country's rules: the table row (name, description, action or points, basis)
// and the logic block of each.
func Parse(r io.Reader, s Section) (map[string]pack.Text, error) {
	row := regexp.MustCompile(`^\| (` + s.Country + `-(?:REG|TM|SEC|GOV)-[0-9]{2}) \| (.+?) \| (.+) \|$`)
	ruleLine := regexp.MustCompile(`^RULE (` + s.Country + `-(?:REG|TM|SEC|GOV)-[0-9]{2}) \|`)
	out := map[string]pack.Text{}
	section := ""
	var current string
	var block []string
	flush := func() {
		if current != "" {
			t := out[current]
			t.EngineeringLogic = strings.TrimRight(strings.Join(block, "\n"), "\n ")
			out[current] = t
		}
		current, block = "", nil
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, s.Tables):
			section = "tables"
		case strings.HasPrefix(line, s.Logic):
			section = "logic"
		case strings.HasPrefix(line, s.End):
			flush()
			section = ""
		}
		switch section {
		case "tables":
			m := row.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			cols := strings.Split(m[3], " | ")
			if len(cols) != 3 {
				return nil, fmt.Errorf("%s: want 5 columns, got %d", m[1], len(cols)+2)
			}
			if _, dup := out[m[1]]; dup {
				return nil, fmt.Errorf("%s appears twice", m[1])
			}
			t := pack.Text{Name: m[2], Description: cols[0], Basis: cols[2]}
			if points.MatchString(cols[1]) {
				t.Points = cols[1]
			} else {
				t.Action = cols[1]
			}
			out[m[1]] = t
		case "logic":
			if m := ruleLine.FindStringSubmatch(line); m != nil {
				flush()
				current = m[1]
			}
			if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "#### ") {
				flush()
				continue
			}
			if current != "" {
				block = append(block, line)
			}
		}
	}
	return out, sc.Err()
}

// Tags are what a rule's basis column says about it.
type Tags struct {
	Provenance pack.Provenance
	// Lineage is param, analogue, cross_reference, stricter or new_local; Of the rule IDs the
	// lineage clause names.
	Lineage string
	Of      []string
	// Enabled and OffReason are the default state.
	Enabled   bool
	OffReason pack.OffReason
}

var (
	tagPattern     = regexp.MustCompile(`^\[(Regulatory|Industry practice|My design)\]\. `)
	defaultPattern = regexp.MustCompile(`Default: (On|Off \(threshold to be confirmed\)|Off \(verify first\))\.`)
	ruleIDs        = regexp.MustCompile(`\b(?:REG|ATO|VEL|MUL|BEH|CNP|LEN|SEC|INS|SIM|CRY|PON|GOV)-[0-9]{2}\b`)
	lineageKinds   = []struct{ prefix, kind string }{ //nolint:gochecknoglobals // fixed vocabulary
		{"Param of", "param"}, {"Analogue of", "analogue"}, {"Cross-reference", "cross_reference"},
		{"Stricter than", "stricter"}, {"New local", "new_local"},
	}
)

// ParseTags reads the "Tag, lineage, default, basis" column.
func ParseTags(basis string) (Tags, error) {
	var t Tags
	m := tagPattern.FindStringSubmatch(basis)
	if m == nil {
		return t, fmt.Errorf("no provenance tag in %q", basis)
	}
	t.Provenance = map[string]pack.Provenance{
		"Regulatory": pack.ProvenanceRegulatory, "Industry practice": pack.ProvenanceIndustry, "My design": pack.ProvenanceDesign,
	}[m[1]]
	rest := basis[len(m[0]):]
	lineage, _, _ := strings.Cut(rest, ". Default:")
	for _, k := range lineageKinds {
		if strings.HasPrefix(lineage, k.prefix) {
			t.Lineage = k.kind
			break
		}
	}
	if t.Lineage == "" {
		return t, fmt.Errorf("no lineage in %q", basis)
	}
	// "Param of ATO-01 (no Ghana equivalent of REG-01 found)": the related rules are named before
	// the note. A new local rule has none, so the rules its note names ("cross-reference CRY-01")
	// are the related ones.
	head, note, _ := strings.Cut(lineage, "(")
	if t.Lineage == "new_local" {
		head = note
	}
	t.Of = ruleIDs.FindAllString(head, -1)
	d := defaultPattern.FindStringSubmatch(basis)
	if d == nil {
		return t, fmt.Errorf("no default in %q", basis)
	}
	switch d[1] {
	case "On":
		t.Enabled = true
	case "Off (threshold to be confirmed)":
		t.OffReason = pack.OffThresholdToConfirm
	default:
		t.OffReason = pack.OffVerifyFirst
	}
	return t, nil
}

// inheritedPattern finds the Nigerian rules section 1 says apply without a new ID.
var inheritedPattern = regexp.MustCompile(`\*\*Inherited from Nigeria without a new ID\.\*\* (.+)`)

// Inherited returns the rules section 1 lists as inherited, by country: BEH-01 is Ghana only.
func Inherited(r io.Reader) (map[string][]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		m := inheritedPattern.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		text, _, _ := strings.Cut(m[1], " For Rwanda,")
		var all []string
		// "CNP-01 to CNP-03" and "GOV-01 to GOV-04" are ranges.
		rng := regexp.MustCompile(`([A-Z]{3})-([0-9]{2}) to ([A-Z]{3})-([0-9]{2})`)
		text = rng.ReplaceAllStringFunc(text, func(s string) string {
			p := rng.FindStringSubmatch(s)
			from, _ := strconv.Atoi(p[2]) // two digits, matched above
			to, _ := strconv.Atoi(p[4])
			var ids []string
			for i := from; i <= to; i++ {
				ids = append(ids, fmt.Sprintf("%s-%02d", p[1], i))
			}
			return strings.Join(ids, ", ")
		})
		all = ruleIDs.FindAllString(text, -1)
		out := map[string][]string{}
		for _, id := range all {
			out["GH"] = append(out["GH"], id)
			if id != "BEH-01" {
				out["RW"] = append(out["RW"], id)
			}
		}
		return out, nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("the inherited-rules paragraph is missing")
}
