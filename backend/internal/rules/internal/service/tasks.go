package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
)

// Task statuses.
const (
	TaskOpen      = "open"
	TaskApproved  = "approved"
	TaskFiled     = "filed"
	TaskDone      = "done"
	TaskDismissed = "dismissed"
)

// The rules the workflow enforces itself.
const (
	// Suspicious transaction reports (REG-14) and their completeness (REG-19).
	ruleSTR        = "REG-14"
	ruleSTRCheck   = "REG-19"
	kindSTR        = "report.STR"
	screenDueParam = "screen_within"
	// Accounts without a BVN are closed (REG-16).
	ruleNoBVN        = "REG-16"
	kindCloseAccount = "close_account"
	// Confirmed fraud is acted on fast (GOV-11) and reported to the industry channel (GOV-12).
	ruleFraudResponse  = "GOV-11"
	respondParam       = "respond_within"
	ruleFraudReport    = "GOV-12"
	kindIndustryReport = "report.industry_fraud"

	systemTaskAuthor = "system:compliance"
)

// TrackedTask reports whether RiskOS keeps a control's task as compliance work. The others are
// actions for the customer's own systems (fresh authentication, enhanced MFA...), returned with
// the decision.
func TrackedTask(kind string) bool {
	return strings.HasPrefix(kind, "report.") || slices.Contains([]string{
		"case.open", "edd", "source_of_funds", "source_of_wealth", "senior_approval", "freeze_account", "close_account",
	}, kind)
}

func isReport(kind string) bool { return strings.HasPrefix(kind, "report.") }

// Workflow sources: alerts and cases (other modules).
type (
	// OpenAlert is an alert still open (REG-14 and GOV-11 count from its creation).
	OpenAlert struct {
		ID         uuid.UUID
		Title      string
		SubjectRef string
		Severity   string
		CreatedAt  time.Time
	}
	// AlertSource lists alerts still open since before a time.
	AlertSource interface {
		OpenAlertsBefore(ctx context.Context, before time.Time, limit int) ([]OpenAlert, error)
	}
	// CaseSource returns who a case is about (the subject of its alerts) and cases resolved as
	// confirmed fraud; a missing case is ErrNotFound.
	CaseSource interface {
		CaseSubject(ctx context.Context, id uuid.UUID) (string, error)
		FraudCase(ctx context.Context, id uuid.UUID) (FraudCase, error)
		ConfirmedFraud(ctx context.Context, limit int) ([]FraudCase, error)
	}
	// FraudCase is a resolved case; Confirmed when it was resolved as confirmed fraud.
	FraudCase struct {
		ID         uuid.UUID
		Title      string
		Confirmed  bool
		ResolvedAt time.Time
		Note       string
		SubjectRef string
	}
)

// UseWorkflow connects the alerts and cases the workflow reads.
func (s *Service) UseWorkflow(a AlertSource, c CaseSource) { s.alerts, s.cases = a, c }

// TaskControl is a live control that fired, with its tasks, as the decision job carries it.
type TaskControl struct {
	Rule  string   `json:"rule"`
	Tasks []string `json:"tasks"`
	// Branch is the variant of the control that applied: its tasks may have their own deadlines.
	Branch string `json:"branch,omitempty"`
	// Jurisdiction is the country whose pack judged the event ("" for the fallback).
	Jurisdiction string `json:"jurisdiction,omitempty"`
}

// TasksInput is what a decision requires of the institution.
type TasksInput struct {
	EvaluationID, EventID uuid.UUID
	OccurredAt            time.Time
	SubjectRef            string
	// AccountRef is the event's account; HasBVN what the event said of its BVN, if anything
	// (the account register of REG-16).
	AccountRef string
	HasBVN     *bool
	Amount     int64
	Currency   string
	Controls   []TaskControl
}

