package ps

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/go-bridget/mig/migrate"
	"github.com/jmoiron/sqlx"
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

// Run applies the loaded migrations.
func (m *DatabaseMigrate) Run(ctx context.Context) error {
	return migrate.RunWithFS(ctx, m.database, m.migrations, migrate.NewOptions())
}
