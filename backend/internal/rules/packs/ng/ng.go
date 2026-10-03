// Package ng is the Nigeria jurisdiction rule pack (ADR-0006): 19 regulatory controls, 52
// risk-scoring rules and 12 governance presets from docs/compliance/NIGERIA_RULES_LIBRARY.md.
// The files are data, embedded in the binary; text.json is generated from the library document
// (go test ./internal/rules/packs/ng -update) so its wording is never retyped by hand.
package ng

import "embed"

// Files are the pack's files: pack.json, text.json and the rule files.
//
//go:embed *.json
var Files embed.FS
