// Package rules is the public interface of the rules module: structured, versioned risk rules (never executable code).
// Its first part is jurisdiction rule packs and the compliance library (ADR-0006).
//
// This package exports only the module's service interface, its DTOs and its constructor.
// Implementation lives in the module's internal packages, which the Go compiler makes
// unreachable from other modules:
//
//	internal/handler  HTTP handlers for this module's operations in api/openapi.yaml; call the service
//	internal/service  business logic; calls the store and other modules' public interfaces only
//	internal/store    the only code that touches this module's tables (sqlc queries)
//	internal/pack     the pack format, its catalog of facts and events, and its validator
//
// See docs/ARCHITECTURE.md for the boundary rules.
package rules
