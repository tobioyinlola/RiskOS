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
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

// Pool is the database pool (*pgxpool.Pool).
type Pool = store.Pool

// ErrChecksumChanged means a loaded pack version's files have changed: publish a new version.
var ErrChecksumChanged = store.ErrChecksumChanged

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
