// Package httpapi composes the public HTTP API: middleware, the routes generated from
// api/openapi.yaml, and the handlers each module contributes.
//
// Composition rule: every operation in the OpenAPI contract is implemented by exactly one module's
// handler set, embedded in server below. A missing implementation is a compile error, so the
// router can never expose an undocumented endpoint or omit a documented one.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/audit"
	"github.com/tobioyinlola/RiskOS/backend/internal/auth"
	"github.com/tobioyinlola/RiskOS/backend/internal/events"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk"
)

// server implements apigen.StrictServerInterface by embedding each module's handlers.
type server struct {
	*health.Handlers
	auth.CredentialHandlers
	audit.LogHandlers
	events.IngestHandlers
	risk.RuleHandlers
}

var _ apigen.StrictServerInterface = server{}

// Deps are the dependencies of the HTTP API.
type Deps struct {
	Logger    *slog.Logger
	Clock     clock.Clock
	HTTP      config.HTTP
	Telemetry *telemetry.Providers
	Health    *health.Service
	Auth      *auth.Module
	Audit     *audit.Module
	Events    *events.Module
	Risk      *risk.Module
	// Limiter enforces RateLimits on authenticated requests.
	Limiter    Allower
	RateLimits RateLimits
}

// New returns the root handler.
//
// Middleware order (outermost first): tracing, request ID, access log, panic recovery, route
// telemetry, security headers, request timeout, body limit, authentication, rate limiting, then
// routing and, per operation, contract validation. Tracing wraps everything so the span covers
// the whole request; recovery sits inside the access log so panics are logged with a 500 status;
// authentication runs before routing so unknown /api/v1 paths cannot be probed without a key;
// rate limits apply to authenticated callers only; contract validation needs the matched route.
func New(d Deps) (http.Handler, error) {
	validator, err := newContractValidator(d.Logger)
	if err != nil {
		return nil, err
	}
	r := chi.NewRouter()
	r.Use(
		httpx.RequestID,
		httpx.AccessLog(d.Logger, d.Clock),
		httpx.Recover(d.Logger),
		httpx.RouteTelemetry,
		httpx.SecurityHeaders,
		httpx.Timeout(d.HTTP.RequestTimeout),
		httpx.BodyLimit(d.HTTP.MaxBodyBytes, d.Logger),
		d.Auth.Middleware(d.Logger),
		rateLimit(d.Limiter, d.RateLimits, d.Logger),
	)
	r.NotFound(httpx.NotFoundHandler(d.Logger))
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler(d.Logger))

	writeErr := func(w http.ResponseWriter, req *http.Request, err error) { httpx.WriteError(w, req, d.Logger, err) }
	impl := server{
		Handlers:           health.NewHandlers(d.Health),
		CredentialHandlers: d.Auth.HTTPHandlers(),
		LogHandlers:        d.Audit.HTTPHandlers(),
		IngestHandlers:     d.Events.HTTPHandlers(),
		RuleHandlers:       d.Risk.HTTPHandlers(),
	}
	strict := apigen.NewStrictHandlerWithOptions(impl, nil, apigen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeErr(w, req, httpx.InvalidRequest("The request is invalid.", err))
		},
		ResponseErrorHandlerFunc: writeErr,
	})
	apigen.HandlerWithOptions(strict, apigen.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []apigen.MiddlewareFunc{validator.middleware},
		ErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeErr(w, req, httpx.InvalidRequest("The request parameters are invalid.", err))
		},
	})

	return otelhttp.NewHandler(r, "http.server",
		otelhttp.WithTracerProvider(d.Telemetry.TracerProvider),
		otelhttp.WithMeterProvider(d.Telemetry.MeterProvider),
		otelhttp.WithPropagators(d.Telemetry.Propagator),
	), nil
}
