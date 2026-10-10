package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

// staffAuditVerify recomputes the staff audit log's hash chain. It reads as the schema owner, like
// audit verify. Entries written before the chain existed are outside it.
func staffAuditVerify(ctx context.Context, cfg config.Config, out io.Writer) error {
	if cfg.Postgres.MigrationURL == "" {
		return errors.New("RISKOS_POSTGRES_MIGRATION_URL is required to verify the staff audit chain")
	}
	ownerCfg := cfg.Postgres
	ownerCfg.URL = cfg.Postgres.MigrationURL
	pool, err := db.NewPool(ctx, ownerCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	res, err := admin.VerifyStaffAudit(ctx, pool)
	if err != nil {
		return err
	}
	if !res.Valid {
		_, _ = fmt.Fprintf(out, "INVALID: %d entries verified; first problem at sequence %d: %s\n", res.Entries, res.FirstInvalidSeq, res.Reason)
		return errors.New("staff audit chain verification failed")
	}
	_, err = fmt.Fprintf(out, "OK: %d entries, chain intact up to sequence %d.\n", res.Entries, res.HeadSeq)
	return err
}
