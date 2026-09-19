package database

import (
	"net/url"
	"path/filepath"
	"strings"
)

// resolveSQLiteDSN anchors a relative sqlite file path to root, so a DSN
// written in a site's own configuration names a file under that site's tree
// rather than under whatever directory the server process was started in.
// Memory databases, absolute paths and file: URIs carry no such intent and
// pass through unchanged, as does everything when there is no root to anchor
// to, which is what a CLI run has.
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
