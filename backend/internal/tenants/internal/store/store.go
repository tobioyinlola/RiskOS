// Package store is the only code that reads or writes the tenants and environments tables.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants/internal/store/sqlcgen"
)

// ErrNotFound is returned when a tenant or environment does not exist in the current scope.
var ErrNotFound = errors.New("not found")

// Store operates inside the tenant transaction carried by the context.
type Store struct{}

func queries(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, err
	}
	return sqlcgen.New(tx), nil
}

// InsertTenant creates a tenant row. origin is "operator" or "self_serve"; a self-serve tenant's
// production environment starts locked.
func (Store) InsertTenant(ctx context.Context, id uuid.UUID, name, region, origin string, productionEnabled bool) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertTenant(ctx, sqlcgen.InsertTenantParams{
		ID: id, Name: name, Region: region, Origin: origin, ProductionEnabled: productionEnabled,
	}); err != nil {
		return fmt.Errorf("insert tenant: %w", err)
	}
	return nil
}

// InsertEnvironment creates an environment of tenantID.
func (Store) InsertEnvironment(ctx context.Context, id, tenantID uuid.UUID, kind string) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertEnvironment(ctx, sqlcgen.InsertEnvironmentParams{ID: id, TenantID: tenantID, Kind: kind}); err != nil {
		return fmt.Errorf("insert environment: %w", err)
	}
	return nil
}

// Environment returns an environment's kind and its tenant's status.
func (Store) Environment(ctx context.Context, tenantID, id uuid.UUID) (kind, tenantStatus string, err error) {
	q, err := queries(ctx)
	if err != nil {
		return "", "", err
	}
	row, err := q.GetEnvironment(ctx, sqlcgen.GetEnvironmentParams{TenantID: tenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("get environment: %w", err)
	}
	return row.Kind, row.TenantStatus, nil
}

// EnvironmentByKind returns the environment ID of kind and the tenant's status.
func (Store) EnvironmentByKind(ctx context.Context, tenantID uuid.UUID, kind string) (uuid.UUID, string, error) {
	q, err := queries(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}
	row, err := q.GetEnvironmentByKind(ctx, sqlcgen.GetEnvironmentByKindParams{TenantID: tenantID, Kind: kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("get environment: %w", err)
	}
	return row.ID, row.TenantStatus, nil
}

// Onboarding is a tenant's compliance onboarding with its tenant's name and production state.
type Onboarding = sqlcgen.GetOnboardingRow

// InsertOnboarding starts tenantID's onboarding as an empty draft.
func (Store) InsertOnboarding(ctx context.Context, tenantID uuid.UUID, by string, now time.Time) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertOnboarding(ctx, sqlcgen.InsertOnboardingParams{TenantID: tenantID, Now: now, UpdatedBy: by}); err != nil {
		return fmt.Errorf("insert onboarding: %w", err)
	}
	return nil
}

// Onboarding returns tenantID's onboarding; forUpdate locks it first. ErrNotFound when the tenant
// has none (operator-created tenants).
func (Store) Onboarding(ctx context.Context, tenantID uuid.UUID, forUpdate bool) (Onboarding, error) {
	q, err := queries(ctx)
	if err != nil {
		return Onboarding{}, err
	}
	if forUpdate {
		if _, err := q.GetOnboardingForUpdate(ctx, tenantID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Onboarding{}, ErrNotFound
			}
			return Onboarding{}, fmt.Errorf("lock onboarding: %w", err)
		}
	}
	o, err := q.GetOnboarding(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Onboarding{}, ErrNotFound
	}
	if err != nil {
		return Onboarding{}, fmt.Errorf("get onboarding: %w", err)
	}
	return o, nil
}

// SaveOnboardingAnswers replaces the answers (a rejected onboarding becomes a draft again).
func (Store) SaveOnboardingAnswers(ctx context.Context, tenantID uuid.UUID, answers []byte, by string, now time.Time) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.SaveOnboardingAnswers(ctx, sqlcgen.SaveOnboardingAnswersParams{
		TenantID: tenantID, Answers: answers, UpdatedBy: by, Now: now,
	}); err != nil {
		return fmt.Errorf("save onboarding: %w", err)
	}
	return nil
}

// SubmitOnboarding marks the onboarding submitted for review.
func (Store) SubmitOnboarding(ctx context.Context, tenantID uuid.UUID, by string, now time.Time) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.SubmitOnboarding(ctx, sqlcgen.SubmitOnboardingParams{TenantID: tenantID, SubmittedBy: &by, Now: &now}); err != nil {
		return fmt.Errorf("submit onboarding: %w", err)
	}
	return nil
}

// ReviewOnboarding records a staff decision ("approved" or "rejected") and sets whether the
// tenant's production environment is open.
func (Store) ReviewOnboarding(ctx context.Context, tenantID uuid.UUID, status, by string, note *string, now time.Time) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.ReviewOnboarding(ctx, sqlcgen.ReviewOnboardingParams{
		TenantID: tenantID, Status: status, ReviewedBy: &by, ReviewNote: note, Now: &now,
	}); err != nil {
		return fmt.Errorf("review onboarding: %w", err)
	}
	if err := q.SetProductionEnabled(ctx, sqlcgen.SetProductionEnabledParams{TenantID: tenantID, Enabled: status == "approved"}); err != nil {
		return fmt.Errorf("set production access: %w", err)
	}
	return nil
}
