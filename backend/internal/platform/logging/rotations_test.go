package logging_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/logging"
)

func TestWarnRotationsNamesVariablesNotValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logging.WarnRotations(t.Context(), logger, map[string]string{
		"RISKOS_CONSOLE_SECRET_PREVIOUS": "run reseal",
		"RISKOS_API_KEY_PEPPER_PREVIOUS": "wait 14 days",
	})
	out := buf.String()
	assert.Contains(t, out, "RISKOS_CONSOLE_SECRET_PREVIOUS")
	assert.Contains(t, out, "RISKOS_API_KEY_PEPPER_PREVIOUS")
	assert.Contains(t, out, "run reseal")
	assert.Less(t, bytes.Index(buf.Bytes(), []byte("RISKOS_API_KEY")), bytes.Index(buf.Bytes(), []byte("RISKOS_CONSOLE")),
		"in a stable order")

	buf.Reset()
	logging.WarnRotations(t.Context(), logger, nil)
	assert.Empty(t, buf.String())
}
