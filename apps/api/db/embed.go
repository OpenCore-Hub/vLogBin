// Package db embeds the goose migration files into the binary.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
