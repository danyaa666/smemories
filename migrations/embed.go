// Package migrations embeds the goose SQL migrations so the binary (and a one-off ECS
// task later) carries its own schema.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
