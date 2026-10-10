// Package store is the only code that reads or writes the auth module's tables. Every query is
// tenant-scoped by construction: the tenant and environment come from the request scope
// (docs/TENANCY.md).
package store
