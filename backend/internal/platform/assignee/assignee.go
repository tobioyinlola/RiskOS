// Package assignee is the contract between the modules that assign work (alerts, cases) and the
// module that knows the workspace's people (auth), so assignment can be validated without the
// modules importing each other.
package assignee

import "context"

// Resolver finds the dashboard user an assignee reference names.
type Resolver interface {
	// ResolveAssignee returns the canonical reference (the user's email) of the active dashboard
	// user of the caller's workspace that ref names, and whether there is one. ref is an email,
	// compared without regard to case. It runs in the caller's tenant transaction.
	ResolveAssignee(ctx context.Context, ref string) (canonical string, found bool, err error)
}
