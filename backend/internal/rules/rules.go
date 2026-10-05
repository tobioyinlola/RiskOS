package rules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/alerts"
	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/cases"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/authz"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/gh"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/rw"
	"github.com/tobioyinlola/RiskOS/backend/internal/tenants"
)

// Pool is the database pool (*pgxpool.Pool).
type Pool = store.Pool

// Errors.
var (
	// ErrChecksumChanged means a loaded pack version's files have changed: publish a new version.
	ErrChecksumChanged = store.ErrChecksumChanged
	// ErrInvalidInput means a request about a rule is malformed; the message says why.
	ErrInvalidInput = service.ErrInvalidInput
	// ErrNotFound means there is no such rule or pack.
	ErrNotFound = service.ErrNotFound
)

// LibraryHandlers are the compliance library operations of the public API.
type LibraryHandlers interface {
	GetCompliancePack(ctx context.Context, req apigen.GetCompliancePackRequestObject) (apigen.GetCompliancePackResponseObject, error)
	GetComplianceClockMatrix(ctx context.Context, req apigen.GetComplianceClockMatrixRequestObject) (apigen.GetComplianceClockMatrixResponseObject, error)
	ExportComplianceLibrary(ctx context.Context, req apigen.ExportComplianceLibraryRequestObject) (apigen.ExportComplianceLibraryResponseObject, error)
	ListComplianceJurisdictions(ctx context.Context, req apigen.ListComplianceJurisdictionsRequestObject) (apigen.ListComplianceJurisdictionsResponseObject, error)
	ListComplianceActivations(ctx context.Context, req apigen.ListComplianceActivationsRequestObject) (apigen.ListComplianceActivationsResponseObject, error)
	ListActiveComplianceRules(ctx context.Context, req apigen.ListActiveComplianceRulesRequestObject) (apigen.ListActiveComplianceRulesResponseObject, error)
	CreateComplianceActivation(ctx context.Context, req apigen.CreateComplianceActivationRequestObject) (apigen.CreateComplianceActivationResponseObject, error)
	UpdateComplianceActivation(ctx context.Context, req apigen.UpdateComplianceActivationRequestObject) (apigen.UpdateComplianceActivationResponseObject, error)
	SetComplianceActivationStatus(ctx context.Context, req apigen.SetComplianceActivationStatusRequestObject) (apigen.SetComplianceActivationStatusResponseObject, error)
	MakeComplianceActivationFallback(ctx context.Context, req apigen.MakeComplianceActivationFallbackRequestObject) (apigen.MakeComplianceActivationFallbackResponseObject, error)
	GetComplianceActivationReadiness(ctx context.Context, req apigen.GetComplianceActivationReadinessRequestObject) (apigen.GetComplianceActivationReadinessResponseObject, error)
	ListComplianceActivationChanges(ctx context.Context, req apigen.ListComplianceActivationChangesRequestObject) (apigen.ListComplianceActivationChangesResponseObject, error)
	ApproveComplianceActivationChange(ctx context.Context, req apigen.ApproveComplianceActivationChangeRequestObject) (apigen.ApproveComplianceActivationChangeResponseObject, error)
	RejectComplianceActivationChange(ctx context.Context, req apigen.RejectComplianceActivationChangeRequestObject) (apigen.RejectComplianceActivationChangeResponseObject, error)
	VerifyComplianceGap(ctx context.Context, req apigen.VerifyComplianceGapRequestObject) (apigen.VerifyComplianceGapResponseObject, error)
	ListCompliancePacks(ctx context.Context, req apigen.ListCompliancePacksRequestObject) (apigen.ListCompliancePacksResponseObject, error)
	ListComplianceRules(ctx context.Context, req apigen.ListComplianceRulesRequestObject) (apigen.ListComplianceRulesResponseObject, error)
	GetComplianceRule(ctx context.Context, req apigen.GetComplianceRuleRequestObject) (apigen.GetComplianceRuleResponseObject, error)
	ListComplianceClocks(ctx context.Context, req apigen.ListComplianceClocksRequestObject) (apigen.ListComplianceClocksResponseObject, error)
	ListComplianceGaps(ctx context.Context, req apigen.ListComplianceGapsRequestObject) (apigen.ListComplianceGapsResponseObject, error)
	ListComplianceGlossary(ctx context.Context, req apigen.ListComplianceGlossaryRequestObject) (apigen.ListComplianceGlossaryResponseObject, error)
	GetComplianceProfile(ctx context.Context, req apigen.GetComplianceProfileRequestObject) (apigen.GetComplianceProfileResponseObject, error)
	PutComplianceProfile(ctx context.Context, req apigen.PutComplianceProfileRequestObject) (apigen.PutComplianceProfileResponseObject, error)
	GetPackRuleConfig(ctx context.Context, req apigen.GetPackRuleConfigRequestObject) (apigen.GetPackRuleConfigResponseObject, error)
	PutPackRuleConfig(ctx context.Context, req apigen.PutPackRuleConfigRequestObject) (apigen.PutPackRuleConfigResponseObject, error)
	ListPackRuleVersions(ctx context.Context, req apigen.ListPackRuleVersionsRequestObject) (apigen.ListPackRuleVersionsResponseObject, error)
	RollbackPackRule(ctx context.Context, req apigen.RollbackPackRuleRequestObject) (apigen.RollbackPackRuleResponseObject, error)
	ListPackRuleChanges(ctx context.Context, req apigen.ListPackRuleChangesRequestObject) (apigen.ListPackRuleChangesResponseObject, error)
	ApprovePackRuleChange(ctx context.Context, req apigen.ApprovePackRuleChangeRequestObject) (apigen.ApprovePackRuleChangeResponseObject, error)
	RejectPackRuleChange(ctx context.Context, req apigen.RejectPackRuleChangeRequestObject) (apigen.RejectPackRuleChangeResponseObject, error)
	ListComplianceFXRates(ctx context.Context, req apigen.ListComplianceFXRatesRequestObject) (apigen.ListComplianceFXRatesResponseObject, error)
	PutComplianceFXRate(ctx context.Context, req apigen.PutComplianceFXRateRequestObject) (apigen.PutComplianceFXRateResponseObject, error)
	DeleteComplianceFXRate(ctx context.Context, req apigen.DeleteComplianceFXRateRequestObject) (apigen.DeleteComplianceFXRateResponseObject, error)
	ListComplianceTasks(ctx context.Context, req apigen.ListComplianceTasksRequestObject) (apigen.ListComplianceTasksResponseObject, error)
	GetComplianceTask(ctx context.Context, req apigen.GetComplianceTaskRequestObject) (apigen.GetComplianceTaskResponseObject, error)
	EditComplianceReport(ctx context.Context, req apigen.EditComplianceReportRequestObject) (apigen.EditComplianceReportResponseObject, error)
	ApproveComplianceReport(ctx context.Context, req apigen.ApproveComplianceReportRequestObject) (apigen.ApproveComplianceReportResponseObject, error)
	FileComplianceReport(ctx context.Context, req apigen.FileComplianceReportRequestObject) (apigen.FileComplianceReportResponseObject, error)
	CloseComplianceTask(ctx context.Context, req apigen.CloseComplianceTaskRequestObject) (apigen.CloseComplianceTaskResponseObject, error)
	ExportComplianceReport(ctx context.Context, req apigen.ExportComplianceReportRequestObject) (apigen.ExportComplianceReportResponseObject, error)
	ConfirmSuspicion(ctx context.Context, req apigen.ConfirmSuspicionRequestObject) (apigen.ConfirmSuspicionResponseObject, error)
	PrepareFraudReport(ctx context.Context, req apigen.PrepareFraudReportRequestObject) (apigen.PrepareFraudReportResponseObject, error)
}

