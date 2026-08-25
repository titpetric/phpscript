package database

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/go-bridget/mig/migrate"
	"github.com/jmoiron/sqlx"

	"github.com/titpetric/phpscript/stdlib/logger"
	"github.com/titpetric/phpscript/telemetry"
)

// DatabaseMigrate loads and runs SQL migrations against a platform database.
//
// The project is the connection name the binding was constructed with, and is
// what mig records applied files under.
type DatabaseMigrate struct {
	database *sqlx.DB
	root     fs.FS
	workDir  string
	project  string
	pattern  string
}

// Load selects the migrations to run, as a glob against the runtime source
// filesystem. Nothing is read here: mig reads the files it applies, and a
// pattern matching nothing is only known to be wrong once Run looks.
//
// A script names its migrations relative to itself ("./schema/*.up.sql"), so
// the work directory is joined in to reach them from the root of the runtime
// filesystem. What the pattern matched is also what mig records, so a file is
// recorded as "schema/bookmarks.up.sql" where this binding used to record the
// base name alone, under no project at all. A database migrated by the older
// binding holds no row under either new name and applies every file again,
// which for a CREATE TABLE is an error rather than a repeat.
func (m *DatabaseMigrate) Load(pattern string) error {
	pattern = strings.TrimPrefix(path.Clean(strings.TrimPrefix(pattern, "./")), "/")
	if m.workDir != "" && m.workDir != "." {
		pattern = path.Join(m.workDir, pattern)
	}

	m.pattern = pattern
	return nil
}

// Run applies the loaded migrations. It is one span rather than one per file:
// migrations run at startup, where what matters is how long the schema took and
// whether it failed, not a row per statement.
func (m *DatabaseMigrate) Run(ctx context.Context) error {
	ctx, span := telemetry.Start(ctx, "migrate", telemetry.KindDatabase)
	defer span.End()

	files, err := migrate.Files(m.root, m.pattern)
	if err != nil {
		span.RecordError(err)
		return err
	}
	span.SetAttribute("migrations", len(files))

	// A pattern matching nothing is ErrNoMigrations from mig, on the grounds
	// that a project pointed at the wrong directory otherwise looks like a
	// clean run. A script can hold a schema directory it has not filled yet,
	// and this binding has always let it start, so the empty set returns here.
	if len(files) == 0 {
		return nil
	}

	manager, err := migrate.NewManager(m.database, m.root, m.project)
	if err != nil {
		span.RecordError(err)
		return err
	}
	manager.Load(m.pattern)

	applied, err := manager.Apply(ctx)

	// Recorded before the run is reported, because recording an error on a
	// span is last write wins and what Apply returns is the driver error on
	// its own. Reporting after it leaves the span with the message that
	// names the file the statement was in.
	span.RecordError(err)
	m.observe(ctx, span, applied, err)
	return err
}

// observe reports the run mig returned. It stands in for the logger mig used to
// be handed and no longer takes, and it is where a failure becomes diagnosable:
// what Apply returns is the driver error on its own, near "THIS": syntax error,
// which does not say which of a dozen files it came out of.
//
// The log keeps the per-file line the old binding produced. The span carries the
// file and the index of the statement within it, because a schema that failed at
// startup is looked at on the trace before anyone opens the log. Only the file
// the run stopped at goes there: mig applies in filename order and returns on
// the first failure, and one attribute per file would bury it under the files
// that were fine.
//
// runErr is what decides a file failed, rather than the status on its record.
// The slice mig reports holds a row per file plus the rows of files that are no
// longer there, and one of those can carry the error of a run that failed
// before the file was deleted; a run that returned nil applied everything it
// found, whatever those rows say.
func (m *DatabaseMigrate) observe(ctx context.Context, span *telemetry.Span, applied []migrate.Migration, runErr error) {
	log := logger.New(ctx, "migrate")
	reported := false

	for _, item := range applied {
		err := item.Err()
		if err == nil || runErr == nil {
			log.Info("migration", "file", item.Filename, "statement", item.StatementIndex, "status", item.Status)
			continue
		}

		if !reported {
			span.SetAttribute("migration", item.Filename)
			span.SetAttribute("statement", item.StatementIndex)
			reported = true
		}

		// The logger fails the span in its context, which is this span,
		// so the trace keeps the error with the file named in it.
		log.Error("failed", "file", item.Filename, "statement", item.StatementIndex, "error", err)
	}
}
