package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store/sqlcgen"
)

// ActivationChange is a change to a country activation: applied at once or pending a second
// person's approval (ADR-0007, phase 4).
type ActivationChange struct {
	ID           uuid.UUID
	ActivationID uuid.UUID
	// Action is create, update, status or fallback.
	Action               string
	StatusFrom, StatusTo *string
	// Details are an update's new values (licence_type, capabilities, day_starts_at).
	Details   map[string]any
	Rationale string
	// State is applied, pending, approved or rejected.
	State        string
	ProposedBy   string
	ProposedAt   time.Time
	DecidedBy    *string
	DecidedAt    *time.Time
	DecisionNote *string
}

// GapVerification is an institution's record that it checked an open question against the
// original text.
type GapVerification struct {
	PackID, GapID, VerifiedBy, Note string
	VerifiedAt                      time.Time
	Link                            *string
}

// ActivationInput is a new activation or an activation's editable settings.
type ActivationInput struct {
	ID, TenantID                     uuid.UUID
	Jurisdiction, BookingEntity      string
	PackID, PackVersion, LicenceType string
	Capabilities                     []string
	DayStartsAt                      int16
	Rationale                        *string
	UpdatedAt                        time.Time
	UpdatedBy                        string
}

// CreateActivation stores a draft activation; ErrConflict when the booking entity is taken.
func (Store) CreateActivation(ctx context.Context, in ActivationInput) (Activation, error) {
	q, err := queries(ctx)
	if err != nil {
		return Activation{}, err
	}
	r, err := q.InsertActivation(ctx, sqlcgen.InsertActivationParams{
		ID: in.ID, TenantID: in.TenantID, Jurisdiction: in.Jurisdiction, BookingEntity: in.BookingEntity,
		PackID: in.PackID, PackVersion: in.PackVersion, LicenceType: in.LicenceType, Capabilities: in.Capabilities,
		DayStartsAt: in.DayStartsAt, Rationale: in.Rationale, UpdatedAt: in.UpdatedAt, UpdatedBy: in.UpdatedBy,
	})
	if isUnique(err) {
		return Activation{}, ErrConflict
	}
	if err != nil {
		return Activation{}, fmt.Errorf("create activation: %w", err)
	}
	return Activation(r), nil
}

// UpdateActivation sets an activation's licence type, capabilities and day start.
func (Store) UpdateActivation(ctx context.Context, in ActivationInput) (Activation, error) {
	q, err := queries(ctx)
	if err != nil {
		return Activation{}, err
	}
	r, err := q.UpdateActivation(ctx, sqlcgen.UpdateActivationParams{
		LicenceType: in.LicenceType, Capabilities: in.Capabilities, DayStartsAt: in.DayStartsAt,
		UpdatedAt: in.UpdatedAt, UpdatedBy: in.UpdatedBy, TenantID: in.TenantID, ID: in.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activation{}, ErrNotFound
	}
	if err != nil {
		return Activation{}, fmt.Errorf("update activation: %w", err)
	}
	return Activation(r), nil
}

// SetActivationStatus moves an activation to status; going live records who proposed and who
// approved it.
func (Store) SetActivationStatus(ctx context.Context, tenantID, id uuid.UUID, status, rationale, by string, approvedBy *string, at time.Time) (Activation, error) {
	q, err := queries(ctx)
	if err != nil {
		return Activation{}, err
	}
	r, err := q.SetActivationStatus(ctx, sqlcgen.SetActivationStatusParams{
		Status: status, Rationale: &rationale, UpdatedAt: at, UpdatedBy: by, ActivatedBy: &by, ApprovedBy: approvedBy,
		TenantID: tenantID, ID: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activation{}, ErrNotFound
	}
	if err != nil {
		return Activation{}, fmt.Errorf("set activation status: %w", err)
	}
	return Activation(r), nil
}

// SetFallback makes an activation the tenant's fallback, in place of the current one.
func (Store) SetFallback(ctx context.Context, tenantID, id uuid.UUID, by string, at time.Time) (Activation, error) {
	q, err := queries(ctx)
	if err != nil {
		return Activation{}, err
	}
	if err := q.ClearFallback(ctx, tenantID); err != nil {
		return Activation{}, fmt.Errorf("clear fallback: %w", err)
	}
	r, err := q.SetFallback(ctx, sqlcgen.SetFallbackParams{UpdatedAt: at, UpdatedBy: by, TenantID: tenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Activation{}, ErrNotFound
	}
	if err != nil {
		return Activation{}, fmt.Errorf("set fallback: %w", err)
	}
	return Activation(r), nil
}

// InsertActivationChange records a change; ErrConflict when the activation already has one
// pending.
func (Store) InsertActivationChange(ctx context.Context, tenantID uuid.UUID, c ActivationChange) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	details := c.Details
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("activation change details: %w", err)
	}
	err = q.InsertActivationChange(ctx, sqlcgen.InsertActivationChangeParams{
		TenantID: tenantID, ID: c.ID, ActivationID: c.ActivationID, Action: c.Action, StatusFrom: c.StatusFrom,
		StatusTo: c.StatusTo, Details: raw, Rationale: c.Rationale, State: c.State, ProposedBy: c.ProposedBy,
		ProposedAt: c.ProposedAt,
	})
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("record activation change: %w", err)
	}
	return nil
}

