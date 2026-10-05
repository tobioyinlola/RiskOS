package rw_test

import (
	"flag"
	"testing"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/librarytest"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/rw"
)

var update = flag.Bool("update", false, "regenerate text.json from the library document")

func TestPackFollowsTheLibrary(t *testing.T) {
	librarytest.Check(t, ".", rw.Files, ng.Files, librarytest.Expect{
		Country:  "RW",
		Families: map[string]int{"REG": 14, "TM": 24, "SEC": 16, "GOV": 10},
		Provenance: map[pack.Provenance]int{
			pack.ProvenanceRegulatory: 31, pack.ProvenanceIndustry: 27, pack.ProvenanceDesign: 6,
		},
		Off: []string{"RW-REG-05", "RW-REG-08", "RW-REG-14", "RW-TM-16", "RW-TM-18", "RW-SEC-03", "RW-SEC-16", "RW-GOV-07"},
	}, *update)
}
