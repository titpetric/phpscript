// The database/sql drivers this command's tests open a connection with. They
// live here, not in the package: a blank import decides the links of a binary. A
// test binary is also a program. The command itself names the same three in
// main.go.
package test_test

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)
