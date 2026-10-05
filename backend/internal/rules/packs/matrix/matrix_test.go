package matrix_test

import (
	"bufio"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/librarytest"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/matrix"
)

var update = flag.Bool("update", false, "regenerate matrix.json from the library document")

// The matrix is section 6 of the library, row for row.
func TestMatrixMatchesLibrary(t *testing.T) {
	f, err := os.Open(librarytest.Path)
	require.NoError(t, err)
	defer f.Close()
	var rows []matrix.Row
	in := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "## 6."):
			in = true
		case strings.HasPrefix(line, "## 7."):
			in = false
		}
		if !in || !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Item |") {
			continue
		}
		cols := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "| "), " |"), " | ")
		require.Len(t, cols, 5, line)
		rows = append(rows, matrix.Row{Item: cols[0], Nigeria: cols[1], Ghana: cols[2], Rwanda: cols[3], WorkingDefault: cols[4]})
	}
	require.NoError(t, sc.Err())
	require.Len(t, rows, 14, "section 6 compares 14 items")
	want, err := json.MarshalIndent(rows, "", "  ")
	require.NoError(t, err)
	want = append(want, '\n')
	if *update {
		require.NoError(t, os.WriteFile("matrix.json", want, 0o600))
	}
	got, err := matrix.Rows()
	require.NoError(t, err)
	assert.Equal(t, rows, got, "matrix.json differs from the library; run go test with -update")
}
