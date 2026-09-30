// Package service implements recording, listing and verifying the audit log.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/requestctx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Service records and reads audit entries.
type Service struct {
	store  store.Store
	runner txn.Runner
	clock  clock.Clock
}

// New returns a Service.
func New(runner txn.Runner, clk clock.Clock) *Service {
	return &Service{runner: runner, clock: clk}
}

// Record appends e to the tenant's chain. It must be called inside the transaction of the action
// being audited (txn.Runner.Run), so the action and its audit entry commit or fail together.
func (s *Service) Record(ctx context.Context, e domain.Entry) error {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	meta, err := canonicalJSON(e.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}

	seq, prev, err := s.store.LockHead(ctx, scope.TenantID)
	if err != nil {
		return err
	}
	r := domain.Record{
		TenantID: scope.TenantID,
		Seq:      seq + 1,
		ID:       ids.NewV7(),
		// PostgreSQL stores microseconds; truncating first keeps the hashed value identical to
		// the stored one.
		OccurredAt:   s.clock.Now().UTC().Truncate(time.Microsecond),
		ActorType:    string(scope.Principal.Type),
		ActorID:      scope.Principal.ID,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		Outcome:      e.Outcome,
		RequestID:    requestctx.RequestID(ctx),
		Metadata:     meta,
		PrevHash:     prev,
	}
	if !e.TenantLevel {
		envID := scope.EnvironmentID
		r.EnvironmentID = &envID
	}
	r.Hash = domain.ComputeHash(r)
	return s.store.Append(ctx, r)
}

// List returns a newest-first page of entries visible to the caller's scope. Requires
// audit_logs:read.
func (s *Service) List(ctx context.Context, p domain.Page) ([]domain.Record, error) {
	if err := authz.Require(ctx, authz.AuditLogsRead); err != nil {
		return nil, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.Record
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		out, err = s.store.List(ctx, scope.TenantID, p)
		return err
	})
	return out, err
}

// verifyBatch is how many entries are read per round trip during verification.
const verifyBatch = 1000

// Verify recomputes a tenant's whole chain and compares it with the stored hashes and head. It
// detects modified, deleted, inserted and reordered entries.
func Verify(ctx context.Context, r store.Reader, tenantID uuid.UUID) (domain.VerifyResult, error) {
	res := domain.VerifyResult{Valid: true}
	headSeq, headHash, err := r.Head(ctx, tenantID)
	if err != nil {
		return res, err
	}
	res.HeadSeq = headSeq

	fail := func(seq int64, reason string) (domain.VerifyResult, error) {
		res.Valid, res.FirstInvalidSeq, res.Reason = false, seq, reason
		return res, nil
	}

	prevHash := domain.GenesisHash()
	var lastSeq int64
	for {
		batch, err := r.Segment(ctx, tenantID, lastSeq, verifyBatch)
		if err != nil {
			return res, err
		}
		for _, rec := range batch {
			switch {
			case rec.Seq != lastSeq+1:
				return fail(lastSeq+1, fmt.Sprintf("entry %d is missing", lastSeq+1))
			case !bytes.Equal(rec.PrevHash, prevHash):
				return fail(rec.Seq, "prev_hash does not match the previous entry")
			case !bytes.Equal(domain.ComputeHash(rec), rec.Hash):
				return fail(rec.Seq, "entry content does not match its hash")
			}
			prevHash, lastSeq = rec.Hash, rec.Seq
			res.Entries++
		}
		if len(batch) < verifyBatch {
			break
		}
	}

	if lastSeq != headSeq || (headSeq > 0 && !bytes.Equal(prevHash, headHash)) {
		return fail(lastSeq+1, fmt.Sprintf("chain ends at %d but the head records %d", lastSeq, headSeq))
	}
	return res, nil
}

// canonicalJSON encodes metadata with sorted keys and no insignificant whitespace.
// encoding/json sorts map keys, so equal maps always produce identical bytes.
func canonicalJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
