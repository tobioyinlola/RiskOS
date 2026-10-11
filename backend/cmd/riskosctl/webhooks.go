package main

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks"
)

// webhooksKeyUsage reports, per webhook signing key id, how many endpoint secrets are still
// derived with it and whether this process's configuration has the key (ADR-0008). A key can be
// removed from configuration only when its count is zero. It prints counts only.
//
// There is deliberately no command that rotates every endpoint's secret: secrets are shown to
// their customer once, so a forced rotation would leave customers without the new secret. They
// move to the new key by rotating their own secret, which the console and API offer with an
// overlap window.
func webhooksKeyUsage(ctx context.Context, cfg config.Config, out io.Writer) error {
	pool, err := db.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()
	usage, err := webhooks.KeyUsage(ctx, pool)
	if err != nil {
		return err
	}
	wc := webhooks.Config{
		SigningKey: []byte(cfg.Webhooks.SigningKey), SigningKeyID: cfg.Webhooks.SigningKeyID,
		PreviousSigningKey: []byte(cfg.Webhooks.PreviousSigningKey), PreviousSigningKeyID: cfg.Webhooks.PreviousSigningKeyID,
	}
	keys := make([]int, 0, len(usage))
	for id := range usage {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	if _, err := fmt.Fprintf(out, "current key id: %d\n", wc.CurrentKeyID()); err != nil {
		return err
	}
	for _, id := range keys {
		state := "configured"
		if !wc.HasKey(id) {
			state = "NOT CONFIGURED: deliveries to these endpoints will fail"
		}
		if _, err := fmt.Fprintf(out, "key %d: %d endpoint secrets in use (%s)\n", id, usage[id], state); err != nil {
			return err
		}
	}
	if len(keys) == 0 {
		_, err = fmt.Fprintln(out, "no webhook endpoints")
		return err
	}
	return nil
}
