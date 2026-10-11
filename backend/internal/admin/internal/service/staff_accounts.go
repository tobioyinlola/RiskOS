package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// EnrollmentEmailArgs is the job that emails a new staff member their enrollment link. It carries
// identifiers only: the token is derived again by the worker, so it is never stored in a job.
type EnrollmentEmailArgs struct {
	StaffID  uuid.UUID `json:"staff_id"`
	IssuedAt int64     `json:"issued_at"`
}

// Kind implements jobs.Args.
func (EnrollmentEmailArgs) Kind() string { return "admin_enrollment_email" }

var _ jobs.Args = EnrollmentEmailArgs{}

// StaffAccountRequest is a request to create a staff account, with display names.
type StaffAccountRequest struct {
	sqlcgen.StaffAccountRequest
	RequesterName string
	DeciderName   string
}

func accountRequestRef(id uuid.UUID) string { return ids.Format(ids.PrefixStaffAccountRequest, id) }

func (s *Service) enrichAccount(r sqlcgen.StaffAccountRequest, sn map[uuid.UUID]string) StaffAccountRequest {
	out := StaffAccountRequest{StaffAccountRequest: r, RequesterName: sn[r.RequestedBy]}
	if r.DecidedBy != nil {
		out.DeciderName = sn[*r.DecidedBy]
	}
	return out
}

// enrollTokenFor derives a new staff member's enrollment token from the console secret, their ID
// and when it was issued, so the job can email it without it being stored anywhere readable.
func enrollTokenFor(secret []byte, staffID uuid.UUID, issuedAt int64) string {
	k := hmac.New(sha256.New, secret)
	k.Write([]byte("riskos-staff-enroll-key"))
	m := hmac.New(sha256.New, k.Sum(nil))
	m.Write(staffID[:])
	var at [8]byte
	binary.BigEndian.PutUint64(at[:], uint64(issuedAt)) //nolint:gosec // a Unix time
	m.Write(at[:])
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Service) accountNames(ctx context.Context) (map[uuid.UUID]string, error) {
	staff, err := s.store.Q().ListStaff(ctx)
	if err != nil {
		return nil, store.Classify(err, "list staff")
	}
	sn := make(map[uuid.UUID]string, len(staff))
	for _, st := range staff {
		sn[st.ID] = st.Name
	}
	return sn, nil
}

// RequestStaffAccount asks for a staff account to be created. A different security administrator
// approves, and the enrollment link goes only to the new person's mailbox. Security
// administrators only.
func (s *Service) RequestStaffAccount(ctx context.Context, sess Session, email, name, role, reason string) (StaffAccountRequest, error) {
	if err := requireSecurity(sess); err != nil {
		return StaffAccountRequest{}, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		return StaffAccountRequest{}, fmt.Errorf("%w: email is not a valid address", ErrInvalid)
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return StaffAccountRequest{}, fmt.Errorf("%w: name must be 1 to 100 characters", ErrInvalid)
	}
	if role != "operations_admin" && role != "security_admin" {
		return StaffAccountRequest{}, fmt.Errorf("%w: role must be operations_admin or security_admin", ErrInvalid)
	}
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 10 || n > 500 {
		return StaffAccountRequest{}, fmt.Errorf("%w: give a reason of 10 to 500 characters", ErrInvalid)
	}
	if !s.cfg.EmailEnabled {
		return StaffAccountRequest{}, fmt.Errorf("%w: email delivery is not configured, so enrollment links cannot be sent; use the operator CLI", ErrConflict)
	}
	id := ids.NewV7()
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		if _, err := q.StaffByEmail(ctx, email); err == nil {
			return fmt.Errorf("%w: a staff member with this email exists", ErrConflict)
		}
		if err := q.InsertStaffAccountRequest(ctx, sqlcgen.InsertStaffAccountRequestParams{
			ID: id, Email: email, Name: name, Role: role, Reason: reason, RequestedBy: sess.StaffID, Now: s.clock.Now(),
		}); err != nil {
			err = store.Classify(err, "insert staff account request")
			if errors.Is(err, store.ErrDuplicate) {
				return fmt.Errorf("%w: a request for this address is already waiting for a decision", ErrConflict)
			}
			return err
		}
		return s.record(ctx, q, sess.Ref(), "staff_account.requested", nil, accountRequestRef(id), "success",
			map[string]any{"email": email, "role": role, "reason": reason})
	})
	if err != nil {
		return StaffAccountRequest{}, err
	}
	return s.getAccountRequest(ctx, id)
}

