package migrations

import "embed"

// FS contains the SQL migrations consumed by Goose.
//
//go:embed *.sql
var FS embed.FS
