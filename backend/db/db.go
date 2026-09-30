// Package db embeds the SQL migrations so every binary ships with the schema it expects.
package db

import "embed"

// Migrations holds the goose migrations in migrations/*.sql. It is read-only.
//
//go:embed migrations/*.sql
var Migrations embed.FS
