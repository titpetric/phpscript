// What this test binary links, which is what the fixtures below it can reach: the
// three database/sql drivers a PLATFORM_DB_* connection names, and the image
// format decoders image.Decode answers from.
//
// A blank import runs an init and offers nothing else, so it belongs where a
// binary is wired rather than in a library. The phpscript command names the same
// set in main.go; a test binary is a program too, and says so beside its tests.
package tests

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	_ "modernc.org/sqlite"
)
