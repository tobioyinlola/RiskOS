package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
)

func TestRotationsInProgress(t *testing.T) {
	assert.Empty(t, config.Config{}.RotationsInProgress(), "nothing previous, nothing to say")

	secret := strings.Repeat("s", 32)
	c := config.Config{
		Auth:     config.Auth{APIKeyPepperPrevious: secret},
		Webhooks: config.Webhooks{PreviousSigningKey: secret},
		Console:  config.Console{SecretPrevious: secret},
	}
	got := c.RotationsInProgress()
	assert.Len(t, got, 3)
	for name, condition := range got {
		assert.True(t, strings.HasPrefix(name, "RISKOS_"), name)
		assert.NotEmpty(t, condition)
		assert.NotContains(t, condition, secret, "values are never reported")
	}
}
