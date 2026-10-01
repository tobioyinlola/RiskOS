package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

// Two-person approval of production rule changes (ARCHITECTURE.md §10, §16): a change proposed
// by one principal applies only when a different person approves it.

// propose records c as a pending change and audits it. current is the rule as it stands (zero
// for a new rule).
func (s *Service) propose(ctx context.Context, scope tenancy.Scope, current domain.Rule, c domain.RuleChange) (RuleResult, error) {
	c.Status, c.ProposedBy, c.ProposedAt = domain.ChangePending, scope.Principal.ID, s.clock.Now()
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		if c.Kind == domain.ChangeCreate {
			if err := checkRuleLimit(ctx, s.store); err != nil {
				return err
			}
			taken, err := s.store.CodeExists(ctx, c.Code)
			if err != nil {
				return err
			}
			if taken {
				return domain.ErrRuleCodeTaken
			}
		}
		if err := s.store.InsertChange(ctx, c); err != nil {
			return err
		}
		meta := versionMetadata(c.Code, c.Proposed)
		delete(meta, "version")
		meta["change_id"], meta["kind"] = ids.Format(ids.PrefixRuleChange, c.ID), string(c.Kind)
		if c.BaseVersion != nil {
			meta["base_version"] = *c.BaseVersion
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "rule.change_proposed", ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, c.RuleID),
			Outcome: audit.OutcomeSuccess, Metadata: meta,
		})
	})
	if err != nil {
		return RuleResult{}, err
	}
	if c.Kind != domain.ChangeCreate {
		current.PendingChange = &c.ID
	}
	return RuleResult{Rule: current, Change: &c}, nil
}

func approvalMetadata(meta map[string]any, c *domain.RuleChange) {
	if c == nil {
		return
	}
	meta["change_id"] = ids.Format(ids.PrefixRuleChange, c.ID)
	meta["proposed_by"] = c.ProposedBy
}

// ApproveChange applies a pending change as the rule's next version. Only a person (a console
// user, never an API key) holding rules:approve may approve, and never their own proposal.
func (s *Service) ApproveChange(ctx context.Context, id uuid.UUID) (domain.RuleChange, error) {
	if err := authz.Require(ctx, authz.RulesApprove); err != nil {
		return domain.RuleChange{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.RuleChange{}, err
	}
	if scope.Principal.Type != tenancy.PrincipalUser {
		return domain.RuleChange{}, domain.ErrApproverNotPerson
	}
	var c domain.RuleChange
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if c, err = s.store.GetChange(ctx, id, true); err != nil {
			return err
		}
		if c.Status != domain.ChangePending {
			return domain.ErrChangeDecided
		}
		if c.ProposedBy == scope.Principal.ID {
			return domain.ErrSelfApproval
		}
		now := s.clock.Now()
		v := c.Proposed
		// The version is the proposer's work; the approval is recorded with it in the audit log.
		v.CreatedBy, v.CreatedAt = c.ProposedBy, now
		if c.Kind == domain.ChangeCreate {
			v.Version = 1
			if err := checkRuleLimit(ctx, s.store); err != nil {
				return err
			}
			if err := s.store.InsertRule(ctx, c.RuleID, c.Code, v); err != nil {
				return err
			}
			meta := versionMetadata(c.Code, v)
			approvalMetadata(meta, &c)
			if err := s.audit.Record(ctx, audit.Entry{
				Action: "rule.created", ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, c.RuleID),
				Outcome: audit.OutcomeSuccess, Metadata: meta,
			}); err != nil {
				return err
			}
		} else {
			r, err := s.store.GetRule(ctx, c.RuleID)
			if err != nil {
				return err
			}
			if c.BaseVersion == nil || r.Current.Version != *c.BaseVersion {
				return domain.ErrChangeStale
			}
			v.Version = r.Current.Version + 1
			if err := s.applyVersion(ctx, r, v, c.Kind, &c); err != nil {
				return err
			}
		}
		if err := s.store.DecideChange(ctx, id, domain.ChangeApproved, scope.Principal.ID, now, nil, &v.Version); err != nil {
			return err
		}
		c.Status, c.DecidedBy, c.DecidedAt, c.AppliedVersion = domain.ChangeApproved, &scope.Principal.ID, &now, &v.Version
		return s.audit.Record(ctx, audit.Entry{
			Action: "rule.change_approved", ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, c.RuleID),
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{
				"change_id": ids.Format(ids.PrefixRuleChange, c.ID), "code": c.Code, "kind": string(c.Kind),
				"proposed_by": c.ProposedBy, "version": v.Version,
			},
		})
	})
	if err != nil {
		return domain.RuleChange{}, err
	}
	s.config.invalidate(ctx)
	return c, nil
}

// RejectChange rejects a pending change; nothing changes on the rule. Holders of rules:approve
// may reject any change; its proposer, holding rules:write, may withdraw it.
func (s *Service) RejectChange(ctx context.Context, id uuid.UUID, reason string) (domain.RuleChange, error) {
	if err := authz.RequireAny(ctx, authz.RulesApprove, authz.RulesWrite); err != nil {
		return domain.RuleChange{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.RuleChange{}, err
	}
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 500 {
		return domain.RuleChange{}, fmt.Errorf("%w: reason must be at most 500 characters", domain.ErrRuleInvalid)
	}
	var c domain.RuleChange
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if c, err = s.store.GetChange(ctx, id, true); err != nil {
			return err
		}
		withdrawn := c.ProposedBy == scope.Principal.ID
		if !withdrawn && !scope.Principal.Can(authz.RulesApprove) {
			return authz.ErrForbidden
		}
		if c.Status != domain.ChangePending {
			return domain.ErrChangeDecided
		}
		now := s.clock.Now()
		var r *string
		if reason != "" {
			r = &reason
		}
		if err := s.store.DecideChange(ctx, id, domain.ChangeRejected, scope.Principal.ID, now, r, nil); err != nil {
			return err
		}
		c.Status, c.DecidedBy, c.DecidedAt, c.Reason = domain.ChangeRejected, &scope.Principal.ID, &now, r
		// The reason is free text, kept with the change, not in the audit log.
		return s.audit.Record(ctx, audit.Entry{
			Action: "rule.change_rejected", ResourceType: "rule", ResourceID: ids.Format(ids.PrefixRule, c.RuleID),
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{
				"change_id": ids.Format(ids.PrefixRuleChange, c.ID), "code": c.Code, "kind": string(c.Kind),
				"proposed_by": c.ProposedBy, "withdrawn": withdrawn, "reason_given": r != nil,
			},
		})
	})
	if err != nil {
		return domain.RuleChange{}, err
	}
	return c, nil
}

// ListChanges returns the newest rule changes, optionally only those in status. Requires
// rules:read.
func (s *Service) ListChanges(ctx context.Context, status *domain.ChangeStatus) ([]domain.RuleChange, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return nil, err
	}
	var out []domain.RuleChange
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.store.ListChanges(ctx, status)
		return err
	})
	return out, err
}

// GetChange returns a rule change. Requires rules:read.
func (s *Service) GetChange(ctx context.Context, id uuid.UUID) (domain.RuleChange, error) {
	if err := authz.Require(ctx, authz.RulesRead); err != nil {
		return domain.RuleChange{}, err
	}
	var c domain.RuleChange
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		c, err = s.store.GetChange(ctx, id, false)
		return err
	})
	if errors.Is(err, domain.ErrChangeNotFound) {
		return domain.RuleChange{}, err
	}
	return c, err
}
