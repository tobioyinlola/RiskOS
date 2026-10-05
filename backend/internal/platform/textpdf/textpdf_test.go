package textpdf_test

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/textpdf"
)

// The document is a well-formed PDF: header, every object at the offset the cross-reference
// table states, and the trailer pointing at the table.
func TestWellFormed(t *testing.T) {
	d := textpdf.New()
	d.Title("Ghana rules library")
	d.Heading("GH-REG-04 Cash transaction report")
	d.Paragraph(strings.Repeat("A cash deposit of GHS 50,000 at a bank (or more) is reported. ", 400))
	d.Label("Caveat", "Uses the FIC’s figures – not the vendor’s ₦ claim (C4).")
	out := d.Bytes()

	require.True(t, bytes.HasPrefix(out, []byte("%PDF-1.4\n")))
	require.True(t, bytes.HasSuffix(out, []byte("%%EOF\n")))
	start := regexp.MustCompile(`startxref\n(\d+)\n`).FindSubmatch(out)
	require.NotNil(t, start)
	xref, err := strconv.Atoi(string(start[1]))
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(out[xref:], []byte("xref\n")))
	entries := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllSubmatch(out[xref:], -1)
	require.NotEmpty(t, entries)
	for i, e := range entries {
		off, err := strconv.Atoi(string(e[1]))
		require.NoError(t, err)
		assert.True(t, bytes.HasPrefix(out[off:], []byte(strconv.Itoa(i+1)+" 0 obj")), "object %d", i+1)
	}
	pages := regexp.MustCompile(`/Count (\d+)`).FindSubmatch(out)
	require.NotNil(t, pages)
	n, err := strconv.Atoi(string(pages[1]))
	require.NoError(t, err)
	assert.Greater(t, n, 1, "long text flows onto more pages")
	assert.Contains(t, string(out), "FIC\\222s", "typographic apostrophes become WinAnsi bytes")
	assert.Contains(t, string(out), "NGN claim", "the naira sign is spelled out")
	assert.Contains(t, string(out), "\\(C4\\)", "parentheses are escaped")
}
