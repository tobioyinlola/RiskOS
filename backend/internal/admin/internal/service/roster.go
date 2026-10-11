package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
)

// StaffRosterRow is a staff account as the roster shows it. Never a credential.
type StaffRosterRow = sqlcgen.StaffRosterRow

func requireSecurity(sess Session) error {
	if sess.Role != string(authz.RoleSecurityAdmin) {
		return fmt.Errorf("%w: the staff roster needs the security_admin role", ErrForbidden)
	}
	return nil
}

// Roster lists staff accounts: role, status, passkeys, last sign-in and whether an enrollment
// link is outstanding. Security administrators only.
func (s *Service) Roster(ctx context.Context, sess Session) ([]StaffRosterRow, error) {
	if err := requireSecurity(sess); err != nil {
		return nil, err
	}
	out, err := s.store.Q().StaffRoster(ctx)
	return out, store.Classify(err, "list staff roster")
}

// SetStaffAccess disables or enables another staff member's account. Security administrators
// only, never one's own account, and never the last active security administrator, so the
// console cannot lock everyone out. Disabling ends the person's sessions at once. Recorded in
// the staff audit log. Creating accounts and issuing enrollment links stay in the operator CLI.
func (s *Service) SetStaffAccess(ctx context.Context, sess Session, id uuid.UUID, active bool) (StaffRosterRow, error) {
	if err := requireSecurity(sess); err != nil {
		return StaffRosterRow{}, err
	}
	if id == sess.StaffID {
		return StaffRosterRow{}, fmt.Errorf("%w: you cannot change your own access", ErrForbidden)
	}
	status, action := "disabled", "staff.disabled"
	if active {
		status, action = "active", "staff.enabled"
	}
	err := s.store.Tx(ctx, func(q *sqlcgen.Queries) error {
		target, err := q.GetStaff(ctx, id)
		if err := store.Classify(err, "get staff"); err != nil {
			return err
		}
		if !active && target.Role == string(authz.RoleSecurityAdmin) && target.Status == "active" {
			admins, err := q.LockActiveSecurityAdmins(ctx)
			if err != nil {
				return store.Classify(err, "lock security administrators")
			}
			if len(admins) <= 1 {
				return fmt.Errorf("%w: this is the last active security administrator", ErrConflict)
			}
		}
		if _, err := q.SetStaffStatus(ctx, sqlcgen.SetStaffStatusParams{ID: id, Status: status}); err != nil {
			return store.Classify(err, "set staff status")
		}
		if !active {
			if err := q.RevokeStaffSessions(ctx, sqlcgen.RevokeStaffSessionsParams{StaffID: id, Now: s.clock.Now()}); err != nil {
				return store.Classify(err, "revoke staff sessions")
			}
		}
		return s.record(ctx, q, sess.Ref(), action, nil, staffRef(id), "success",
			map[string]any{"email": target.Email, "role": target.Role})
	})
	if errors.Is(err, store.ErrNotFound) {
		return StaffRosterRow{}, ErrNotFound
	}
	if err != nil {
		return StaffRosterRow{}, err
	}
	rows, err := s.store.Q().StaffRoster(ctx)
	if err != nil {
		return StaffRosterRow{}, store.Classify(err, "list staff roster")
	}
	for _, r := range rows {
		if r.ID == id {
			return r, nil
		}
	}
	return StaffRosterRow{}, ErrNotFound
}
