// Package store is the only code that reads or writes the audit tables.
package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/db"
)

// Store operates inside the tenant transaction carried by the context.
type Store struct{}

func queries(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, err := db.TxFrom(ctx)
	if err != nil {
		return nil, err
	}
	return sqlcgen.New(tx), nil
}

// LockHead returns the tenant's chain head, creating it on first use, and locks it until the
// transaction ends so appends for a tenant are serialized.
func (Store) LockHead(ctx context.Context, tenantID uuid.UUID) (seq int64, hash []byte, err error) {
	q, err := queries(ctx)
	if err != nil {
		return 0, nil, err
	}
	if err := q.EnsureChainHead(ctx, tenantID); err != nil {
		return 0, nil, fmt.Errorf("ensure audit chain head: %w", err)
	}
	head, err := q.LockChainHead(ctx, tenantID)
	if err != nil {
		return 0, nil, fmt.Errorf("lock audit chain head: %w", err)
	}
	return head.Seq, head.Hash, nil
}

// Append inserts r and advances the chain head to it.
func (Store) Append(ctx context.Context, r domain.Record) error {
	q, err := queries(ctx)
	if err != nil {
		return err
	}
	if err := q.InsertAuditEntry(ctx, sqlcgen.InsertAuditEntryParams{
		TenantID: r.TenantID, Seq: r.Seq, ID: r.ID, EnvironmentID: r.EnvironmentID,
		OccurredAt: r.OccurredAt, ActorType: r.ActorType, ActorID: r.ActorID, Action: r.Action,
		ResourceType: r.ResourceType, ResourceID: r.ResourceID, Outcome: string(r.Outcome),
		RequestID: r.RequestID, Metadata: string(r.Metadata), PrevHash: r.PrevHash, Hash: r.Hash,
	}); err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	if err := q.AdvanceChainHead(ctx, sqlcgen.AdvanceChainHeadParams{TenantID: r.TenantID, Seq: r.Seq, Hash: r.Hash}); err != nil {
		return fmt.Errorf("advance audit chain head: %w", err)
	}
	return nil
}

// List returns a newest-first page of the entries visible in the current scope.
func (Store) List(ctx context.Context, tenantID uuid.UUID, p domain.Page) ([]domain.Record, error) {
	q, err := queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAuditEntries(ctx, sqlcgen.ListAuditEntriesParams{
		TenantID: tenantID, BeforeID: p.Before, RowLimit: int32(p.Limit), //nolint:gosec // limit validated to 1..101
	})
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	return toRecords(rows), nil
}

// Reader reads a tenant's whole chain for verification. It is used with an operator connection
// (the table owner), because entries of every environment must be visible to verify the chain.
type Reader struct {
	q *sqlcgen.Queries
}

// NewReader returns a Reader on conn (a *pgxpool.Pool or connection of the owner role).
func NewReader(conn sqlcgen.DBTX) Reader { return Reader{q: sqlcgen.New(conn)} }

// Segment returns up to limit entries with seq > afterSeq, in seq order.
func (r Reader) Segment(ctx context.Context, tenantID uuid.UUID, afterSeq int64, limit int32) ([]domain.Record, error) {
	rows, err := r.q.ListChainSegment(ctx, sqlcgen.ListChainSegmentParams{TenantID: tenantID, AfterSeq: afterSeq, RowLimit: limit})
	if err != nil {
		return nil, fmt.Errorf("read audit chain: %w", err)
	}
	return toRecords(rows), nil
}

// Head returns the recorded chain head.
func (r Reader) Head(ctx context.Context, tenantID uuid.UUID) (seq int64, hash []byte, err error) {
	h, err := r.q.GetChainHead(ctx, tenantID)
	if err != nil {
		return 0, nil, fmt.Errorf("read audit chain head: %w", err)
	}
	return h.Seq, h.Hash, nil
}

func toRecords(rows []sqlcgen.AuditLog) []domain.Record {
	out := make([]domain.Record, len(rows))
	for i, r := range rows {
		out[i] = domain.Record{
			TenantID: r.TenantID, Seq: r.Seq, ID: r.ID, EnvironmentID: r.EnvironmentID,
			OccurredAt: r.OccurredAt, ActorType: r.ActorType, ActorID: r.ActorID, Action: r.Action,
			ResourceType: r.ResourceType, ResourceID: r.ResourceID, Outcome: domain.Outcome(r.Outcome),
			RequestID: r.RequestID, Metadata: []byte(r.Metadata), PrevHash: r.PrevHash, Hash: r.Hash,
		}
	}
	return out
}