func activationChange(r sqlcgen.ActivationChange) ActivationChange {
	c := ActivationChange{
		ID: r.ID, ActivationID: r.ActivationID, Action: r.Action, StatusFrom: r.StatusFrom, StatusTo: r.StatusTo,
		Rationale: r.Rationale, State: r.State, ProposedBy: r.ProposedBy, ProposedAt: r.ProposedAt,
		DecidedBy: r.DecidedBy, DecidedAt: r.DecidedAt, DecisionNote: r.DecisionNote, Details: map[string]any{},
	}
	_ = json.Unmarshal(r.Details, &c.Details) // written by InsertActivationChange
	return c
}

// ActivationChanges lists the tenant's activation changes, newest first; activation and state
// filter when set.
func (Store) ActivationChanges(ctx context.Context, tenantID uuid.UUID, activation *uuid.UUID, state string) ([]ActivationChange, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	var st *string
	if state != "" {
		st = &state
	}
	rows, err := q.ListActivationChanges(ctx, sqlcgen.ListActivationChangesParams{TenantID: tenantID, ActivationID: activation, State: st})
	if err != nil {
		return nil, fmt.Errorf("list activation changes: %w", err)
	}
	out := make([]ActivationChange, len(rows))
	for i, r := range rows {
		out[i] = activationChange(r)
	}
	return out, nil
}

// ActivationChange returns one change, locked for its decision.
func (Store) ActivationChange(ctx context.Context, tenantID, id uuid.UUID) (ActivationChange, error) {
	q, err := queries(ctx)
	if err != nil {
		return ActivationChange{}, err
	}
	r, err := q.GetActivationChange(ctx, sqlcgen.GetActivationChangeParams{TenantID: tenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivationChange{}, ErrNotFound
	}
	if err != nil {
		return ActivationChange{}, fmt.Errorf("activation change: %w", err)
	}
	return activationChange(r), nil
}

// DecideActivationChange approves or rejects a pending change.
func (Store) DecideActivationChange(ctx context.Context, tenantID, id uuid.UUID, state, by string, note *string, at time.Time) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.DecideActivationChange(ctx, sqlcgen.DecideActivationChangeParams{
		State: state, DecidedBy: &by, DecidedAt: &at, DecisionNote: note, TenantID: tenantID, ID: id,
	}); err != nil {
		return fmt.Errorf("decide activation change: %w", err)
	}
	return nil
}

// VerifyGap records a gap verification; ErrConflict when the gap is already verified.
func (Store) VerifyGap(ctx context.Context, tenantID uuid.UUID, v GapVerification) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	err = q.InsertGapVerification(ctx, sqlcgen.InsertGapVerificationParams{
		TenantID: tenantID, PackID: v.PackID, GapID: v.GapID, VerifiedBy: v.VerifiedBy, VerifiedAt: v.VerifiedAt,
		Note: v.Note, Link: v.Link,
	})
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("verify gap: %w", err)
	}
	return nil
}

// GapVerifications returns a pack's verified gaps for the tenant, by gap ID.
func (Store) GapVerifications(ctx context.Context, tenantID uuid.UUID, packID string) (map[string]GapVerification, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListGapVerifications(ctx, sqlcgen.ListGapVerificationsParams{TenantID: tenantID, PackID: packID})
	if err != nil {
		return nil, fmt.Errorf("list gap verifications: %w", err)
	}
	out := make(map[string]GapVerification, len(rows))
	for _, r := range rows {
		out[r.GapID] = GapVerification{
			PackID: r.PackID, GapID: r.GapID, VerifiedBy: r.VerifiedBy, Note: r.Note, VerifiedAt: r.VerifiedAt, Link: r.Link,
		}
	}
	return out, nil
}
