package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A cell a spreadsheet would read as a formula is exported as text (CSV injection).
func TestSpreadsheetSafe(t *testing.T) {
	for in, want := range map[string]string{
		"=HYPERLINK(\"http://x\")":    "'=HYPERLINK(\"http://x\")",
		"+1":                          "'+1",
		"-5% fee":                     "'-5% fee",
		"@SUM(A1)":                    "'@SUM(A1)",
		"\tTab":                       "'\tTab",
		"\rReturn":                    "'\rReturn",
		"GH-REG-04":                   "GH-REG-04",
		"Cash at or above GHS 50,000": "Cash at or above GHS 50,000",
		"":                            "",
	} {
		assert.Equal(t, want, spreadsheetSafe(in), "%q", in)
	}
}
