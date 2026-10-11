package service

import (
	"context"
	"fmt"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/console"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
)

// ResealResult counts what a reseal did, never what was sealed.
type ResealResult struct {
	// Resealed values were under the previous secret and are now under the current one.
	Resealed int
	// Current values were already under the current secret.
	Current int
	// Failed values could not be opened with either secret: they are left as they are.
	Failed int
}

// Reseal moves every value sealed under the previous console secret (TOTP secrets and SSO client
// secrets) to the current one, so the previous secret can be retired (ADR-0008). It is idempotent
// and safe to run while the service is live: each write is a compare and swap on the value read.
// db must connect as the schema owner; it works across tenants.
func Reseal(ctx context.Context, db store.DB, secret, previous []byte) (ResealResult, error) {
	var res ResealResult
	if len(previous) == 0 {
		return res, fmt.Errorf("reseal needs the previous secret (RISKOS_CONSOLE_SECRET_PREVIOUS)")
	}
	sealer, err := console.NewSealer(secret, previous)
	if err != nil {
		return res, err
	}
	for _, kind := range []store.SealedKind{store.SealedTOTP, store.SealedSSO} {
		rows, err := store.ListSealed(ctx, db, kind)
		if err != nil {
			return res, err
		}
		for _, r := range rows {
			aad := r.ID[:]
			if kind == store.SealedSSO {
				aad = secretAAD(r.ID)
			}
			upgraded, err := sealer.Reseal(r.Ciphertext, aad)
			switch {
			case err != nil:
				res.Failed++
			case upgraded == nil:
				res.Current++
			default:
				if _, err := store.ReplaceSealed(ctx, db, kind, r.ID, r.Ciphertext, upgraded); err != nil {
					return res, err
				}
				res.Resealed++
			}
		}
	}
	return res, nil
}
