// Package store is the only code that touches the webhooks module's tables. Every query runs in
// the caller's tenant transaction and names tenant and environment explicitly.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/webhooks/internal/store/sqlcgen"
)

// Store accesses webhook endpoints, deliveries and attempts.
type Store struct{}

func scoped(ctx context.Context) (*sqlcgen.Queries, tenancy.Scope, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, tenancy.Scope{}, err
	}
	return sqlcgen.New(tx), scope, nil
}

// CountEndpoints returns the environment's number of endpoints.
func (Store) CountEndpoints(ctx context.Context) (int64, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountEndpoints(ctx, sqlcgen.CountEndpointsParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID})
	if err != nil {
		return 0, fmt.Errorf("count webhook endpoints: %w", err)
	}
	return n, nil
}

// InsertEndpoint creates an active endpoint with secret version 1.
func (Store) InsertEndpoint(ctx context.Context, e domain.Endpoint) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	keyID, err := keyID16(e.SecretKeyID)
	if err != nil {
		return err
	}
	if err := q.InsertEndpoint(ctx, sqlcgen.InsertEndpointParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: e.ID, Url: e.URL, Description: e.Description,
		EventTypes: typesToStrings(e.EventTypes), SecretKeyID: keyID, Now: e.CreatedAt, CreatedBy: e.CreatedBy,
	}); err != nil {
		return fmt.Errorf("insert webhook endpoint: %w", err)
	}
	return nil
}

// GetEndpoint returns an endpoint; forUpdate locks it for the rest of the transaction.
func (Store) GetEndpoint(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Endpoint, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return domain.Endpoint{}, err
	}
	var row sqlcgen.WebhookEndpoint
	if forUpdate {
		row, err = q.GetEndpointForUpdate(ctx, sqlcgen.GetEndpointForUpdateParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id})
	} else {
		row, err = q.GetEndpoint(ctx, sqlcgen.GetEndpointParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Endpoint{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Endpoint{}, fmt.Errorf("get webhook endpoint: %w", err)
	}
	return endpointToDomain(row), nil
}

// ListEndpoints returns the environment's endpoints, newest first.
func (Store) ListEndpoints(ctx context.Context) ([]domain.Endpoint, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListEndpoints(ctx, sqlcgen.ListEndpointsParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID})
	if err != nil {
		return nil, fmt.Errorf("list webhook endpoints: %w", err)
	}
	out := make([]domain.Endpoint, len(rows))
	for i, r := range rows {
		out[i] = endpointToDomain(r)
	}
	return out, nil
}

// Subscribed returns the active endpoints subscribed to eventType.
func (Store) Subscribed(ctx context.Context, eventType domain.EventType) ([]uuid.UUID, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	out, err := q.ListSubscribedEndpoints(ctx, sqlcgen.ListSubscribedEndpointsParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, EventType: string(eventType),
	})
	if err != nil {
		return nil, fmt.Errorf("list subscribed endpoints: %w", err)
	}
	return out, nil
}

// UpdateEndpoint stores an endpoint's mutable configuration and status.
func (Store) UpdateEndpoint(ctx context.Context, e domain.Endpoint) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.UpdateEndpoint(ctx, sqlcgen.UpdateEndpointParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: e.ID, Url: e.URL, Description: e.Description,
		EventTypes: typesToStrings(e.EventTypes), Status: e.Status, DisabledReason: e.DisabledReason,
		FailingSince: e.FailingSince, Now: e.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("update webhook endpoint: %w", err)
	}
	return nil
}

// RotateSecret moves to the next secret version, derived with key keyID; the previous one (and
// the key it was derived with) stays valid until until.
func (Store) RotateSecret(ctx context.Context, id uuid.UUID, keyID int, until, now time.Time) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	key, err := keyID16(keyID)
	if err != nil {
		return err
	}
	if err := q.RotateEndpointSecret(ctx, sqlcgen.RotateEndpointSecretParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id, PreviousValidUntil: &until,
		SecretKeyID: key, Now: now,
	}); err != nil {
		return fmt.Errorf("rotate webhook secret: %w", err)
	}
	return nil
}

// SetHealth records an endpoint's failing state and status.
func (Store) SetHealth(ctx context.Context, id uuid.UUID, failingSince *time.Time, status string, reason *string, now time.Time) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetEndpointHealth(ctx, sqlcgen.SetEndpointHealthParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id, FailingSince: failingSince,
		Status: status, DisabledReason: reason, Now: now,
	}); err != nil {
		return fmt.Errorf("set webhook endpoint health: %w", err)
	}
	return nil
}

// InsertDelivery creates a pending delivery.
func (Store) InsertDelivery(ctx context.Context, d domain.Delivery) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertDelivery(ctx, sqlcgen.InsertDeliveryParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: d.ID, EndpointID: d.EndpointID,
		EventID: d.EventID, EventType: string(d.EventType), Payload: d.Payload, Now: &d.CreatedAt,
	}); err != nil {
		return fmt.Errorf("insert webhook delivery: %w", err)
	}
	return nil
}

// GetDelivery returns a delivery; forUpdate locks it for the rest of the transaction.
func (Store) GetDelivery(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Delivery, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return domain.Delivery{}, err
	}
	var row sqlcgen.WebhookDelivery
	if forUpdate {
		row, err = q.GetDeliveryForUpdate(ctx, sqlcgen.GetDeliveryForUpdateParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id})
	} else {
		row, err = q.GetDelivery(ctx, sqlcgen.GetDeliveryParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Delivery{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Delivery{}, fmt.Errorf("get webhook delivery: %w", err)
	}
	return deliveryToDomain(row), nil
}

// DeliveryFilter selects an endpoint's deliveries.
type DeliveryFilter struct {
	EndpointID uuid.UUID
	Status     *string
	Limit      int
	Before     *uuid.UUID
}

// ListDeliveries returns a newest-first page of an endpoint's deliveries.
func (Store) ListDeliveries(ctx context.Context, f DeliveryFilter) ([]domain.Delivery, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDeliveries(ctx, sqlcgen.ListDeliveriesParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, EndpointID: f.EndpointID, Status: f.Status,
		BeforeID: f.Before, RowLimit: int32(f.Limit), //nolint:gosec // bounded page size
	})
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	out := make([]domain.Delivery, len(rows))
	for i, r := range rows {
		out[i] = deliveryToDomain(r)
	}
	return out, nil
}

