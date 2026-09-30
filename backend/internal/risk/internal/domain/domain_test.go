package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

func TestScoreIsSumOfWeightsCappedAt100(t *testing.T) {
	sig := func(w ...int) []domain.Signal {
		out := make([]domain.Signal, len(w))
		for i, x := range w {
			out[i] = domain.Signal{Code: "s", Weight: x}
		}
		return out
	}
	assert.Equal(t, 0, domain.Score(nil))
	assert.Equal(t, 59, domain.Score(sig(20, 15, 24)), "the spec's three example signals sum to 59, not 87")
	assert.Equal(t, 84, domain.Score(sig(20, 15, 24, 15, 10)), "the spec's five-signal example")
	assert.Equal(t, 100, domain.Score(sig(60, 60)))
}

func TestLevelBands(t *testing.T) {
	tests := map[int]domain.Level{
		0: domain.LevelMinimal, 19: domain.LevelMinimal,
		20: domain.LevelLow, 49: domain.LevelLow,
		50: domain.LevelElevated, 69: domain.LevelElevated,
		70: domain.LevelHigh, 89: domain.LevelHigh,
		90: domain.LevelCritical, 100: domain.LevelCritical,
	}
	for score, want := range tests {
		assert.Equal(t, want, domain.LevelForScore(score, domain.DefaultThresholds()), "score %d", score)
	}
	custom := domain.Thresholds{Low: 10, Elevated: 30, High: 60, Critical: 80}
	assert.Equal(t, domain.LevelCritical, domain.LevelForScore(84, custom), "tenant calibration")
	assert.Equal(t, domain.LevelMinimal, domain.LevelForScore(9, custom))
}

func TestThresholdsValid(t *testing.T) {
	assert.True(t, domain.DefaultThresholds().Valid())
	for _, bad := range []domain.Thresholds{
		{Low: 0, Elevated: 50, High: 70, Critical: 90}, {Low: 20, Elevated: 20, High: 70, Critical: 90},
		{Low: 20, Elevated: 50, High: 90, Critical: 70}, {Low: 20, Elevated: 50, High: 70, Critical: 101},
	} {
		assert.False(t, bad.Valid(), "%+v", bad)
	}
}

func TestDefaultDecisionNeverBlocks(t *testing.T) {
	assert.Equal(t, domain.DecisionAllow, domain.DefaultDecision(domain.LevelMinimal))
	assert.Equal(t, domain.DecisionAllow, domain.DefaultDecision(domain.LevelLow))
	assert.Equal(t, domain.DecisionReview, domain.DefaultDecision(domain.LevelElevated))
	assert.Equal(t, domain.DecisionStepUp, domain.DefaultDecision(domain.LevelHigh))
	assert.Equal(t, domain.DecisionReview, domain.DefaultDecision(domain.LevelCritical), "CRITICAL defaults to REVIEW; BLOCK is opt-in (step 8)")
}
