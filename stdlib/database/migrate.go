package database

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/go-bridget/mig/migrate"
	"github.com/jmoiron/sqlx"

	"github.com/titpetric/phpscript/telemetry"
)

// DatabaseMigrate loads and runs SQL migrations against a platform database.
type DatabaseMigrate struct {
	database   *sqlx.DB
	root       fs.FS
	workDir    string
	migrations migrate.FS
}

// Load reads migrations matching pattern from the runtime source filesystem.
func (m *DatabaseMigrate) Load(pattern string) error {
	pattern = strings.TrimPrefix(path.Clean(strings.TrimPrefix(pattern, "./")), "/")
	if m.workDir != "" && m.workDir != "." {
		pattern = path.Join(m.workDir, pattern)
	}

	files, err := fs.Glob(m.root, pattern)
	if err != nil {
		return err
	}

	migrations := migrate.NewFS()
	for _, filename := range files {
		contents, err := fs.ReadFile(m.root, filename)
		if err != nil {
			return err
		}
		migrations[path.Base(filename)] = contents
	}
	m.migrations = migrations
	return nil
}

// Run applies the loaded migrations. It is one span rather than one per file:
// migrations run at startup, where what matters is how long the schema took and
// whether it failed, not a row per statement.
func (m *DatabaseMigrate) Run(ctx context.Context) error {
	span := telemetry.StartSpan(ctx, "migrate", telemetry.KindDatabase)
	defer span.End()
	span.SetAttribute("migrations", len(m.migrations))

	// mig requires a log sink and returns logger.ErrNoLogFn without one. The
	// lines it writes are per-file status, so they go on the span. Writing them
	// to the script's output would put them in whatever the script is producing,
	// which for a migration run at startup is an HTTP body or a rendered page.
	var progress strings.Builder
	options := migrate.NewOptions()
	options.LogFn = func(format string, args ...any) {
		fmt.Fprintf(&progress, format, args...)
		progress.WriteByte('\n')
	}

	err := migrate.RunWithFS(ctx, m.database, m.migrations, options)
	if progress.Len() > 0 {
		span.SetAttribute("log", strings.TrimRight(progress.String(), "\n"))
	}
	span.RecordError(err)
	return err
}