// AttemptOutcome is the result of one attempt, as recorded.
type AttemptOutcome struct {
	Attempt    int
	StartedAt  time.Time
	Duration   time.Duration
	StatusCode int // 0 when there was no response
	Error      string
	// Status is the delivery's new status; NextAttemptAt is set when it will be retried.
	Status        string
	NextAttemptAt *time.Time
}

// RecordAttempt logs an attempt and updates the delivery.
func (Store) RecordAttempt(ctx context.Context, deliveryID uuid.UUID, o AttemptOutcome) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	var code *int32
	if o.StatusCode != 0 {
		c := int32(o.StatusCode) //nolint:gosec // HTTP status codes are small
		code = &c
	}
	var errText *string
	if o.Error != "" {
		errText = &o.Error
	}
	if err := q.InsertAttempt(ctx, sqlcgen.InsertAttemptParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: ids.NewV7(), DeliveryID: deliveryID,
		Attempt: int32(o.Attempt), StartedAt: o.StartedAt, DurationMs: int32(o.Duration.Milliseconds()), //nolint:gosec // small values
		StatusCode: code, Error: errText,
	}); err != nil {
		return fmt.Errorf("insert webhook attempt: %w", err)
	}
	var completed *time.Time
	if o.Status != domain.DeliveryPending {
		end := o.StartedAt.Add(o.Duration)
		completed = &end
	}
	if err := q.RecordDeliveryAttempt(ctx, sqlcgen.RecordDeliveryAttemptParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: deliveryID, Status: o.Status,
		AttemptedAt: &o.StartedAt, LastStatusCode: code, LastError: errText, NextAttemptAt: o.NextAttemptAt,
		CompletedAt: completed,
	}); err != nil {
		return fmt.Errorf("update webhook delivery: %w", err)
	}
	return nil
}

// FailDelivery ends a pending delivery without an attempt (for example, its endpoint was
// disabled).
func (Store) FailDelivery(ctx context.Context, id uuid.UUID, reason string, now time.Time) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.FailDelivery(ctx, sqlcgen.FailDeliveryParams{
		TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id, LastError: &reason, Now: &now,
	}); err != nil {
		return fmt.Errorf("fail webhook delivery: %w", err)
	}
	return nil
}

// RestartDelivery starts a new delivery cycle for a failed delivery.
func (Store) RestartDelivery(ctx context.Context, id uuid.UUID, now time.Time) error {
	q, s, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.RestartDelivery(ctx, sqlcgen.RestartDeliveryParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, ID: id, Now: &now}); err != nil {
		return fmt.Errorf("restart webhook delivery: %w", err)
	}
	return nil
}

// Attempts returns a delivery's attempts, oldest first.
func (Store) Attempts(ctx context.Context, deliveryID uuid.UUID) ([]domain.Attempt, error) {
	q, s, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAttempts(ctx, sqlcgen.ListAttemptsParams{TenantID: s.TenantID, EnvironmentID: s.EnvironmentID, DeliveryID: deliveryID})
	if err != nil {
		return nil, fmt.Errorf("list webhook attempts: %w", err)
	}
	out := make([]domain.Attempt, len(rows))
	for i, r := range rows {
		out[i] = domain.Attempt{Attempt: int(r.Attempt), StartedAt: r.StartedAt, DurationMS: int(r.DurationMs), Error: r.Error}
		if r.StatusCode != nil {
			c := int(*r.StatusCode)
			out[i].StatusCode = &c
		}
	}
	return out, nil
}

func endpointToDomain(r sqlcgen.WebhookEndpoint) domain.Endpoint {
	e := domain.Endpoint{
		ID: r.ID, URL: r.Url, Description: r.Description, Status: r.Status, DisabledReason: r.DisabledReason,
		SecretVersion: int(r.SecretVersion), SecretKeyID: int(r.SecretKeyID), PreviousSecretValidUntil: r.PreviousSecretValidUntil,
		FailingSince: r.FailingSince, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy, UpdatedAt: r.UpdatedAt,
	}
	for _, t := range r.EventTypes {
		e.EventTypes = append(e.EventTypes, domain.EventType(t))
	}
	if r.PreviousSecretVersion != nil {
		v := int(*r.PreviousSecretVersion)
		e.PreviousSecretVersion = &v
	}
	if r.PreviousSecretKeyID != nil {
		k := int(*r.PreviousSecretKeyID)
		e.PreviousSecretKeyID = &k
	}
	return e
}

func deliveryToDomain(r sqlcgen.WebhookDelivery) domain.Delivery {
	d := domain.Delivery{
		ID: r.ID, EndpointID: r.EndpointID, EventID: r.EventID, EventType: domain.EventType(r.EventType),
		Payload: r.Payload, Status: r.Status, Attempts: int(r.Attempts), NextAttemptAt: r.NextAttemptAt,
		LastAttemptAt: r.LastAttemptAt, LastError: r.LastError, CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt,
	}
	if r.LastStatusCode != nil {
		c := int(*r.LastStatusCode)
		d.LastStatusCode = &c
	}
	return d
}

func typesToStrings(ts []domain.EventType) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return out
}