func (s *Service) getAccountRequest(ctx context.Context, id uuid.UUID) (StaffAccountRequest, error) {
	r, err := s.store.Q().GetStaffAccountRequest(ctx, id)
	if err := store.Classify(err, "get staff account request"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return StaffAccountRequest{}, ErrNotFound
		}
		return StaffAccountRequest{}, err
	}
	sn, err := s.accountNames(ctx)
	if err != nil {
		return StaffAccountRequest{}, err
	}
	return s.enrichAccount(r, sn), nil
}

// ListStaffAccountRequests returns recent requests, newest first. Security administrators only.
func (s *Service) ListStaffAccountRequests(ctx context.Context, sess Session) ([]StaffAccountRequest, error) {
	if err := requireSecurity(sess); err != nil {
		return nil, err
	}
	rows, err := s.store.Q().ListStaffAccountRequests(ctx)
	if err != nil {
		return nil, store.Classify(err, "list staff account requests")
	}
	sn, err := s.accountNames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StaffAccountRequest, len(rows))
	for i, r := range rows {
		out[i] = s.enrichAccount(r, sn)
	}
	return out, nil
}

// DecideStaffAccount approves or rejects another security administrator's request. Approval
// creates the account and queues the email with its enrollment link, in one transaction; nobody
// is shown the link.
func (s *Service) DecideStaffAccount(ctx context.Context, sess Session, id uuid.UUID, approve bool, note string) (StaffAccountRequest, error) {
	if err := requireSecurity(sess); err != nil {
		return StaffAccountRequest{}, err
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 500 {
		return StaffAccountRequest{}, fmt.Errorf("%w: a note is at most 500 characters", ErrInvalid)
	}
	if approve && (!s.cfg.EmailEnabled || s.cfg.Jobs == nil) {
		return StaffAccountRequest{}, fmt.Errorf("%w: email delivery is not configured, so the enrollment link cannot be sent; use the operator CLI", ErrConflict)
	}
	err := s.store.TxWith(ctx, func(tx store.Tx, q *sqlcgen.Queries) error {
		cur, err := q.GetStaffAccountRequestForUpdate(ctx, id)
		if err := store.Classify(err, "lock staff account request"); err != nil {
			return err
		}
		if cur.Status != "pending" {
			return fmt.Errorf("%w: the request is %s", ErrConflict, cur.Status)
		}
		if cur.RequestedBy == sess.StaffID {
			return fmt.Errorf("%w: another security administrator must decide on your request", ErrForbidden)
		}
		now := s.clock.Now()
		var noteArg *string
		if note != "" {
			noteArg = &note
		}
		if !approve {
			if err := q.DecideStaffAccountRequest(ctx, sqlcgen.DecideStaffAccountRequestParams{
				ID: id, Status: "rejected", DecidedBy: &sess.StaffID, Now: now, DecisionNote: noteArg,
			}); err != nil {
				return store.Classify(err, "reject staff account request")
			}
			return s.record(ctx, q, sess.Ref(), "staff_account.rejected", nil, accountRequestRef(id), "success",
				map[string]any{"email": cur.Email, "note": note})
		}
		staffID := ids.NewV7()
		issued := now.Unix()
		hash := hashToken(enrollTokenFor(s.cfg.Secret, staffID, issued))
		exp := now.Add(s.cfg.EnrollTTL)
		if err := store.Classify(q.InsertStaff(ctx, sqlcgen.InsertStaffParams{
			ID: staffID, Email: cur.Email, Name: cur.Name, Role: cur.Role, EnrollTokenHash: hash, EnrollExpiresAt: &exp,
			Now: now, CreatedBy: "console:" + sess.Ref(),
		}), "insert staff"); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				return fmt.Errorf("%w: a staff member with this email exists", ErrConflict)
			}
			return err
		}
		if err := q.DecideStaffAccountRequest(ctx, sqlcgen.DecideStaffAccountRequestParams{
			ID: id, Status: "approved", DecidedBy: &sess.StaffID, Now: now, DecisionNote: noteArg, StaffID: &staffID,
		}); err != nil {
			return store.Classify(err, "approve staff account request")
		}
		if err := s.cfg.Jobs.EnqueueTx(ctx, tx, EnrollmentEmailArgs{StaffID: staffID, IssuedAt: issued}); err != nil {
			return err
		}
		return s.record(ctx, q, sess.Ref(), "staff_account.approved", nil, accountRequestRef(id), "success",
			map[string]any{"email": cur.Email, "role": cur.Role, "staff": staffRef(staffID), "note": note})
	})
	if errors.Is(err, store.ErrNotFound) {
		return StaffAccountRequest{}, ErrNotFound
	}
	if err != nil {
		return StaffAccountRequest{}, err
	}
	return s.getAccountRequest(ctx, id)
}