// taskDue resolves a task's deadline from its rule's setting in this environment: the task of
// the branch that applied, if it has its own tasks, else the outcome's.
func taskDue(def *pack.Rule, cfg RuleConfig, kind, branch string, from time.Time) *time.Time {
	list := def.Outcome.Tasks
	if i := slices.IndexFunc(def.Branches, func(b pack.Branch) bool { return b.Label == branch }); i >= 0 && len(def.Branches[i].Tasks) > 0 {
		list = def.Branches[i].Tasks
	}
	for _, t := range list {
		if t.Type != kind || t.Due == "" {
			continue
		}
		v, ok := interp.Compile(def, cfg.Settings).Param(t.Due)
		if !ok {
			return nil
		}
		due := from.Add(time.Duration(v) * time.Second)
		return &due
	}
	return nil
}

func (s *Service) recordTask(ctx context.Context, action string, t store.Task) error {
	meta := map[string]any{}
	// No tipping off (GOV-09): what a suspicious transaction report is about stays out of the
	// audit log, which other roles read; the log says that compliance work happened.
	if !t.Sensitive {
		meta["kind"], meta["rule"] = t.Kind, t.RuleID
	}
	return s.audit.Record(ctx, audit.Entry{
		Action: action, ResourceType: "compliance_task", ResourceID: ids.Format(ids.PrefixComplianceTask, t.ID),
		Outcome: audit.OutcomeSuccess, Metadata: meta,
	})
}

