package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// Retention across jurisdictions (ADR-0007, phase 5; RW-GOV-10): a record is kept for the
// longest period any country the institution operates in requires, and a deletion refused for
// it is logged with the rule that refused it.

// Record types the retention rules speak of (record.type in the catalog).
var recordTypes = []string{"customer_file", "transaction", "report", "audit_log", "other"} //nolint:gochecknoglobals // fixed list

// retentionParams are the settings that hold a minimum retention, and the record types each
// covers: every record (GOV-08 and its country versions, RW-GOV-10), or audit logs only
// (GH-SEC-11's six years).
var retentionParams = map[string][]string{ //nolint:gochecknoglobals // fixed table
	"retain_for":  recordTypes,
	"retain_logs": {"audit_log"},
}

// RetentionRule is one rule's minimum retention for some record types, in one country, and the
// rule's mode in the environment (counted whatever it is).
type RetentionRule struct {
	Jurisdiction, Rule, Name, Setting, Mode string
	Period                                  time.Duration
	RecordTypes                             []string
}

// RetentionPeriod is the retention in force for a record type: the longest of the rules that
// apply (Rule, the winner), and every rule considered.
type RetentionPeriod struct {
	RecordType string
	Period     time.Duration
	Winner     *RetentionRule
	Considered []RetentionRule
}

// retentionRules lists the retention rules of every country the tenant has activated (drafts
// excepted: nothing was recorded under them), with their settings in the caller's environment.
// A rule switched off still counts: a minimum the law sets is not lifted by a setting, and a
// governance preset is switched off without a second person, so counting only rules switched on
// would let one person make every deletion allowed. It must run inside the runner.
func (s *Service) retentionRules(ctx context.Context) ([]RetentionRule, error) {
	scope, err := readScope(ctx)
	if err != nil {
		return nil, err
	}
	acts, err := s.store.Activations(ctx, scope.TenantID)
	if err != nil {
		return nil, err
	}
	var out []RetentionRule
	seen := map[string]bool{}
	for i := range acts {
		a := &acts[i]
		if a.Status == "draft" || seen[a.Jurisdiction] {
			continue
		}
		seen[a.Jurisdiction] = true
		lib, err := s.load(ctx, scope, a.Jurisdiction)
		if err != nil {
			return nil, err
		}
		for j := range lib.pack.Rules {
			def := &lib.pack.Rules[j]
			if ok := applies(def, lib.profile); ok != nil && !*ok {
				continue
			}
			var v *store.RuleVersion
			if c, ok := lib.configured[def.ID]; ok {
				v = &c
			}
			cfg := configOf(def, v, lib.profile)
			for _, prm := range def.Parameters {
				types, ok := retentionParams[prm.Name]
				if !ok || prm.Type != pack.ParamDuration {
					continue
				}
				val, set := cfg.Settings[prm.Name]
				if !set && prm.Default != nil {
					val, set = *prm.Default, true
				}
				if !set {
					continue
				}
				out = append(out, RetentionRule{
					Jurisdiction: lib.pack.Jurisdiction, Rule: def.ID, Name: def.Text.Name, Setting: prm.Name,
					Mode: cfg.Mode, Period: time.Duration(val) * time.Second, RecordTypes: types,
				})
			}
		}
	}
	return out, nil
}

// periodsOf computes the retention in force per record type: the longest period wins, the
// first rule listed on a tie.
func periodsOf(rules []RetentionRule) []RetentionPeriod {
	out := make([]RetentionPeriod, 0, len(recordTypes))
	for _, rt := range recordTypes {
		p := RetentionPeriod{RecordType: rt, Considered: []RetentionRule{}}
		for i := range rules {
			r := rules[i]
			if !slices.Contains(r.RecordTypes, rt) {
				continue
			}
			p.Considered = append(p.Considered, r)
			if p.Winner == nil || r.Period > p.Period {
				p.Period, p.Winner = r.Period, &r
			}
		}
		out = append(out, p)
	}
	return out
}

// Retention returns the retention in force for each record type across the countries the
// tenant operates in. Requires compliance:read.
func (s *Service) Retention(ctx context.Context) ([]RetentionPeriod, error) {
	if _, err := readScope(ctx); err != nil {
		return nil, err
	}
	var out []RetentionPeriod
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		rules, err := s.retentionRules(ctx)
		if err != nil {
			return err
		}
		out = periodsOf(rules)
		return nil
	})
	return out, err
}

// DeletionRequest asks whether a record may be deleted now: its type, the institution's
// reference for it, when its retention started (the relationship's end for a customer file,
// the transaction or report date otherwise), and why it is to be deleted.
type DeletionRequest struct {
	RecordType, RecordRef string
	Since                 time.Time
	// Reason is data_subject_request or disposal_policy.
	Reason string
}

// DeletionVerdict is the answer: allowed, or refused until RetainUntil under the winning rule.
type DeletionVerdict struct {
	Allowed     bool
	RetainUntil time.Time
	Period      RetentionPeriod
}

// CheckDeletion decides whether a record may be deleted: refused while it is younger than the
// longest retention any country the institution operates in requires, whatever asked for the
// deletion (RW-GOV-10: the longer legal duty wins over a data-protection request or a disposal
// policy). Every answer is audited, a refusal with the rule and country that required it.
// Requires compliance:read; RiskOS holds no customer records and deletes nothing itself.
func (s *Service) CheckDeletion(ctx context.Context, in DeletionRequest, now time.Time) (DeletionVerdict, error) {
	if _, err := readScope(ctx); err != nil {
		return DeletionVerdict{}, err
	}
	in.RecordRef = strings.TrimSpace(in.RecordRef)
	switch {
	case !slices.Contains(recordTypes, in.RecordType):
		return DeletionVerdict{}, fmt.Errorf("%w: record_type must be one of %s", ErrInvalidInput, strings.Join(recordTypes, ", "))
	case in.RecordRef == "" || len([]rune(in.RecordRef)) > 128:
		return DeletionVerdict{}, fmt.Errorf("%w: a record_ref of 1 to 128 characters is required", ErrInvalidInput)
	case in.Reason != "data_subject_request" && in.Reason != "disposal_policy":
		return DeletionVerdict{}, fmt.Errorf("%w: reason must be data_subject_request or disposal_policy", ErrInvalidInput)
	case in.Since.IsZero():
		return DeletionVerdict{}, fmt.Errorf("%w: since is required", ErrInvalidInput)
	}
	var out DeletionVerdict
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		rules, err := s.retentionRules(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(recordTypes, func(t string) bool { return t == in.RecordType })
		out.Period = periodsOf(rules)[i]
		out.RetainUntil = in.Since.Add(out.Period.Period)
		out.Allowed = !now.Before(out.RetainUntil)
		action, outcome := "compliance.deletion_allowed", audit.OutcomeSuccess
		meta := map[string]any{
			"record_type": in.RecordType, "reason": in.Reason, "since": in.Since.UTC().Format(time.RFC3339),
			"retain_until": out.RetainUntil.UTC().Format(time.RFC3339),
		}
		if w := out.Period.Winner; w != nil {
			meta["rule"], meta["jurisdiction"], meta["retention_days"] = w.Rule, w.Jurisdiction, int(w.Period.Hours()/24)
		}
		if !out.Allowed {
			action, outcome = "compliance.deletion_denied", audit.OutcomeDenied
		}
		return s.audit.Record(ctx, audit.Entry{
			Action: action, ResourceType: "record", ResourceID: in.RecordType + ":" + in.RecordRef, Outcome: outcome, Metadata: meta,
		})
	})
	return out, err
}
