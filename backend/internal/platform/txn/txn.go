// Package txn lets services run work in one database transaction without depending on the
// database driver. The transaction travels in the context; stores pick it up from there.
//
// Everything a use case writes (for example an API key and its audit entry) goes through one Run
// call, so it commits or rolls back together.
package txn

import "context"

// Runner executes fn inside a transaction bound to the tenant scope in ctx.
//
// Implementations must refuse to run without a tenant scope, must apply row-level security for
// that scope, and must commit only if fn returns nil.
type Runner interface {
	Run(ctx context.Context, fn func(ctx context.Context) error) error
}
