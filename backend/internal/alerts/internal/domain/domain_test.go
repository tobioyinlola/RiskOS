package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts/internal/domain"
)

func TestCheckTransition(t *testing.T) {
	ok := [][2]domain.Status{
		{domain.StatusOpen, domain.StatusInvestigating},
		{domain.StatusOpen, domain.StatusFalsePositive},
		{domain.StatusOpen, domain.StatusResolved},
		{domain.StatusInvestigating, domain.StatusOpen},
		{domain.StatusInvestigating, domain.StatusResolved},
		{domain.StatusResolved, domain.StatusResolved},
	}
	for _, c := range ok {
		assert.NoError(t, domain.CheckTransition(c[0], c[1]), "%s -> %s", c[0], c[1])
	}
	conflicts := [][2]domain.Status{
		{domain.StatusEscalated, domain.StatusResolved},
		{domain.StatusResolved, domain.StatusOpen},
		{domain.StatusFalsePositive, domain.StatusInvestigating},
	}
	for _, c := range conflicts {
		assert.ErrorIs(t, domain.CheckTransition(c[0], c[1]), domain.ErrConflict, "%s -> %s", c[0], c[1])
	}
	assert.ErrorIs(t, domain.CheckTransition(domain.StatusOpen, domain.StatusEscalated), domain.ErrInvalid,
		"escalation only by creating a case")
	assert.ErrorIs(t, domain.CheckTransition(domain.StatusOpen, "closed"), domain.ErrInvalid)
}

func TestSeverity(t *testing.T) {
	assert.Equal(t, "critical", domain.MaxSeverity("high", "critical"))
	assert.Equal(t, "high", domain.MaxSeverity("high", "medium"))
	assert.Equal(t, "Critical-risk activity", domain.Title(domain.KindRisk, "critical"))
	assert.Equal(t, "Risk evaluations ran with missing signals", domain.Title(domain.KindSecurity, "high"))
}
