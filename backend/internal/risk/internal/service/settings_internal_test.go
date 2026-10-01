package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

func TestAutoBlockTransition(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	off := domain.Settings{AutoBlock: domain.AutoBlockOff}
	shadowSince := func(d time.Duration) domain.Settings {
		since := now.Add(-d)
		return domain.Settings{AutoBlock: domain.AutoBlockShadow, AutoBlockSince: &since}
	}

	mode, since, err := autoBlockTransition(off, domain.AutoBlockShadow, now)
	require.NoError(t, err)
	assert.Equal(t, domain.AutoBlockShadow, mode)
	assert.Equal(t, now, *since, "shadow evidence starts now")

	_, _, err = autoBlockTransition(off, domain.AutoBlockOn, now)
	assert.ErrorIs(t, err, domain.ErrSettingsInvalid, "never straight from off to on")
	_, _, err = autoBlockTransition(shadowSince(13*24*time.Hour), domain.AutoBlockOn, now)
	assert.ErrorIs(t, err, domain.ErrSettingsInvalid, "not before 14 days of shadow")

	enough := shadowSince(14 * 24 * time.Hour)
	mode, since, err = autoBlockTransition(enough, domain.AutoBlockOn, now)
	require.NoError(t, err)
	assert.Equal(t, domain.AutoBlockOn, mode)
	assert.Equal(t, enough.AutoBlockSince, since, "the evidence period is kept")

	on := domain.Settings{AutoBlock: domain.AutoBlockOn, AutoBlockSince: enough.AutoBlockSince}
	mode, since, err = autoBlockTransition(on, domain.AutoBlockShadow, now)
	require.NoError(t, err, "switching back to shadow is always allowed")
	assert.Equal(t, domain.AutoBlockShadow, mode)
	assert.Equal(t, enough.AutoBlockSince, since)
	mode, since, err = autoBlockTransition(on, domain.AutoBlockOff, now)
	require.NoError(t, err, "the kill switch is always allowed")
	assert.Equal(t, domain.AutoBlockOff, mode)
	assert.Nil(t, since)

	mode, _, err = autoBlockTransition(on, "", now)
	require.NoError(t, err)
	assert.Equal(t, domain.AutoBlockOn, mode, "an omitted mode keeps the current one")
	_, _, err = autoBlockTransition(off, "always", now)
	assert.ErrorIs(t, err, domain.ErrSettingsInvalid)
}
