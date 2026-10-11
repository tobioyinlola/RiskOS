package webhooks

import (
	"context"
	"fmt"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/store"
)

// Querier runs queries outside a tenant transaction. *pgxpool.Pool connected as the application
// role satisfies it.
type Querier = store.Querier

// KeyUsage reports how many endpoint secrets, across tenants, were derived with each signing key
// id and are still in use (current secrets, and previous ones within their overlap). A key may be
// removed from configuration only once its count is zero (ADR-0008). Counts only.
func KeyUsage(ctx context.Context, q Querier) (map[int]int, error) {
	return store.KeyUsage(ctx, q)
}

// PreviousKeyUnused reports whether a previous signing key is configured and no endpoint secret
// still depends on it, which means it can be removed (ADR-0008).
func PreviousKeyUnused(ctx context.Context, q Querier, cfg Config) (bool, error) {
	if len(cfg.PreviousSigningKey) == 0 {
		return false, nil
	}
	usage, err := KeyUsage(ctx, q)
	if err != nil {
		return false, err
	}
	return usage[cfg.PreviousSigningKeyID] == 0, nil
}

// VerifyKeyring refuses a configuration that lacks a signing key some endpoint still uses.
// Starting anyway would fail every delivery to those endpoints, so the API and worker call it at
// start-up and stop.
func VerifyKeyring(ctx context.Context, q Querier, cfg Config) error {
	usage, err := KeyUsage(ctx, q)
	if err != nil {
		return err
	}
	var missing []int
	for id, n := range usage {
		if n > 0 && !cfg.HasKey(id) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return fmt.Errorf("webhook signing keys %v are not configured but endpoints still use them: "+
			"set RISKOS_WEBHOOK_SIGNING_KEY_PREVIOUS and its _ID (ADR-0008)", missing)
	}
	return nil
}
