package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

// consoleReseal moves every value sealed under RISKOS_CONSOLE_SECRET_PREVIOUS to
// RISKOS_CONSOLE_SECRET, so the previous secret can be retired (ADR-0008). It reads across
// tenants as the schema owner, like audit verify, and prints counts only. Run it again until
// "still on the previous secret" is zero; values that opened with neither secret are reported and
// left untouched.
func consoleReseal(ctx context.Context, cfg config.Config, out io.Writer) error {
	if cfg.Console.SecretPrevious == "" {
		return errors.New("RISKOS_CONSOLE_SECRET_PREVIOUS is required: there is nothing to reseal without it")
	}
	if err := cfg.Console.Validate(cfg.Env); err != nil {
		return err
	}
	if cfg.Postgres.MigrationURL == "" {
		return errors.New("RISKOS_POSTGRES_MIGRATION_URL is required to reseal across tenants")
	}
	ownerCfg := cfg.Postgres
	ownerCfg.URL = cfg.Postgres.MigrationURL
	pool, err := db.NewPool(ctx, ownerCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := auth.ResealConsoleSecrets(ctx, pool, []byte(cfg.Console.Secret), []byte(cfg.Console.SecretPrevious))
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "resealed: %d\nalready on the current secret: %d\ncould not be opened with either secret: %d\n",
		res.Resealed, res.Current, res.Failed); err != nil {
		return err
	}
	if res.Failed > 0 {
		return fmt.Errorf("%d sealed values could not be opened with either secret; they were left unchanged", res.Failed)
	}
	return nil
}
