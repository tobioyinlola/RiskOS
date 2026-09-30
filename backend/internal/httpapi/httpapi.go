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
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/clock"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/config"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/health"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/telemetry"
)

// server implements apigen.StrictServerInterface by embedding each module's handlers.
type server struct {
	*health.Handlers
}

var _ apigen.StrictServerInterface = server{}

// Deps are the dependencies of the HTTP API.
type Deps struct {
	Logger    *slog.Logger
	Clock     clock.Clock
	HTTP      config.HTTP
	Telemetry *telemetry.Providers
	Health    *health.Service
}

// New returns the root handler.
//
// Middleware order (outermost first): tracing, request ID, access log, panic recovery, security
// headers, request timeout, body limit, then routing. Tracing wraps everything so the span covers
// the whole request; recovery sits inside the access log so panics are logged with a 500 status.
func New(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(
		httpx.RequestID,
		httpx.AccessLog(d.Logger, d.Clock),
		httpx.Recover(d.Logger),
		httpx.RouteTelemetry,
		httpx.SecurityHeaders,
		httpx.Timeout(d.HTTP.RequestTimeout),
		httpx.BodyLimit(d.HTTP.MaxBodyBytes, d.Logger),
	)
	r.NotFound(httpx.NotFoundHandler(d.Logger))
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler(d.Logger))

	writeErr := func(w http.ResponseWriter, req *http.Request, err error) { httpx.WriteError(w, req, d.Logger, err) }
	strict := apigen.NewStrictHandlerWithOptions(server{Handlers: health.NewHandlers(d.Health)}, nil, apigen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeErr(w, req, httpx.InvalidRequest("The request is invalid.", err))
		},
		ResponseErrorHandlerFunc: writeErr,
	})
	apigen.HandlerWithOptions(strict, apigen.ChiServerOptions{
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeErr(w, req, httpx.InvalidRequest("The request parameters are invalid.", err))
		},
	})

	// Versioned product API. Operations are added to api/openapi.yaml under /api/v1 during
	// system design (TODO(phase-0)); until then every path here is a contract-shaped 404.
	r.Route("/api/v1", func(chi.Router) {})

	return otelhttp.NewHandler(r, "http.server",
		otelhttp.WithTracerProvider(d.Telemetry.TracerProvider),
		otelhttp.WithMeterProvider(d.Telemetry.MeterProvider),
		otelhttp.WithPropagators(d.Telemetry.Propagator),
	)
}
