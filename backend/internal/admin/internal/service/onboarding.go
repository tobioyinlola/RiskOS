package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

// Onboarding reviews (docs/ADMIN.md#onboarding-reviews): operations admins read self-serve
// workspaces' compliance onboarding and approve it, which opens production, or send it back with
// a note. Every read and decision is in the staff audit log and the tenant's.

func requireOperations(sess Session) error {
	if sess.Role != string(authz.RoleOperationsAdmin) {
		return fmt.Errorf("%w: onboarding reviews need the operations_admin role", ErrForbidden)
	}
	return nil
}

func mapOnboardingErr(err error) error {
	switch {
	case errors.Is(err, tenants.ErrNoOnboarding):
		return ErrNotFound
	case errors.Is(err, tenants.ErrOnboardingState):
		return fmt.Errorf("%w: only a submitted onboarding can be decided", ErrConflict)
	case errors.Is(err, tenants.ErrInvalidInput):
		return fmt.Errorf("%w: %s", ErrInvalid, strings.TrimPrefix(err.Error(), tenants.ErrInvalidInput.Error()+": "))
	default:
		return err
	}
}

// OnboardingQueue lists self-serve workspaces' onboarding: identity, status and dates.
func (s *Service) OnboardingQueue(ctx context.Context, sess Session) ([]store.OnboardingRow, error) {
	if err := requireOperations(sess); err != nil {
		return nil, err
	}
	return s.store.OnboardingQueue(ctx)
}

// onboardingTenant returns the tenant scope a staff member acts in for an onboarding review.
func (s *Service) onboardingTenant(ctx context.Context, sess Session, tenantID uuid.UUID) (context.Context, error) {
	if err := requireOperations(sess); err != nil {
		return nil, err
	}
	return s.staffTenant(ctx, tenantID, tenancy.EnvironmentTest, sess.Ref(), nil)
}

// Onboarding returns one workspace's onboarding answers, recorded in both audit logs.
func (s *Service) Onboarding(ctx context.Context, sess Session, tenantID uuid.UUID) (tenants.Onboarding, error) {
	tctx, err := s.onboardingTenant(ctx, sess, tenantID)
	if err != nil {
		return tenants.Onboarding{}, err
	}
	var out tenants.Onboarding
	err = s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		if out, err = s.tenants.StaffOnboarding(ctx); err != nil {
			return mapOnboardingErr(err)
		}
		return s.record(ctx, q, sess.Ref(), "onboarding.viewed", &tenantID, ids.Format(ids.PrefixTenant, tenantID), "success", nil)
	})
	return out, err
}

// ReviewOnboarding approves (opening production) or rejects (note required) a submitted
// onboarding, recorded in both audit logs in one transaction.
func (s *Service) ReviewOnboarding(ctx context.Context, sess Session, tenantID uuid.UUID, approve bool, note string) (tenants.Onboarding, error) {
	tctx, err := s.onboardingTenant(ctx, sess, tenantID)
	if err != nil {
		return tenants.Onboarding{}, err
	}
	action := "onboarding.approved"
	if !approve {
		action = "onboarding.rejected"
	}
	var out tenants.Onboarding
	err = s.runner.Run(tctx, func(ctx context.Context) error {
		q, err := s.store.Scoped(ctx)
		if err != nil {
			return err
		}
		if out, err = s.tenants.ReviewOnboarding(ctx, approve, note, s.clock.Now()); err != nil {
			return mapOnboardingErr(err)
		}
		return s.record(ctx, q, sess.Ref(), action, &tenantID, ids.Format(ids.PrefixTenant, tenantID), "success",
			map[string]any{"note": note})
	})
	return out, err
}
