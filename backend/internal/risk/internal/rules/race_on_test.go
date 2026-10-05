//go:build race

package rules_test

// boundSlowdown widens wall-clock bounds under the race detector, which slows code 5 to 10 times.
const boundSlowdown = 10
