package auth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/handler"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/txn"
)

// Public types and errors.
type (
	// Config configures the auth module.
	Config = service.Config
	// CreateKeyInput describes an API key to create.
	CreateKeyInput = service.CreateKeyInput
	// CreatedKey is a new API key with the only copy of its secret.
	CreatedKey = domain.CreatedKey
)

// Errors.
var (
	ErrUnauthenticated = domain.ErrUnauthenticated
	ErrTenantSuspended = domain.ErrTenantSuspended
	ErrInvalidInput    = domain.ErrInvalidInput
)

// CredentialHandlers are the auth operations of the public API.
type CredentialHandlers interface {
	GetWhoami(ctx context.Context, req apigen.GetWhoamiRequestObject) (apigen.GetWhoamiResponseObject, error)
	ListAPIKeys(ctx context.Context, req apigen.ListAPIKeysRequestObject) (apigen.ListAPIKeysResponseObject, error)
	CreateAPIKey(ctx context.Context, req apigen.CreateAPIKeyRequestObject) (apigen.CreateAPIKeyResponseObject, error)
	RotateAPIKey(ctx context.Context, req apigen.RotateAPIKeyRequestObject) (apigen.RotateAPIKeyResponseObject, error)
	RevokeAPIKey(ctx context.Context, req apigen.RevokeAPIKeyRequestObject) (apigen.RevokeAPIKeyResponseObject, error)
}

// Pool runs the pre-authentication key lookup. *pgxpool.Pool connected as the application role
// satisfies it.
type Pool = store.Querier

// Module is the auth module.
type Module struct {
	svc   *service.Service
	clock clock.Clock
}

// New returns the auth module.
func New(pool Pool, runner txn.Runner, rec audit.Recorder, clk clock.Clock, cfg Config) *Module {
	return &Module{svc: service.New(store.New(pool), runner, rec, clk, cfg), clock: clk}
}

// Middleware authenticates /api/v1 requests and sets the tenant scope.
func (m *Module) Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return handler.Middleware(m.svc, logger)
}

// HTTPHandlers returns the module's API handlers.
func (m *Module) HTTPHandlers() CredentialHandlers { return handler.New(m.svc, m.clock) }

// CreateKey creates an API key in the environment of the scope in ctx. Used by operator tooling
// to issue a tenant's first key; customers use the API.
func (m *Module) CreateKey(ctx context.Context, in CreateKeyInput) (CreatedKey, error) {
	return m.svc.CreateKey(ctx, in)
}

// DefaultCacheTTL is the default lookup cache TTL: the revocation staleness bound.
const DefaultCacheTTL = 5 * time.Second
