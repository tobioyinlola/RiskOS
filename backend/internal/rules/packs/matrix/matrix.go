// Package matrix is the library's side-by-side comparison of the countries' legal clocks and
// thresholds (GHANA_RWANDA_RULES_LIBRARY.md section 6, ADR-0007). matrix.json is copied from the
// library by its test (go test ./internal/rules/packs/matrix -update), never retyped.
package matrix

import (
	_ "embed"
	"encoding/json"
)

// Row is one item compared across Nigeria, Ghana and Rwanda, with the working default for a
// multi-country workspace, verbatim.
type Row struct {
	Item           string `json:"item"`
	Nigeria        string `json:"nigeria"`
	Ghana          string `json:"ghana"`
	Rwanda         string `json:"rwanda"`
	WorkingDefault string `json:"working_default"`
}

//go:embed matrix.json
var file []byte

// Rows returns the comparison.
func Rows() ([]Row, error) {
	var out []Row
	err := json.Unmarshal(file, &out)
	return out, err
}
