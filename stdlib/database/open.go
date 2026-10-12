package database

import (
	"database/sql"
	"fmt"
)

// Open creates a *DB from a driver name and a DSN. It is the default
// open function of DatabaseProvider.
//
// The connection is not instrumented. Query level spans belong to the driver,
// not to the provider, so a caller that wants them supplies an instrumented
// open function instead.
func Open(driver, dsn string) (*DB, error) {
	pool, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	return newDB(pool, driver), nil
}
