package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/go-chi/chi/v5"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/httpx"
)

// contractValidator checks every request against api/openapi.yaml before the handler runs:
// parameters, required fields, types, formats, patterns, enums, ranges and additionalProperties
// (unknown fields are rejected). It is the "strict schema validation at the edge" of
// ARCHITECTURE.md §9. Authentication is not its job: the auth middleware has already run.
type contractValidator struct {
	spec   *openapi3.T
	routes map[string]*routers.Route // "METHOD /chi/route/pattern"
	logger *slog.Logger
}

func newContractValidator(logger *slog.Logger) (*contractValidator, error) {
	spec, err := apigen.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI spec: %w", err)
	}
	spec.Servers = nil // validate paths as served, whatever host the request used
	if err := spec.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("invalid embedded OpenAPI spec: %w", err)
	}
	v := &contractValidator{spec: spec, routes: map[string]*routers.Route{}, logger: logger}
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			v.routes[method+" "+path] = &routers.Route{Spec: spec, Path: path, PathItem: item, Method: method, Operation: op}
		}
	}
	return v, nil
}

// middleware validates the routed request. It must run after chi has matched the route (it is
// installed through the generated router's per-operation middleware), so the route pattern is
// the OpenAPI path.
func (v *contractValidator) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := chi.RouteContext(r.Context())
		route, ok := v.routes[r.Method+" "+rc.RoutePattern()]
		if !ok {
			// Every generated route is in the spec; this is a programming error.
			httpx.WriteError(w, r, v.logger, fmt.Errorf("no contract for %s %s", r.Method, rc.RoutePattern()))
			return
		}

		var body []byte
		if r.Body != nil && r.Body != http.NoBody {
			var err error
			if body, err = io.ReadAll(r.Body); err != nil {
				httpx.WriteError(w, r, v.logger, err) // includes *http.MaxBytesError -> 413
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		params := map[string]string{}
		for i, k := range rc.URLParams.Keys {
			params[k] = rc.URLParams.Values[i]
		}
		err := openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: params,
			Route:      route,
			Options: &openapi3filter.Options{
				// Authentication was enforced by the auth middleware before routing.
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
				MultiError:         false,
			},
		})
		if err != nil {
			httpx.WriteError(w, r, v.logger, httpx.InvalidRequest(describe(err), err))
			return
		}
		if body != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			r = r.WithContext(httpx.WithRawBody(r.Context(), body))
		}
		next.ServeHTTP(w, r)
	})
}

// describe turns a validation error into a short, client-safe message naming the offending field.
// It never includes schema internals or the whole input.
func describe(err error) string {
	var reqErr *openapi3filter.RequestError
	if !errors.As(err, &reqErr) {
		return "The request is invalid."
	}
	where := "request"
	if reqErr.Parameter != nil {
		where = fmt.Sprintf("%s parameter %q", reqErr.Parameter.In, reqErr.Parameter.Name)
	} else if reqErr.RequestBody != nil {
		where = "request body"
	}
	var schemaErr *openapi3.SchemaError
	if errors.As(err, &schemaErr) {
		field := strings.Join(schemaErr.JSONPointer(), ".")
		if field != "" {
			where += " field " + field
		}
		return fmt.Sprintf("Invalid %s: %s.", where, strings.TrimSuffix(schemaErr.Reason, "."))
	}
	if reqErr.Reason != "" {
		return fmt.Sprintf("Invalid %s: %s.", where, strings.TrimSuffix(reqErr.Reason, "."))
	}
	return fmt.Sprintf("Invalid %s.", where)
}
