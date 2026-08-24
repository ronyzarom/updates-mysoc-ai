// Package migrations embeds the server's schema migration files so the
// binary applies them itself at startup — no manual psql step on deploy.
// Down files are intentionally not embedded: rollbacks are an operator
// decision, performed with the on-disk *.down.sql files.
package migrations

import "embed"

//go:embed *.up.sql
var Up embed.FS
