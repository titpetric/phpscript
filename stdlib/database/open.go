package database

import (
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Open creates a *sqlx.DB from a driver name and a DSN. It is the default
// open function of DatabaseProvider.
//
// The connection is not instrumented. Query level spans belong to the driver,
// not to the provider, so a caller that wants them supplies an instrumented
// open function instead.
func Open(driver, dsn string) (*sqlx.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	return sqlx.NewDb(db, driver), nil
}
