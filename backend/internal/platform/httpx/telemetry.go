package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// unmatchedRoute labels requests that matched no route, keeping metric cardinality bounded:
// raw paths are never used as span names or metric attributes.
const unmatchedRoute = "unmatched"

// RouteTelemetry names the active span "<METHOD> <route pattern>" and adds http.route to the
// span and to otelhttp's request metrics. It must run inside the chi router, where the route
// pattern is known once the request has been routed.
func RouteTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		route := unmatchedRoute
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		attr := attribute.String("http.route", route)
		span := trace.SpanFromContext(r.Context())
		span.SetName(r.Method + " " + route)
		span.SetAttributes(attr)
		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			labeler.Add(attr)
		}
	})
}
