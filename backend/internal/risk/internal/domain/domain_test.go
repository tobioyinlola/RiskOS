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
		0: domain.LevelLow, 29: domain.LevelLow,
		30: domain.LevelMedium, 59: domain.LevelMedium,
		60: domain.LevelHigh, 79: domain.LevelHigh,
		80: domain.LevelCritical, 100: domain.LevelCritical,
	}
	for score, want := range tests {
		assert.Equal(t, want, domain.LevelForScore(score), "score %d", score)
	}
}

func TestDefaultDecisionNeverBlocks(t *testing.T) {
	assert.Equal(t, domain.DecisionAllow, domain.DefaultDecision(domain.LevelLow))
	assert.Equal(t, domain.DecisionAllow, domain.DefaultDecision(domain.LevelMedium))
	assert.Equal(t, domain.DecisionStepUp, domain.DefaultDecision(domain.LevelHigh))
	assert.Equal(t, domain.DecisionReview, domain.DefaultDecision(domain.LevelCritical), "CRITICAL defaults to REVIEW (decision 12)")
}
