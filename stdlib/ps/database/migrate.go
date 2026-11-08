package database

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/go-bridget/mig/migrate"

	"github.com/titpetric/phpscript/stdlib/logger"
	"github.com/titpetric/phpscript/telemetry"
)

// DatabaseMigrate loads and runs SQL migrations against a platform database.
//
// The project is the binding's own connection name, and is
// what mig records applied files under.
type DatabaseMigrate struct {
	database *DB
	root     fs.FS
	workDir  string
	project  string
	pattern  string
}

// Load names the migrations to run, as a glob against the runtime source
// filesystem with the work directory joined in. It opens no file; Run does.
func (m *DatabaseMigrate) Load(pattern string) error {
	pattern = strings.TrimPrefix(path.Clean(strings.TrimPrefix(pattern, "./")), "/")
	if m.workDir != "" && m.workDir != "." {
		pattern = path.Join(m.workDir, pattern)
	}

	m.pattern = pattern
	return nil
}

// Run applies the loaded migrations. It opens one span and not one per file:
// migrations run at startup, where what matters is how long the schema took and
// whether it failed, with no row per statement.
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
	// clean run. A script can hold a schema directory it has not filled yet.
	// This binding has always let it start and the empty set returns here.
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

	// Recorded before observe reports the run, because recording an error on a
	// span is last write wins and what Apply returns is the driver error
	// alone. Reporting after it leaves the span with the message that
	// names the file holding the statement.
	span.RecordError(err)
	m.observe(ctx, span, applied, err)
	return err
}

// observe reports the run mig returned. It replaces the logger mig once took
// and now refuses. It turns a failure into something diagnosable: what Apply
// returns is the driver error alone, near "THIS": syntax error, which does not
// say which of a dozen files it came out of.
//
// The log keeps the per-file line the old binding produced. The span records the
// file and the statement's index in that file, because an operator reads a
// startup schema failure off the trace before anyone opens the log. Only the
// file the run stopped at goes there: mig applies in filename order and returns
// on the first failure, and one attribute per file would bury it under the
// files that pass.
//
// runErr is what decides a file failed, and not the status on its record.
// The slice mig reports holds a row per file plus the rows of files since
// removed from the tree, and one of those can hold the error of a run that
// failed from before the file's removal; a run answering nil applied everything
// it found, whatever those rows say.
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

		// The logger fails the span in its context, which is this span.
		// The trace keeps the error with the file named in it.
		log.Error("failed", "file", item.Filename, "statement", item.StatementIndex, "error", err)
	}
}
