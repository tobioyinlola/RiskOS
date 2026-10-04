package gh_test

import (
	"flag"
	"testing"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/gh"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/librarytest"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

var update = flag.Bool("update", false, "regenerate text.json from the library document")

func TestPackFollowsTheLibrary(t *testing.T) {
	librarytest.Check(t, ".", gh.Files, ng.Files, librarytest.Expect{
		Country:  "GH",
		Families: map[string]int{"REG": 13, "TM": 28, "SEC": 14, "GOV": 13},
		Provenance: map[pack.Provenance]int{
			pack.ProvenanceRegulatory: 30, pack.ProvenanceIndustry: 28, pack.ProvenanceDesign: 10,
		},
		Off: []string{"GH-REG-13", "GH-TM-16", "GH-TM-19", "GH-TM-23", "GH-TM-24", "GH-SEC-13", "GH-GOV-11"},
	}, *update)
}
