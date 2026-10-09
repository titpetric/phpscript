// The database/sql drivers this command's tests open a connection with. They are
// here and not in the package because a blank import decides what a binary
// links, and a test binary is a program too; the command itself names the same
// three in main.go.
package test_test

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)