// CreateTasks records the compliance work a decision's live controls require, once per
// evaluation, rule and kind (a retried job creates nothing twice). Deadlines come from the
// rules' settings in the environment. Requires the system permission of the decision job.
func (s *Service) CreateTasks(ctx context.Context, in TasksInput, now time.Time) error {
	if err := authz.Require(ctx, authz.ComplianceTasksCreate); err != nil {
		return err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return err
	}
	return s.runner.Run(ctx, func(ctx context.Context) error {
		if in.AccountRef != "" && in.HasBVN != nil {
			var subject *string
			if in.SubjectRef != "" {
				subject = &in.SubjectRef
			}
			if err := s.store.RecordAccount(ctx, in.AccountRef, subject, *in.HasBVN, in.OccurredAt); err != nil {
				return err
			}
		}
		for _, c := range in.Controls {
			_, def, cfg, err := s.ruleIn(ctx, scope, c.Jurisdiction, c.Rule)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			for _, kind := range c.Tasks {
				if !TrackedTask(kind) {
					continue
				}
				t := store.Task{
					ID: ids.NewV7(), Kind: kind, RuleID: c.Rule, EvaluationID: &in.EvaluationID, EventID: &in.EventID,
					Sensitive: kind == kindSTR, Status: TaskOpen, DueAt: taskDue(def, cfg, kind, c.Branch, in.OccurredAt),
					Report: map[string]any{}, CreatedAt: now, CreatedBy: systemTaskAuthor,
				}
				if in.SubjectRef != "" {
					t.SubjectRef = &in.SubjectRef
				}
				if in.AccountRef != "" {
					t.AccountRef = &in.AccountRef
				}
				if isReport(kind) && in.Currency != "" {
					t.Report = map[string]any{"amount": in.Amount, "currency": in.Currency, "occurred_at": in.OccurredAt}
				}
				created, err := s.store.InsertTask(ctx, t)
				if err != nil {
					return err
				}
				if created {
					if err := s.recordTask(ctx, "compliance.task_created", t); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}

// ConfirmSuspicion records that a compliance officer confirmed suspicion on a case: it opens a
// suspicious transaction report with its legal deadline (REG-14's str_due from now). Requires
// compliance:sensitive (GOV-09).
func (s *Service) ConfirmSuspicion(ctx context.Context, caseID uuid.UUID, note string, now time.Time) (store.Task, error) {
	scope, err := person(ctx, authz.ComplianceSensitive)
	if err != nil {
		return store.Task{}, err
	}
	if s.cases == nil {
		return store.Task{}, fmt.Errorf("%w: cases are not connected", ErrInvalidInput)
	}
	var out store.Task
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		subject, err := s.cases.CaseSubject(ctx, caseID)
		if err != nil {
			return err
		}
		_, def, cfg, err := s.rule(ctx, scope, ruleSTR)
		if err != nil {
			return err
		}
		t := store.Task{
			ID: ids.NewV7(), Kind: kindSTR, RuleID: ruleSTR, CaseID: &caseID, Sensitive: true, Status: TaskOpen,
			DueAt: taskDue(def, cfg, kindSTR, "", now), Report: map[string]any{}, CreatedAt: now,
			CreatedBy: scope.Principal.ID,
		}
		if n := strings.TrimSpace(note); n != "" {
			t.Report["narrative"] = n
		}
		if subject != "" {
			t.SubjectRef = &subject
		}
		if _, err := s.store.InsertTask(ctx, t); err != nil {
			return err
		}
		out = t
		return s.recordTask(ctx, "compliance.task_created", t)
	})
	return out, err
}

// Escalation is an alert open beyond a deadline: REG-14's screening deadline, or GOV-11's
// response target for a high or critical alert. The compliance officer must look at it.
type Escalation struct {
	Rule  string
	Alert OpenAlert
	DueAt time.Time
}

// Queue is the compliance work of an environment.
type Queue struct {
	Tasks       []store.Task
	Escalations []Escalation
	// Unreported are confirmed-fraud cases without an industry fraud report (GOV-12).
	Unreported []FraudCase
}

// queueLimit bounds the escalations and unreported cases the queue lists.
const queueLimit = 200

// Queue lists the environment's compliance tasks (suspicious transaction reports only for
// compliance:sensitive); for callers who read alerts, the alerts past REG-14's screening deadline
// and the high or critical ones past GOV-11's response target; and for callers who read cases,
// the confirmed-fraud cases not yet reported (GOV-12). Requires compliance:read.
func (s *Service) Queue(ctx context.Context, status string, now time.Time) (Queue, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return Queue{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return Queue{}, err
	}
	sensitive := authz.Require(ctx, authz.ComplianceSensitive) == nil
	readsAlerts := authz.Require(ctx, authz.AlertsRead) == nil
	readsCases := authz.Require(ctx, authz.CasesRead) == nil
	var out Queue
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if out.Tasks, err = s.store.Tasks(ctx, status, sensitive); err != nil {
			return err
		}
		if status != "" && status != TaskOpen {
			return nil
		}
		if s.alerts != nil && readsAlerts {
			for _, d := range []struct {
				rule, param string
				urgent      bool
			}{{ruleSTR, screenDueParam, false}, {ruleFraudResponse, respondParam, true}} {
				late, err := s.lateAlerts(ctx, scope, d.rule, d.param, d.urgent, now)
				if err != nil {
					return err
				}
				out.Escalations = append(out.Escalations, late...)
			}
		}
		if s.cases != nil && readsCases {
			if out.Unreported, err = s.unreported(ctx, scope); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// lateAlerts returns the alerts still open past the deadline the rule's setting param gives,
// only high and critical ones when urgent. Nothing when the rule is off or does not apply.
func (s *Service) lateAlerts(ctx context.Context, scope tenancy.Scope, rule, param string, urgent bool, now time.Time) ([]Escalation, error) {
	_, def, cfg, err := s.rule(ctx, scope, rule)
	if errors.Is(err, ErrNotFound) || (err == nil && cfg.Mode == ModeOff) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	within, ok := interp.Compile(def, cfg.Settings).Param(param)
	if !ok {
		return nil, nil
	}
	limit := time.Duration(within) * time.Second
	late, err := s.alerts.OpenAlertsBefore(ctx, now.Add(-limit), queueLimit)
	if err != nil {
		return nil, err
	}
	var out []Escalation
	for _, a := range late {
		if urgent && a.Severity != "high" && a.Severity != "critical" {
			continue
		}
		out = append(out, Escalation{Rule: rule, Alert: a, DueAt: a.CreatedAt.Add(limit)})
	}
	return out, nil
}

// unreported returns the confirmed-fraud cases without an industry fraud report that was not
// dismissed (GOV-12). Nothing when the rule is off or does not apply.
func (s *Service) unreported(ctx context.Context, scope tenancy.Scope) ([]FraudCase, error) {
	_, _, cfg, err := s.rule(ctx, scope, ruleFraudReport)
	if errors.Is(err, ErrNotFound) || (err == nil && cfg.Mode == ModeOff) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	confirmed, err := s.cases.ConfirmedFraud(ctx, queueLimit)
	if err != nil {
		return nil, err
	}
	reported, err := s.store.TaskCaseIDs(ctx, ruleFraudReport, kindIndustryReport)
	if err != nil {
		return nil, err
	}
	var out []FraudCase
	for _, c := range confirmed {
		if !slices.Contains(reported, c.ID) {
			out = append(out, c)
		}
	}
	return out, nil
}

// PrepareFraudReport opens a draft industry fraud report (GOV-12) for a case resolved as
// confirmed fraud, prefilled from the case. It has no deadline: the source states none. A person
// files it with the industry channel; RiskOS never submits it. Requires compliance:write.
func (s *Service) PrepareFraudReport(ctx context.Context, caseID uuid.UUID, now time.Time) (store.Task, error) {
	scope, err := person(ctx, authz.ComplianceWrite)
	if err != nil {
		return store.Task{}, err
	}
	if s.cases == nil {
		return store.Task{}, fmt.Errorf("%w: cases are not connected", ErrInvalidInput)
	}
	var out store.Task
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		if _, _, _, err := s.rule(ctx, scope, ruleFraudReport); err != nil {
			return err
		}
		c, err := s.cases.FraudCase(ctx, caseID)
		if err != nil {
			return err
		}
		if !c.Confirmed {
			return fmt.Errorf("%w: only a case resolved as confirmed fraud is reported", ErrInvalidInput)
		}
		t := store.Task{
			ID: ids.NewV7(), Kind: kindIndustryReport, RuleID: ruleFraudReport, CaseID: &caseID, Status: TaskOpen,
			Report: map[string]any{
				"category": "fraud", "case_title": c.Title, "confirmed_at": c.ResolvedAt.UTC().Format(time.RFC3339),
			},
			CreatedAt: now, CreatedBy: scope.Principal.ID,
		}
		if c.Note != "" {
			t.Report["narrative"] = c.Note
		}
		if c.SubjectRef != "" {
			t.SubjectRef = &c.SubjectRef
		}
		created, err := s.store.InsertTask(ctx, t)
		if err != nil {
			return err
		}
		if !created {
			return fmt.Errorf("%w: this case already has an industry fraud report", ErrConflict)
		}
		out = t
		return s.recordTask(ctx, "compliance.task_created", t)
	})
	return out, err
}

// dailyBatch is how many accounts the daily check handles per transaction; dailyBatches bounds
// a run (the rest waits for the next day).
const (
	dailyBatch   = 500
	dailyBatches = 40
)

// DailyResult is what a daily check did.
type DailyResult struct {
	Opened, Closed int
}

// DailyCheck runs REG-16 over the account register: every account without a BVN gets a task to
// close it, due close_after from when the account was first seen, and open tasks of accounts
// that now have a BVN are done. Only when REG-16 is live in the environment. Idempotent.
// Requires the system permission of the compliance jobs.
func (s *Service) DailyCheck(ctx context.Context, now time.Time) (DailyResult, error) {
	if err := authz.Require(ctx, authz.ComplianceTasksCreate); err != nil {
		return DailyResult{}, err
	}
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return DailyResult{}, err
	}
	var res DailyResult
	for range dailyBatches {
		var n int
		err := s.runner.Run(ctx, func(ctx context.Context) error {
			var err error
			n, err = s.dailyBatch(ctx, scope, now, &res)
			return err
		})
		if err != nil {
			return res, err
		}
		if n < dailyBatch {
			break
		}
	}
	return res, nil
}

// dailyBatch handles one batch of the daily check and returns the size of its larger half.
func (s *Service) dailyBatch(ctx context.Context, scope tenancy.Scope, now time.Time, res *DailyResult) (int, error) {
	_, def, cfg, err := s.rule(ctx, scope, ruleNoBVN)
	if errors.Is(err, ErrNotFound) || (err == nil && cfg.Mode != ModeLive) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	accounts, err := s.store.AccountsWithoutTask(ctx, ruleNoBVN, kindCloseAccount, dailyBatch)
	if err != nil {
		return 0, err
	}
	for _, a := range accounts {
		t := store.Task{
			ID: ids.NewV7(), Kind: kindCloseAccount, RuleID: ruleNoBVN, SubjectRef: a.SubjectRef, AccountRef: &a.Ref,
			Status: TaskOpen, DueAt: taskDue(def, cfg, kindCloseAccount, "", a.FirstSeenAt), Report: map[string]any{},
			CreatedAt: now, CreatedBy: systemTaskAuthor,
		}
		created, err := s.store.InsertTask(ctx, t)
		if err != nil {
			return 0, err
		}
		if created {
			res.Opened++
			if err := s.recordTask(ctx, "compliance.task_created", t); err != nil {
				return 0, err
			}
		}
	}
	tasks, err := s.store.OpenTasksOfAccountsWithBVN(ctx, ruleNoBVN, kindCloseAccount, dailyBatch)
	if err != nil {
		return 0, err
	}
	note, by := "A BVN is now on file for the account.", systemTaskAuthor
	for _, t := range tasks {
		t.Status, t.ClosedBy, t.ClosedAt, t.CloseNote, t.UpdatedAt = TaskDone, &by, &now, &note, now
		if err := s.store.UpdateTask(ctx, t); err != nil {
			return 0, err
		}
		if err := s.recordTask(ctx, "compliance.task_done", t); err != nil {
			return 0, err
		}
	}
	res.Closed += len(tasks)
	return max(len(accounts), len(tasks)), nil
}

// ReportFields are what a person writes in a report.
type ReportFields struct {
	Category         *string
	PredicateOffence *string
	Indicators       []string
	Narrative        *string
}

// strCategories are the report categories goAML accepts (REG-19), and "other".
func strCategories() []string {
	if f, ok := pack.Facts["report.category"]; ok {
		return f.Values
	}
	return nil
}

// task loads a task the caller may act on: sensitive ones need compliance:sensitive, others perm.
func (s *Service) task(ctx context.Context, id uuid.UUID, perm authz.Permission) (store.Task, error) {
	t, err := s.store.Task(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Task{}, fmt.Errorf("%w: task", ErrNotFound)
	}
	if err != nil {
		return store.Task{}, err
	}
	if t.Sensitive && authz.Require(ctx, authz.ComplianceSensitive) != nil {
		// Not even its existence is shown (GOV-09).
		return store.Task{}, fmt.Errorf("%w: task", ErrNotFound)
	}
	if err := authz.Require(ctx, perm); err != nil {
		return store.Task{}, err
	}
	return t, nil
}

func (s *Service) updateTask(ctx context.Context, id uuid.UUID, perm authz.Permission, change func(context.Context, store.Task, tenancy.Scope) (store.Task, string, error), now time.Time) (store.Task, error) {
	scope, err := person(ctx, authz.ComplianceRead)
	if err != nil {
		return store.Task{}, err
	}
	var out store.Task
	err = s.runner.Run(ctx, func(ctx context.Context) error {
		t, err := s.task(ctx, id, perm)
		if err != nil {
			return err
		}
		t, action, err := change(ctx, t, scope)
		if err != nil {
			return err
		}
		t.UpdatedAt = now
		if err := s.store.UpdateTask(ctx, t); err != nil {
			return err
		}
		out = t
		return s.recordTask(ctx, action, t)
	})
	return out, err
}

// EditReport changes a draft report's fields. Requires compliance:write, and
// compliance:sensitive for a suspicious transaction report.
func (s *Service) EditReport(ctx context.Context, id uuid.UUID, f ReportFields, now time.Time) (store.Task, error) {
	return s.updateTask(ctx, id, authz.ComplianceWrite, func(_ context.Context, t store.Task, _ tenancy.Scope) (store.Task, string, error) {
		if !isReport(t.Kind) || t.Status != TaskOpen {
			return t, "", fmt.Errorf("%w: only a draft report can be edited", ErrConflict)
		}
		if f.Category != nil {
			if *f.Category != "" && !slices.Contains(strCategories(), *f.Category) {
				return t, "", fmt.Errorf("%w: category must be one of %v", ErrInvalidInput, strCategories())
			}
			t.Report["category"] = *f.Category
		}
		for name, v := range map[string]*string{"predicate_offence": f.PredicateOffence, "narrative": f.Narrative} {
			if v == nil {
				continue
			}
			if len([]rune(*v)) > 4000 {
				return t, "", fmt.Errorf("%w: %s must be at most 4,000 characters", ErrInvalidInput, name)
			}
			t.Report[name] = strings.TrimSpace(*v)
		}
		if f.Indicators != nil {
			if len(f.Indicators) > 20 {
				return t, "", fmt.Errorf("%w: at most 20 indicators", ErrInvalidInput)
			}
			list := make([]any, 0, len(f.Indicators))
			for _, i := range f.Indicators {
				if i = strings.TrimSpace(i); i != "" {
					list = append(list, i)
				}
			}
			t.Report["indicators"] = list
		}
		return t, "compliance.report_edited", nil
	}, now)
}

// strProblems returns why goAML would reject a suspicious transaction report (REG-19), through
// the pack rule itself.
func (s *Service) strProblems(ctx context.Context, scope tenancy.Scope, t store.Task) ([]string, error) {
	_, def, cfg, err := s.rule(ctx, scope, ruleSTRCheck)
	if err != nil {
		return nil, err
	}
	text := func(k string) string { v, _ := t.Report[k].(string); return v }
	indicators, _ := t.Report["indicators"].([]any)
	facts := interp.Facts{
		"report.has_predicate_offence": text("predicate_offence") != "",
		"report.has_indicator":         len(indicators) > 0,
	}
	if c := text("category"); c != "" {
		facts["report.category"] = c
	}
	res := interp.Evaluate(interp.Compile(def, cfg.Settings), interp.Env{Kinds: []string{"report_submit"}, Facts: facts})
	if res.Status == interp.StatusClear {
		return nil, nil
	}
	var problems []string
	if text("predicate_offence") == "" {
		problems = append(problems, "the predicate offence is missing")
	}
	if len(indicators) == 0 {
		problems = append(problems, "no indicator is given")
	}
	if c := text("category"); c == "" || c == "other" {
		problems = append(problems, "the category must be one goAML accepts")
	}
	return problems, nil
}

// ApproveReport approves a draft report for filing (GOV-06): only a compliance officer
// (compliance:sensitive), and a suspicious transaction report only when goAML would accept it
// (REG-19).
func (s *Service) ApproveReport(ctx context.Context, id uuid.UUID, now time.Time) (store.Task, error) {
	var problems []string
	t, err := s.updateTask(ctx, id, authz.ComplianceSensitive, func(ctx context.Context, t store.Task, scope tenancy.Scope) (store.Task, string, error) {
		if !isReport(t.Kind) || t.Status != TaskOpen {
			return t, "", fmt.Errorf("%w: only a draft report can be approved", ErrConflict)
		}
		if t.Kind == kindSTR {
			var err error
			if problems, err = s.strProblems(ctx, scope, t); err != nil {
				return t, "", err
			}
			if len(problems) > 0 {
				return t, "", fmt.Errorf("%w: this report would be rejected by goAML: %s", ErrInvalidInput, strings.Join(problems, "; "))
			}
		}
		t.Status, t.ApprovedBy, t.ApprovedAt = TaskApproved, &scope.Principal.ID, &now
		return t, "compliance.report_approved", nil
	}, now)
	return t, err
}

// FileReport records that a person filed an approved report with the regulator, with the
// reference the regulator's portal gave. RiskOS never submits reports itself. Requires
// compliance:sensitive.
func (s *Service) FileReport(ctx context.Context, id uuid.UUID, ref string, now time.Time) (store.Task, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || len([]rune(ref)) > 128 {
		return store.Task{}, fmt.Errorf("%w: a filing reference of 1 to 128 characters is required", ErrInvalidInput)
	}
	return s.updateTask(ctx, id, authz.ComplianceSensitive, func(ctx context.Context, t store.Task, scope tenancy.Scope) (store.Task, string, error) {
		if t.Status != TaskApproved {
			return t, "", fmt.Errorf("%w: only an approved report can be filed", ErrConflict)
		}
		t.Status, t.FilingRef, t.ClosedBy, t.ClosedAt = TaskFiled, &ref, &scope.Principal.ID, &now
		return t, "compliance.report_filed", nil
	}, now)
}

// CloseTask marks a task done (not reports: they are filed) or dismisses any open task with a
// note. Requires compliance:write, and compliance:sensitive for a suspicious transaction report.
func (s *Service) CloseTask(ctx context.Context, id uuid.UUID, status, note string, now time.Time) (store.Task, error) {
	note = strings.TrimSpace(note)
	return s.updateTask(ctx, id, authz.ComplianceWrite, func(ctx context.Context, t store.Task, scope tenancy.Scope) (store.Task, string, error) {
		switch {
		case t.Status != TaskOpen && t.Status != TaskApproved:
			return t, "", fmt.Errorf("%w: the task is already %s", ErrConflict, t.Status)
		case status == TaskDone && isReport(t.Kind):
			return t, "", fmt.Errorf("%w: a report is closed by filing it", ErrInvalidInput)
		case status == TaskDismissed && note == "":
			return t, "", fmt.Errorf("%w: dismissing needs a note", ErrInvalidInput)
		case status != TaskDone && status != TaskDismissed:
			return t, "", fmt.Errorf("%w: status must be done or dismissed", ErrInvalidInput)
		case len([]rune(note)) > 1000:
			return t, "", fmt.Errorf("%w: the note must be at most 1,000 characters", ErrInvalidInput)
		}
		t.Status, t.ClosedBy, t.ClosedAt = status, &scope.Principal.ID, &now
		if note != "" {
			t.CloseNote = &note
		}
		return t, "compliance.task_" + status, nil
	}, now)
}

// Task returns one task the caller may see.
func (s *Service) Task(ctx context.Context, id uuid.UUID) (store.Task, error) {
	if err := authz.Require(ctx, authz.ComplianceRead); err != nil {
		return store.Task{}, err
	}
	var out store.Task
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.task(ctx, id, authz.ComplianceRead)
		return err
	})
	return out, err
}

// ExportReport returns an approved (or filed) report for a person to enter in the regulator's
// portal: RiskOS never submits it. Audited. Requires compliance:write, and compliance:sensitive
// for a suspicious transaction report.
func (s *Service) ExportReport(ctx context.Context, id uuid.UUID) (store.Task, error) {
	if _, err := person(ctx, authz.ComplianceRead); err != nil {
		return store.Task{}, err
	}
	var out store.Task
	err := s.runner.Run(ctx, func(ctx context.Context) error {
		t, err := s.task(ctx, id, authz.ComplianceWrite)
		if err != nil {
			return err
		}
		if !isReport(t.Kind) || (t.Status != TaskApproved && t.Status != TaskFiled) {
			return fmt.Errorf("%w: only an approved report can be exported", ErrConflict)
		}
		out = t
		return s.recordTask(ctx, "compliance.report_exported", t)
	})
	return out, err
}