// Module is the rules module.
type Module struct {
	svc *service.Service
}

// New returns the rules module.
func New(runner txn.Runner, rec audit.Recorder) *Module {
	return &Module{svc: service.New(runner, rec)}
}

// HTTPHandlers returns the compliance library handlers of the public API.
func (m *Module) HTTPHandlers(clk clock.Clock) LibraryHandlers { return handler.New(m.svc, clk) }

// BuiltInPacks returns the jurisdiction packs compiled into the binary, in load order: Ghana
// and Rwanda reuse Nigerian rules, so Nigeria loads first (ADR-0007).
func BuiltInPacks() []fs.FS { return []fs.FS{ng.Files, gh.Files, rw.Files} }

// LoadPacks validates packs (normally BuiltInPacks) and stores every version not yet in the
// database. pool must be connected as the schema owner (the application role can only read
// packs). Idempotent; a loaded version whose files changed returns ErrChecksumChanged.
func LoadPacks(ctx context.Context, pool Pool, packs []fs.FS, logger *slog.Logger) error {
	return service.LoadPacks(ctx, pool, packs, logger)
}

// Evaluation of a tenant's pack by the risk engine.
type (
	// TenantPack is one activation's pack compiled for evaluation.
	TenantPack = service.TenantPack
	// TenantPacks are a tenant's activations compiled for evaluation; nil without any.
	TenantPacks = service.TenantPacks
	// Resolution is which pack governs an event and why (ADR-0007).
	Resolution = service.Resolution
	// USDRate is the exchange rate a conversion used, recorded with the evaluation.
	USDRate = service.USDRate
	// Outcome is one triggered rule's evaluation.
	Outcome = service.Outcome
	// Window is one windowed aggregate a rule reads.
	Window = interp.AggRef
	// Facts are an event's known values by catalog name.
	Facts = interp.Facts
	// Money is an amount in minor units of a currency.
	Money = interp.Money
	// Aggs answers windowed aggregates.
	Aggs = interp.Aggs
	// Env is what one evaluation sees.
	Env = interp.Env
)

