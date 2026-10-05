//go:build !race

package rules_test

// boundSlowdown widens wall-clock bounds under the race detector; without it, none.
const boundSlowdown = 1
