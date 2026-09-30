// Package store is the only code that reads or writes the events and event_idempotency tables.
package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/events/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// Store operates inside the tenant transaction carried by the context.
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

// KeyRecord is a previously claimed idempotency key.
type KeyRecord struct {
	PayloadHash []byte
	EventID     uuid.UUID
	Response    []byte
}

// Claim claims an idempotency key for eventID. It returns (nil, nil) when this call claimed the
// key, or the existing record when the key was already claimed (after waiting for a concurrent
// claimant to commit).
func (Store) Claim(ctx context.Context, key string, payloadHash []byte, eventID uuid.UUID) (*KeyRecord, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	n, err := q.ClaimEventKey(ctx, sqlcgen.ClaimEventKeyParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, EventKey: key, PayloadHash: payloadHash, EventID: eventID,
	})
	if err != nil {
		return nil, fmt.Errorf("claim event key: %w", err)
	}
	if n == 1 {
		return nil, nil //nolint:nilnil // claimed: there is no previous record
	}
	row, err := q.GetEventKey(ctx, sqlcgen.GetEventKeyParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, EventKey: key})
	if err != nil {
		return nil, fmt.Errorf("read event key: %w", err)
	}
	rec := &KeyRecord{PayloadHash: row.PayloadHash, EventID: row.EventID}
	if row.Response != nil {
		rec.Response = []byte(*row.Response)
	}
	return rec, nil
}

// SetResponse stores the response returned for a claimed key.
func (Store) SetResponse(ctx context.Context, key string, response []byte) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	if err := q.SetEventKeyResponse(ctx, sqlcgen.SetEventKeyResponseParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, EventKey: key, Response: new(string(response)),
	}); err != nil {
		return fmt.Errorf("store event response: %w", err)
	}
	return nil
}

// InsertParams describes a stored event.
type InsertParams struct {
	ID           uuid.UUID
	ReceivedAt   time.Time
	Event        domain.Event
	Normalized   []byte
	PayloadHash  []byte
	RawPayload   []byte
	IP           *netip.Addr
	EvaluationID uuid.UUID
	Score        int
	Level        string
	Decision     string
	RequestID    string
}

// Insert stores an event.
func (Store) Insert(ctx context.Context, p InsertParams) error {
	q, scope, err := scoped(ctx)
	if err != nil {
		return err
	}
	e := p.Event
	if err := q.InsertEvent(ctx, sqlcgen.InsertEventParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ID: p.ID, ReceivedAt: p.ReceivedAt,
		EventKey: e.Key, Type: e.Type, OccurredAt: e.OccurredAt,
		UserRef: e.Actor.UserID, AccountRef: optional(e.Actor.AccountID), DeviceRef: optional(e.Actor.DeviceID),
		SessionRef: optional(e.Actor.SessionID), Ip: p.IP, Country: optional(e.Context.Country),
		RawPayload: string(p.RawPayload), Normalized: p.Normalized, PayloadHash: p.PayloadHash,
		EvaluationID: p.EvaluationID, RiskScore: int16(p.Score), RiskLevel: p.Level, Decision: p.Decision, //nolint:gosec // score is 0..100
		RequestID: p.RequestID,
	}); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// Get returns an event of the current environment. receivedAt must be the time embedded in id.
func (Store) Get(ctx context.Context, id uuid.UUID, receivedAt time.Time) (domain.Detail, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return domain.Detail{}, err
	}
	row, err := q.GetEvent(ctx, sqlcgen.GetEventParams{TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, ReceivedAt: receivedAt, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Detail{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Detail{}, fmt.Errorf("get event: %w", err)
	}
	return domain.Detail{
		ID: row.ID, Key: row.EventKey, Type: row.Type, OccurredAt: row.OccurredAt, ReceivedAt: row.ReceivedAt,
		RawPayload: []byte(row.RawPayload), Normalized: bytes.Clone(row.Normalized), EvaluationID: row.EvaluationID,
	}, nil
}

// List returns a newest-first page of the current environment's events.
func (Store) List(ctx context.Context, p domain.Page) ([]domain.Summary, error) {
	q, scope, err := scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListEvents(ctx, sqlcgen.ListEventsParams{
		TenantID: scope.TenantID, EnvironmentID: scope.EnvironmentID, BeforeID: p.Before,
		RowLimit: int32(p.Limit), //nolint:gosec // limit validated to 1..101
	})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	out := make([]domain.Summary, len(rows))
	for i, r := range rows {
		out[i] = domain.Summary{
			ID: r.ID, Key: r.EventKey, Type: r.Type, OccurredAt: r.OccurredAt, ReceivedAt: r.ReceivedAt,
			Score: int(r.RiskScore), Level: r.RiskLevel, Decision: r.Decision,
		}
	}
	return out, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
