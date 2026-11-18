// This test binary links a set of packages. The fixtures below reach those
// packages: the database/sql drivers a PLATFORM_DB_* connection names, plus the
// image format decoders image.Decode answers from.
//
// A blank import runs an init for its side effect, and for that reason it
// belongs in a binary's wiring and not in a library. The phpscript command names the same
// set in main.go; a test binary is a program too. It says so beside its tests.
package tests

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	_ "modernc.org/sqlite"
)