// Rule outcome statuses.
const (
	StatusFired        = interp.StatusFired
	StatusClear        = interp.StatusClear
	StatusNotEvaluated = interp.StatusNotEvaluated
)

// Rule kinds and modes.
const (
	KindControl = pack.KindControl
	KindSignal  = pack.KindSignal
	ModeLive    = pack.ModeLive
)

// How an event's jurisdiction was resolved.
const (
	ResolvedBookingEntity = service.ResolvedBookingEntity
	ResolvedFallback      = service.ResolvedFallback
	ResolvedNone          = service.ResolvedNone
	ResolvedInactive      = service.ResolvedInactive
)

// TenantPacks returns the caller's tenant's activations compiled for evaluation, or nil without
// any. It runs in the caller's transaction.
func (m *Module) TenantPacks(ctx context.Context) (*TenantPacks, error) {
	return m.svc.TenantPacks(ctx)
}

// Candidates compiles one rule for a backtest: with settings (nil: the current configuration)
// and with the environment's current configuration.
func (m *Module) Candidates(ctx context.Context, ruleID string, settings map[string]float64) (candidate, current *TenantPack, err error) {
	return m.svc.Candidates(ctx, ruleID, settings)
}

// Record reports whether an event passes a window's filter (record) and whether the filter could
// be checked (known): an unchecked event is recorded as unknown, so the window reads as
// unavailable rather than undercounting.
func Record(w Window, facts Facts) (record, known bool) {
	switch interp.Where(w, facts) {
	case interp.True:
		return true, true
	case interp.Unknown:
		return true, false
	default:
		return false, true
	}
}

// The compliance workflow: tasks fired controls require, suspicious transaction reports, and
// the alerts and cases it reads.

// TaskControl is a live control that fired, with its tasks.
type TaskControl = service.TaskControl

// TrackedTask reports whether RiskOS keeps a control's task as compliance work.
func TrackedTask(kind string) bool { return service.TrackedTask(kind) }

// ConnectWorkflow connects the alerts and cases the compliance workflow reads: alerts past
// their screening deadline (REG-14) and who a case is about (suspicious transaction reports).
func (m *Module) ConnectWorkflow(a *alerts.Module, c *cases.Module) {
	m.svc.UseWorkflow(alertSource{a}, caseSource{c})
}

type alertSource struct{ m *alerts.Module }

func (s alertSource) OpenAlertsBefore(ctx context.Context, before time.Time, limit int) ([]service.OpenAlert, error) {
	list, err := s.m.OpenBefore(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]service.OpenAlert, len(list))
	for i, a := range list {
		out[i] = service.OpenAlert{ID: a.ID, Title: a.Title, SubjectRef: a.SubjectRef, Severity: a.Severity, CreatedAt: a.CreatedAt}
	}
	return out, nil
}

type caseSource struct{ m *cases.Module }

// CaseSubject returns the subject of the case's first alert ("" for a case without alerts).
func (s caseSource) CaseSubject(ctx context.Context, id uuid.UUID) (string, error) {
	d, err := s.m.Get(ctx, id)
	if errors.Is(err, cases.ErrNotFound) {
		return "", fmt.Errorf("%w: case", ErrNotFound)
	}
	if err != nil {
		return "", err
	}
	for _, a := range d.Alerts {
		return a.SubjectRef, nil
	}
	return "", nil
}

// FraudCase returns a case with whether it was resolved as confirmed fraud.
func (s caseSource) FraudCase(ctx context.Context, id uuid.UUID) (service.FraudCase, error) {
	d, err := s.m.Get(ctx, id)
	if errors.Is(err, cases.ErrNotFound) {
		return service.FraudCase{}, fmt.Errorf("%w: case", ErrNotFound)
	}
	if err != nil {
		return service.FraudCase{}, err
	}
	out := fraudCase(d.Case)
	for _, a := range d.Alerts {
		out.SubjectRef = a.SubjectRef
		break
	}
	return out, nil
}