// WithdrawStaffAccount lets the requester take back their own pending request.
func (s *Service) WithdrawStaffAccount(ctx context.Context, sess Session, id uuid.UUID) (StaffAccountRequest, error) {
	if err := requireSecurity(sess); err != nil {
		return StaffAccountRequest{}, err
	}
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		cur, err := q.GetStaffAccountRequestForUpdate(ctx, id)
		if err := store.Classify(err, "lock staff account request"); err != nil {
			return err
		}
		if cur.RequestedBy != sess.StaffID {
			return fmt.Errorf("%w: only the requester can withdraw a request", ErrForbidden)
		}
		n, err := q.WithdrawStaffAccountRequest(ctx, sqlcgen.WithdrawStaffAccountRequestParams{
			ID: id, StaffID: sess.StaffID, Now: s.clock.Now(),
		})
		if err != nil {
			return store.Classify(err, "withdraw staff account request")
		}
		if n == 0 {
			return fmt.Errorf("%w: the request is no longer pending", ErrConflict)
		}
		return s.record(ctx, q, sess.Ref(), "staff_account.withdrawn", nil, accountRequestRef(id), "success",
			map[string]any{"email": cur.Email})
	})
	if errors.Is(err, store.ErrNotFound) {
		return StaffAccountRequest{}, ErrNotFound
	}
	if err != nil {
		return StaffAccountRequest{}, err
	}
	return s.getAccountRequest(ctx, id)
}

// PendingEnrollment is what the email job needs to send a staff member their link.
type PendingEnrollment struct {
	Email     string
	Name      string
	Role      string
	Token     string
	ExpiresAt time.Time
}

// EnrollmentEmail returns the link to email for a job, or nil when it should not be sent (the
// account is disabled, the link was used, expired or replaced). The token is derived with the
// current console secret or, during a rotation, the previous one, and must match the stored hash.
func (s *Service) EnrollmentEmail(ctx context.Context, a EnrollmentEmailArgs) (*PendingEnrollment, error) {
	st, err := s.store.Q().GetStaff(ctx, a.StaffID)
	if errors.Is(store.Classify(err, "get staff"), store.ErrNotFound) {
		return nil, nil //nolint:nilnil // nothing to send is a normal outcome
	}
	if err != nil {
		return nil, store.Classify(err, "get staff")
	}
	if st.Status != "active" || st.EnrollTokenHash == nil || st.EnrollExpiresAt == nil || !s.clock.Now().Before(*st.EnrollExpiresAt) {
		return nil, nil //nolint:nilnil // see above
	}
	for _, secret := range [][]byte{s.cfg.Secret, s.cfg.SecretPrevious} {
		if len(secret) == 0 {
			continue
		}
		token := enrollTokenFor(secret, a.StaffID, a.IssuedAt)
		if hmac.Equal(hashToken(token), st.EnrollTokenHash) {
			return &PendingEnrollment{Email: st.Email, Name: st.Name, Role: st.Role, Token: token, ExpiresAt: *st.EnrollExpiresAt}, nil
		}
	}
	return nil, nil //nolint:nilnil // a replaced link is not re-sent
}
