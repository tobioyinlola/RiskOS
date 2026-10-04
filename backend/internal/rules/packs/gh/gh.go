// Package gh is the Ghana jurisdiction rule pack (ADR-0007): 13 regulatory controls, 28
// typology rules, 14 cybersecurity rules and 13 governance presets from
// docs/compliance/GHANA_RWANDA_RULES_LIBRARY.md, plus the Nigerian rules that apply unchanged.
// The files are data, embedded in the binary; text.json is generated from the library document
// (go test ./internal/rules/packs/gh -update) so its wording is never retyped by hand.
package gh

import "embed"

// Files are the pack's files: pack.json, text.json and the rule files.
//
//go:embed *.json
var Files embed.FS
