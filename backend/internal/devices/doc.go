// Package devices is the public interface of the devices module: device identities and their relationships to users, accounts and sessions.
//
// This package will export only the module's service interface, its DTOs and its constructor.
// Implementation lives in the module's internal packages, which the Go compiler makes
// unreachable from other modules:
//
//	internal/handler  HTTP handlers for this module's operations in api/openapi.yaml; call the service
//	internal/service  business logic; calls the store and other modules' public interfaces only
//	internal/store    the only code that touches this module's tables (sqlc queries)
//
// Design and implementation: TODO(phase-0). See docs/ARCHITECTURE.md for the boundary rules.
package devices
