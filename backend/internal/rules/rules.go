package rules

import (
	"context"
	"io/fs"
	"log/slog"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
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

// BuiltInPacks returns the jurisdiction packs compiled into the binary.
func BuiltInPacks() []fs.FS { return []fs.FS{ng.Files} }

// LoadPacks validates packs (normally BuiltInPacks) and stores every version not yet in the
// database. pool must be connected as the schema owner (the application role can only read
// packs). Idempotent; a loaded version whose files changed returns ErrChecksumChanged.
func LoadPacks(ctx context.Context, pool Pool, packs []fs.FS, logger *slog.Logger) error {
	return service.LoadPacks(ctx, pool, packs, logger)
}

// Evaluation of a tenant's pack by the risk engine.
type (
	// TenantPack is a tenant's pack compiled for evaluation; nil when it has no profile.
	TenantPack = service.TenantPack
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

// TenantPack returns the caller's tenant's pack compiled for evaluation, or nil without a
// compliance profile. It runs in the caller's transaction.
func (m *Module) TenantPack(ctx context.Context) (*TenantPack, error) { return m.svc.TenantPack(ctx) }

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