// ConfirmedFraud lists cases resolved as confirmed fraud, newest first.
func (s caseSource) ConfirmedFraud(ctx context.Context, limit int) ([]service.FraudCase, error) {
	list, err := s.m.ConfirmedFraud(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]service.FraudCase, len(list))
	for i, c := range list {
		out[i] = fraudCase(c)
	}
	return out, nil
}

func fraudCase(c cases.Case) service.FraudCase {
	out := service.FraudCase{ID: c.ID, Title: c.Title}
	if c.Resolution != nil && *c.Resolution == "confirmed_fraud" {
		out.Confirmed = true
	}
	if c.ResolvedAt != nil {
		out.ResolvedAt = *c.ResolvedAt
	}
	if c.ResolutionNote != nil {
		out.Note = *c.ResolutionNote
	}
	return out
}

// TasksArgs is the job recording the compliance work of a decision. It carries identifiers and
// the controls that fired, never payloads.
type TasksArgs struct {
	TenantID, EnvironmentID uuid.UUID
	EvaluationID, EventID   uuid.UUID
	OccurredAt              time.Time
	SubjectRef              string
	// AccountRef and HasBVN feed the account register of REG-16's daily check.
	AccountRef string `json:",omitempty"`
	HasBVN     *bool  `json:",omitempty"`
	Amount     int64
	Currency   string
	Controls   []TaskControl
}

// Kind implements jobs.Args.
func (TasksArgs) Kind() string { return "compliance_tasks" }

// tasksActor names the task job in the audit log.
const tasksActor = "system:compliance"

// TasksJob returns the handler of TasksArgs: it re-establishes the tenant scope with the single
// system permission the job needs, and drops jobs of tenants no longer active.
func (m *Module) TasksJob(scopes *tenants.Module, clk clock.Clock, logger *slog.Logger) func(context.Context, TasksArgs) error {
	return func(ctx context.Context, a TasksArgs) error {
		ctx, err := scopes.JobScope(ctx, a.TenantID, a.EnvironmentID, tasksActor, []string{authz.ComplianceTasksCreate})
		if errors.Is(err, tenants.ErrNotFound) || errors.Is(err, tenants.ErrNotActive) {
			logger.WarnContext(ctx, "compliance task job dropped: tenant not active",
				slog.String("tenant_id", a.TenantID.String()), slog.String("evaluation_id", a.EvaluationID.String()))
			return nil
		}
		if err != nil {
			return err
		}
		return m.svc.CreateTasks(ctx, service.TasksInput{
			EvaluationID: a.EvaluationID, EventID: a.EventID, OccurredAt: a.OccurredAt, SubjectRef: a.SubjectRef,
			AccountRef: a.AccountRef, HasBVN: a.HasBVN, Amount: a.Amount, Currency: a.Currency, Controls: a.Controls,
		}, clk.Now())
	}
}

// FactHasBVN is the event fact saying whether the event's account has a BVN on file.
const FactHasBVN = "account.has_bvn"

// DailyArgs is the daily compliance check (REG-16's accounts without a BVN). It carries nothing:
// the job visits every environment with accounts in the register.
type DailyArgs struct{}

// Kind implements jobs.Args.
func (DailyArgs) Kind() string { return "compliance_daily" }

// DailyJob returns the handler of DailyArgs. It lists the environments to visit on db, outside
// any tenant scope (identifiers only), then checks each inside its tenant's scope with the
// single system permission it needs. One failing environment does not stop the others.
func (m *Module) DailyJob(db store.DBTX, scopes *tenants.Module, clk clock.Clock, logger *slog.Logger) func(context.Context, DailyArgs) error {
	return func(ctx context.Context, _ DailyArgs) error {
		list, err := store.AccountScopes(ctx, db)
		if err != nil {
			return err
		}
		var errs []error
		for _, sc := range list {
			scoped, err := scopes.JobScope(ctx, sc.TenantID, sc.EnvironmentID, tasksActor, []string{authz.ComplianceTasksCreate})
			if errors.Is(err, tenants.ErrNotFound) || errors.Is(err, tenants.ErrNotActive) {
				continue
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}
			res, err := m.svc.DailyCheck(scoped, clk.Now())
			if err != nil {
				errs = append(errs, fmt.Errorf("daily compliance check of tenant %s: %w", sc.TenantID, err))
				continue
			}
			if res.Opened > 0 || res.Closed > 0 {
				logger.InfoContext(ctx, "daily compliance check", slog.String("tenant_id", sc.TenantID.String()),
					slog.Int("opened", res.Opened), slog.Int("closed", res.Closed))
			}
		}
		return errors.Join(errs...)
	}
}
