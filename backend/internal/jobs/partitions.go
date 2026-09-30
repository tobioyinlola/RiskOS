package jobs

// EnsurePartitionsArgs is the daily job that keeps monthly partitions of the time-partitioned
// tables (events, evaluations) created ahead of time. It is idempotent.
type EnsurePartitionsArgs struct{}

// Kind implements Args.
func (EnsurePartitionsArgs) Kind() string { return "ensure_time_partitions" }
