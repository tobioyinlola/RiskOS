package audit

import (
	"context"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/store/sqlcgen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Public types.
type (
	// Entry is what a module records.
	Entry = domain.Entry
	// Outcome of an audited action.
	Outcome = domain.Outcome
	// VerifyResult reports a chain verification.
	VerifyResult = domain.VerifyResult
)

// Outcomes.
const (
	OutcomeSuccess = domain.OutcomeSuccess
	OutcomeFailure = domain.OutcomeFailure
	OutcomeDenied  = domain.OutcomeDenied
)

// Recorder appends audit entries. Record must be called inside the transaction of the audited
// action (txn.Runner.Run), so the action and its entry commit together.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// LogHandlers are the audit operations of the public API.
type LogHandlers interface {
	ListAuditLogs(ctx context.Context, req apigen.ListAuditLogsRequestObject) (apigen.ListAuditLogsResponseObject, error)
}

// Module is the audit module.
type Module struct {
	svc *service.Service
}

// New returns the audit module.
func New(runner txn.Runner, clk clock.Clock) *Module {
	return &Module{svc: service.New(runner, clk)}
}

// Recorder returns the audit recorder for other modules.
func (m *Module) Recorder() Recorder { return m.svc }

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() LogHandlers { return handler.New(m.svc) }

// Verify recomputes the tenant's whole hash chain. conn must be an operator connection (the
// table owner, typically a *pgxpool.Pool), because entries of every environment are verified.
func Verify(ctx context.Context, conn sqlcgen.DBTX, tenantID uuid.UUID) (VerifyResult, error) {
	return service.Verify(ctx, store.NewReader(conn), tenantID)
}
