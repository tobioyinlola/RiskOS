// Package handler adapts HTTP requests for the auth module: the authentication middleware and
// the API key and whoami operations.
package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/service"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
)

// ProtectedPrefix is the path prefix that requires an API key. Unknown paths under it are
// rejected as unauthenticated before routing, so routes cannot be enumerated without a key.
const ProtectedPrefix = "/api/v1/"

// Middleware authenticates every request under ProtectedPrefix and places the resulting tenant
// scope in the request context. It is the only place a request scope is created.
func Middleware(svc *service.Service, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, ProtectedPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			scope, err := svc.Authenticate(r.Context(), bearerToken(r))
			if err != nil {
				writeAuthError(w, r, logger, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(tenancy.WithScope(r.Context(), scope)))
		})
	}
}

func bearerToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func writeAuthError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		// Security signal for detection (ARCHITECTURE.md §16.4); never logs the presented key.
		logger.InfoContext(r.Context(), "authentication failed", slog.String("reason", "invalid_credential"))
		w.Header().Set("WWW-Authenticate", `Bearer realm="riskos"`)
		httpx.WriteError(w, r, logger, httpx.NewError(http.StatusUnauthorized, httpx.CodeUnauthenticated,
			"A valid API key is required.", err))
	case errors.Is(err, domain.ErrTenantSuspended):
		logger.WarnContext(r.Context(), "authentication rejected", slog.String("reason", "tenant_not_active"))
		httpx.WriteError(w, r, logger, httpx.NewError(http.StatusForbidden, httpx.CodeTenantSuspended,
			"This account is not active.", err))
	default:
		httpx.WriteError(w, r, logger, httpx.NewError(http.StatusServiceUnavailable, httpx.CodeServiceUnavailable,
			"Authentication is temporarily unavailable.", err))
	}
}

// Handlers implements the auth operations of apigen.StrictServerInterface.
type Handlers struct {
	svc   *service.Service
	clock clock.Clock
}

// New returns the auth HTTP handlers.
func New(svc *service.Service, clk clock.Clock) *Handlers { return &Handlers{svc: svc, clock: clk} }

// GetWhoami implements GET /api/v1/whoami.
func (h *Handlers) GetWhoami(ctx context.Context, _ apigen.GetWhoamiRequestObject) (apigen.GetWhoamiResponseObject, error) {
	scope, err := tenancy.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	scopes := make([]apigen.Scope, len(scope.Principal.Permissions))
	for i, p := range scope.Principal.Permissions {
		scopes[i] = apigen.Scope(p)
	}
	return apigen.GetWhoami200JSONResponse{
		TenantId:      ids.Format(ids.PrefixTenant, scope.TenantID),
		EnvironmentId: ids.Format(ids.PrefixEnvironment, scope.EnvironmentID),
		Environment:   apigen.Environment(scope.EnvironmentKind),
		ApiKeyId:      scope.Principal.ID,
		Scopes:        scopes,
	}, nil
}

// ListAPIKeys implements GET /api/v1/api-keys.
func (h *Handlers) ListAPIKeys(ctx context.Context, req apigen.ListAPIKeysRequestObject) (apigen.ListAPIKeysResponseObject, error) {
	page, err := httpx.ParsePage(req.Params.Limit, req.Params.Cursor, ids.PrefixAPIKey)
	if err != nil {
		return nil, err
	}
	keys, err := h.svc.ListKeys(ctx, domain.Page{Limit: page.Limit + 1, Before: page.Before})
	if err != nil {
		return nil, mapError(err)
	}
	keys, next := httpx.NextCursor(keys, page.Limit, ids.PrefixAPIKey, func(k domain.APIKey) uuid.UUID { return k.ID })
	now := h.clock.Now()
	data := make([]apigen.ApiKey, len(keys))
	for i, k := range keys {
		data[i] = toAPI(k, now)
	}
	return apigen.ListAPIKeys200JSONResponse{Data: data, NextCursor: next}, nil
}

// CreateAPIKey implements POST /api/v1/api-keys.
func (h *Handlers) CreateAPIKey(ctx context.Context, req apigen.CreateAPIKeyRequestObject) (apigen.CreateAPIKeyResponseObject, error) {
	if req.Body == nil {
		return nil, httpx.InvalidRequest("A request body is required.", nil)
	}
	scopes := make([]string, len(req.Body.Scopes))
	for i, s := range req.Body.Scopes {
		scopes[i] = string(s)
	}
	created, err := h.svc.CreateKey(ctx, service.CreateKeyInput{Name: req.Body.Name, Scopes: scopes, ExpiresAt: req.Body.ExpiresAt})
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.CreateAPIKey201JSONResponse{ApiKey: toAPI(created.Key, h.clock.Now()), Secret: created.Secret}, nil
}

// RotateAPIKey implements POST /api/v1/api-keys/{key_id}/rotate.
func (h *Handlers) RotateAPIKey(ctx context.Context, req apigen.RotateAPIKeyRequestObject) (apigen.RotateAPIKeyResponseObject, error) {
	id, err := ids.Parse(ids.PrefixAPIKey, req.KeyId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	overlap := service.DefaultOverlap
	if req.Body != nil && req.Body.OverlapSeconds != nil {
		overlap = time.Duration(*req.Body.OverlapSeconds) * time.Second
	}
	created, err := h.svc.RotateKey(ctx, id, overlap)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.RotateAPIKey201JSONResponse{ApiKey: toAPI(created.Key, h.clock.Now()), Secret: created.Secret}, nil
}

// RevokeAPIKey implements POST /api/v1/api-keys/{key_id}/revoke.
func (h *Handlers) RevokeAPIKey(ctx context.Context, req apigen.RevokeAPIKeyRequestObject) (apigen.RevokeAPIKeyResponseObject, error) {
	id, err := ids.Parse(ids.PrefixAPIKey, req.KeyId)
	if err != nil {
		return nil, httpx.NotFound(err)
	}
	key, err := h.svc.RevokeKey(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return apigen.RevokeAPIKey200JSONResponse(toAPI(key, h.clock.Now())), nil
}

// mapError translates domain errors to the error contract; anything else (including
// authz.ErrForbidden, which httpx maps to 403) passes through.
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return httpx.NotFound(err)
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.InvalidRequest(strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": "), err)
	case errors.Is(err, domain.ErrConflict):
		return httpx.Conflict("The API key has been revoked, has expired or was already rotated.", err)
	default:
		return err
	}
}

func toAPI(k domain.APIKey, now time.Time) apigen.ApiKey {
	scopes := make([]apigen.Scope, len(k.Scopes))
	for i, s := range k.Scopes {
		scopes[i] = apigen.Scope(s)
	}
	out := apigen.ApiKey{
		Id:          ids.Format(ids.PrefixAPIKey, k.ID),
		Name:        k.Name,
		Prefix:      domain.DisplayPrefix(k.Environment, k.Lookup),
		Environment: apigen.Environment(k.Environment),
		Scopes:      scopes,
		Status:      apigen.ApiKeyStatus(k.Status(now)),
		CreatedAt:   k.CreatedAt,
		ExpiresAt:   k.ExpiresAt,
		RevokedAt:   k.RevokedAt,
	}
	if k.ReplacedBy != nil {
		r := ids.Format(ids.PrefixAPIKey, *k.ReplacedBy)
		out.ReplacedBy = &r
	}
	return out
}
