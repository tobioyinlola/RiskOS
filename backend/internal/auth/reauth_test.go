package auth_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
)

// Every re-authentication operation must name a real API operation, or the check silently
// protects nothing.
func TestReauthOperationsExist(t *testing.T) {
	api := reflect.TypeFor[apigen.StrictServerInterface]()
	for _, op := range auth.ReauthOperations() {
		_, ok := api.MethodByName(op)
		assert.True(t, ok, "%s is not an operation of the API", op)
	}
}
