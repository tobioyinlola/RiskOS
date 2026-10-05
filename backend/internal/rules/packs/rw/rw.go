// Package rw is the Rwanda jurisdiction rule pack (ADR-0007): 14 regulatory controls, 24
// typology rules, 16 cybersecurity rules and 10 governance presets from
// docs/compliance/GHANA_RWANDA_RULES_LIBRARY.md, plus the Nigerian rules that apply unchanged.
// The files are data, embedded in the binary; text.json is generated from the library document
// (go test ./internal/rules/packs/rw -update) so its wording is never retyped by hand.
package rw

import "embed"

// Files are the pack's files: pack.json, text.json and the rule files.
//
//go:embed *.json
var Files embed.FS
