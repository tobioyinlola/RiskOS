// Package events is the public interface of the events module: event ingestion: validation, normalization, idempotency and persistence.
//
// This package exports only the module's service interface, its DTOs and its constructor.
// Implementation lives in the module's internal packages, which the Go compiler makes
// unreachable from other modules:
//
//	internal/handler  HTTP handlers for this module's operations in api/openapi.yaml; call the service
//	internal/service  business logic; calls the store and other modules' public interfaces only
//	internal/store    the only code that touches this module's tables (sqlc queries)
//
// See docs/ARCHITECTURE.md for the boundary rules.
package events
