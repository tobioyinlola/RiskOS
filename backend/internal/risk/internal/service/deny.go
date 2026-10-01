package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

// DenyInput describes a new deny-list entry.
type DenyInput struct {
	Kind      domain.DenyKind
	Value     string
	Reason    string
	ExpiresAt *time.Time
}

// DenyListing is an environment's deny list as the console shows it.
type DenyListing struct {
	Entries []domain.DenyEntry
	Total   int64
}

// ListDeny returns the newest deny-list entries (up to 1,000), optionally of one kind.
// Requires rules:read or risk:configure.
func (s *Service) ListDeny(ctx context.Context, kind *domain.DenyKind) (DenyListing, error) {
	if err := authz.RequireAny(ctx, authz.RulesRead, authz.RiskConfigure); err != nil {
		return DenyListing{}, err
	}
	var out DenyListing
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		if out.Entries, err = s.store.ListDeny(ctx, kind); err != nil {
			return err
		}
		out.Total, err = s.store.CountDeny(ctx)
		return err
	})
	return out, err
}

// AddDeny puts an identifier on the deny list: from now on (within the configuration cache's few
// seconds) events carrying it are blocked. Requires risk:configure; audited.
func (s *Service) AddDeny(ctx context.Context, in DenyInput) (domain.DenyEntry, error) {
	if err := authz.Require(ctx, authz.RiskConfigure); err != nil {
		return domain.DenyEntry{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return domain.DenyEntry{}, err
	}
	value, err := domain.NormalizeDenyValue(in.Kind, in.Value)
	if err != nil {
		return domain.DenyEntry{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > 200 {
		return domain.DenyEntry{}, fmt.Errorf("%w: reason must be at most 200 characters", domain.ErrDenyInvalid)
	}
	now := s.clock.Now()
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return domain.DenyEntry{}, fmt.Errorf("%w: expires_at must be in the future", domain.ErrDenyInvalid)
	}
	e := domain.DenyEntry{
		ID: ids.NewV7(), Kind: in.Kind, Value: value, Reason: reason, ExpiresAt: in.ExpiresAt,
		CreatedAt: now, CreatedBy: scope.Principal.ID,
	}
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		n, err := s.store.CountDeny(ctx)
		if err != nil {
			return err
		}
		if n >= domain.MaxDenyEntries {
			return fmt.Errorf("%w: an environment can list at most %d identifiers; remove expired or unneeded ones",
				domain.ErrDenyLimit, domain.MaxDenyEntries)
		}
		if err := s.store.InsertDeny(ctx, e); err != nil {
			return err
		}
		meta := map[string]any{"kind": string(e.Kind), "value": e.Value}
		if e.ExpiresAt != nil {
			meta["expires_at"] = e.ExpiresAt.UTC().Format(time.RFC3339)
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "deny_list.added", ResourceType: "deny_list_entry", ResourceID: ids.Format(ids.PrefixDenyEntry, e.ID),
			Outcome: audit.OutcomeSuccess, Metadata: meta,
		})
	})
	if err != nil {
		return domain.DenyEntry{}, err
	}
	s.config.invalidate(ctx)
	return e, nil
}

// RemoveDeny takes an entry off the deny list. Requires risk:configure; audited.
func (s *Service) RemoveDeny(ctx context.Context, id uuid.UUID) error {
	if err := authz.Require(ctx, authz.RiskConfigure); err != nil {
		return err
	}
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		kind, value, err := s.store.DeleteDeny(ctx, id)
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: "deny_list.removed", ResourceType: "deny_list_entry", ResourceID: ids.Format(ids.PrefixDenyEntry, id),
			Outcome: audit.OutcomeSuccess, Metadata: map[string]any{"kind": string(kind), "value": value},
		})
	})
	if err != nil {
		return err
	}
	s.config.invalidate(ctx)
	return nil
}
