// Package migrations embeds the versioned SQL migration files so the
// service can apply them at startup without depending on the working
// directory at runtime.
package migrations

import "embed"

// FS holds the *.up.sql and *.down.sql files consumed by the golang-migrate
// iofs source driver.
//
//go:embed *.sql
var FS embed.FS //nolint:gochecknoglobals // go:embed only populates package-level vars.
