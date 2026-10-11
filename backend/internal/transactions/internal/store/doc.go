// Package store is the only code that reads or writes the transactions module's tables. Every query is
// tenant-scoped by construction once the module has tables (not built yet).
package store
