// Package domain holds the audit log's types and its hash-chain rules. It has no I/O, so the
// writer (service) and the verifier share exactly the same canonical encoding.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/google/uuid"
)

// Outcome of an audited action.
type Outcome string

// Outcomes.
const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
)

// Entry is what a module records. Actor, tenant, environment, time and request ID are taken from
// the context by the audit service, never supplied by the caller.
type Entry struct {
	Action       string // "<resource>.<verb>", e.g. "api_key.created"
	ResourceType string
	ResourceID   string
	Outcome      Outcome
	// Metadata is small, non-sensitive context. Never put secrets or personal data here.
	Metadata map[string]any
	// TenantLevel records the entry without an environment (visible from every environment).
	TenantLevel bool
}

// Record is a stored audit entry.
type Record struct {
	TenantID      uuid.UUID
	Seq           int64
	ID            uuid.UUID
	EnvironmentID *uuid.UUID
	OccurredAt    time.Time
	ActorType     string
	ActorID       string
	Action        string
	ResourceType  string
	ResourceID    string
	Outcome       Outcome
	RequestID     string
	// Metadata is canonical JSON (sorted keys, no insignificant whitespace), stored verbatim
	// so the hash can be recomputed byte for byte.
	Metadata []byte
	PrevHash []byte
	Hash     []byte
}

// GenesisHash is the prev_hash of a tenant's first entry.
func GenesisHash() []byte { return make([]byte, sha256.Size) }

// hashDomain separates this hash from any other use of SHA-256 and versions the encoding.
const hashDomain = "riskos.audit.v1"

// ComputeHash returns SHA-256 over a length-prefixed, fixed-order encoding of every field of r
// except Hash, ending with PrevHash. Changing any field, the order, or the previous entry
// changes the result.
func ComputeHash(r Record) []byte {
	var b bytes.Buffer
	str := func(s string) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(s))) //nolint:gosec // lengths are bounded well below 4 GiB
		b.WriteString(s)
	}
	b.WriteString(hashDomain)
	b.Write(r.TenantID[:])
	_ = binary.Write(&b, binary.BigEndian, r.Seq)
	b.Write(r.ID[:])
	if r.EnvironmentID != nil {
		b.WriteByte(1)
		b.Write(r.EnvironmentID[:])
	} else {
		b.WriteByte(0)
	}
	_ = binary.Write(&b, binary.BigEndian, r.OccurredAt.UnixMicro())
	str(r.ActorType)
	str(r.ActorID)
	str(r.Action)
	str(r.ResourceType)
	str(r.ResourceID)
	str(string(r.Outcome))
	str(r.RequestID)
	str(string(r.Metadata))
	b.Write(r.PrevHash)
	sum := sha256.Sum256(b.Bytes())
	return sum[:]
}

// VerifyResult reports the outcome of a chain verification.
type VerifyResult struct {
	Entries int64
	HeadSeq int64
	Valid   bool
	// FirstInvalidSeq and Reason are set when Valid is false.
	FirstInvalidSeq int64
	Reason          string
}

// Page selects a page of a newest-first listing.
type Page struct {
	Limit  int
	Before *uuid.UUID
}
