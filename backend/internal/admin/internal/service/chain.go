package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
)

// staffAuditDomain separates this hash from the tenant audit log's and from any other use of
// SHA-256, and versions the encoding.
const staffAuditDomain = "riskos.staffaudit.v1"

// ChainEntry is a staff audit entry as the hash covers it.
type ChainEntry struct {
	Seq          int64
	ID           uuid.UUID
	OccurredAt   time.Time // microsecond precision, as stored
	Actor        string
	Action       string
	TenantID     *uuid.UUID
	ResourceID   string
	Outcome      string
	MetadataText string
	PrevHash     []byte
}

// GenesisHash is the previous hash of the first chained entry.
func GenesisHash() []byte { return make([]byte, sha256.Size) }

// ComputeChainHash returns SHA-256 over a length-prefixed, fixed-order encoding of every field of
// e and the previous hash. Changing any field, the order or the previous entry changes it.
func ComputeChainHash(e ChainEntry) []byte {
	var b bytes.Buffer
	str := func(s string) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(s))) //nolint:gosec // lengths are bounded well below 4 GiB
		b.WriteString(s)
	}
	b.WriteString(staffAuditDomain)
	_ = binary.Write(&b, binary.BigEndian, e.Seq)
	b.Write(e.ID[:])
	_ = binary.Write(&b, binary.BigEndian, e.OccurredAt.UnixMicro())
	str(e.Actor)
	str(e.Action)
	if e.TenantID != nil {
		b.WriteByte(1)
		b.Write(e.TenantID[:])
	} else {
		b.WriteByte(0)
	}
	str(e.ResourceID)
	str(e.Outcome)
	str(e.MetadataText)
	b.Write(e.PrevHash)
	sum := sha256.Sum256(b.Bytes())
	return sum[:]
}

// ChainResult reports the outcome of verifying the staff audit chain.
type ChainResult struct {
	// Entries is how many chained entries were verified.
	Entries int64
	// HeadSeq is the sequence number of the last one (0 when none).
	HeadSeq int64
	Valid   bool
	// FirstInvalidSeq and Reason say where and why it broke, when it did.
	FirstInvalidSeq int64
	Reason          string
}

// VerifyChain recomputes the whole staff audit chain and compares it with the stored head. q must
// be an operator connection. Entries written before the chain existed are outside it.
func VerifyChain(ctx context.Context, q *sqlcgen.Queries) (ChainResult, error) {
	rows, err := q.ListChainedStaffAudit(ctx)
	if err != nil {
		return ChainResult{}, store.Classify(err, "list chained staff audit")
	}
	head, err := q.GetStaffAuditHead(ctx)
	if err != nil {
		return ChainResult{}, store.Classify(err, "get staff audit head")
	}
	res := ChainResult{Valid: true}
	prev := GenesisHash()
	fail := func(seq int64, why string) (ChainResult, error) {
		res.Valid, res.FirstInvalidSeq, res.Reason = false, seq, why
		return res, nil
	}
	for i, r := range rows {
		want := int64(i) + 1
		seq := int64(0)
		if r.Seq != nil {
			seq = *r.Seq
		}
		switch {
		case seq != want:
			return fail(want, fmt.Sprintf("expected entry %d, found %d: an entry was removed or reordered", want, seq))
		case !bytes.Equal(r.PrevHash, prev):
			return fail(seq, "the entry does not follow the previous one")
		}
		computed := ComputeChainHash(ChainEntry{
			Seq: seq, ID: r.ID, OccurredAt: r.OccurredAt.UTC().Truncate(time.Microsecond), Actor: r.Actor, Action: r.Action,
			TenantID: r.TenantID, ResourceID: r.ResourceID, Outcome: r.Outcome, MetadataText: deref(r.MetadataText), PrevHash: r.PrevHash,
		})
		if !bytes.Equal(computed, r.Hash) {
			return fail(seq, "the entry's hash does not match its contents: it was modified")
		}
		prev = r.Hash
		res.Entries, res.HeadSeq = int64(i)+1, seq
	}
	if head.Seq != res.HeadSeq || !bytes.Equal(head.Hash, headHash(prev, res.HeadSeq)) {
		return fail(head.Seq, "the stored head does not match the chain: entries were removed from its end")
	}
	return res, nil
}

func headHash(last []byte, seq int64) []byte {
	if seq == 0 {
		return GenesisHash()
	}
	return last
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
