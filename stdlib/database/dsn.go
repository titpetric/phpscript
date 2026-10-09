package database

import (
	"net/url"
	"path/filepath"
	"strings"
)

// resolveSQLiteDSN anchors a relative sqlite file path to root, so a DSN
// written in a site's own configuration names a file under that site's tree
// and not under whatever directory the server process was started in.
// Memory databases, absolute paths and file: URIs carry no such intent and
// pass through unchanged, as does everything when there is no root to anchor
// to, as a CLI run has.
func resolveSQLiteDSN(root, dsn string) string {
	if root == "" || isSQLiteMemoryDSN(dsn) {
		return dsn
	}
	path, query, hasQuery := strings.Cut(dsn, "?")
	if path == "" || strings.HasPrefix(path, "file:") || filepath.IsAbs(path) {
		return dsn
	}
	path = filepath.Join(root, path)
	if hasQuery {
		return path + "?" + query
	}
	return path
}

func cleanDSN(driver, dsn string) string {
	switch driver {
	case "sqlite":
		if isSQLiteMemoryDSN(dsn) {
			return dsn
		}
		dsn = addOptionToDSN(dsn, "?", "?")
		dsn = addOptionToDSN(dsn, "_busy_timeout=", "&_busy_timeout=5000")
		dsn = addOptionToDSN(dsn, "_journal_mode=", "&_journal_mode=wal")
		dsn = strings.Replace(dsn, "?&", "?", 1)
		return dsn
	case "mysql":
		dsn = addOptionToDSN(dsn, "?", "?")
		dsn = addOptionToDSN(dsn, "collation=", "&collation=utf8mb4_general_ci")
		dsn = addOptionToDSN(dsn, "parseTime=", "&parseTime=true")
		dsn = addOptionToDSN(dsn, "loc=", "&loc=Local")
		dsn = strings.Replace(dsn, "?&", "?", 1)
		return dsn
	default:
		return dsn
	}
}

func addOptionToDSN(dsn, match, option string) string {
	if !strings.Contains(dsn, match) {
		dsn += option
	}
	return dsn
}

func isSQLiteMemoryDSN(dsn string) bool {
	database, query, _ := strings.Cut(dsn, "?")
	if strings.EqualFold(database, ":memory:") || strings.EqualFold(database, "file::memory:") {
		return true
	}
	values, _ := url.ParseQuery(query)
	return strings.EqualFold(values.Get("mode"), "memory")
}

// scriptSQLitePath reports the sqlite file path a DSN names, and whether it
// names one that would resolve outside the application root.
//
// resolveSQLiteDSN passes an absolute path and a file: URI through
// untouched, because an operator writing config.yml may point a connection at a
// real path on the host. A script is not the operator: Database::register takes
// its DSN from PHP, so the same spelling there is a tenant naming a host path,
// and opening the connection creates the file. Only the script-facing
// registration consults this; a configured DSN keeps resolving as it did.
//
// A memory DSN names no file and is always allowed.
func scriptSQLitePath(dsn string) (path string, outside bool) {
	driver, rest := "mysql", dsn
	if i := strings.Index(rest, "://"); i != -1 {
		driver, rest = rest[:i], rest[i+3:]
	}
	if !strings.EqualFold(driver, "sqlite") || isSQLiteMemoryDSN(rest) {
		return "", false
	}

	path, _, _ = strings.Cut(rest, "?")
	if path == "" {
		return "", false
	}
	// A file: URI is handed to the driver verbatim, so it is not anchored by
	// resolveSQLiteDSN and names whatever it says.
	return path, filepath.IsAbs(path) || strings.HasPrefix(path, "file:")
}
