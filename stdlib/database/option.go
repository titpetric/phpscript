package database

import "github.com/jmoiron/sqlx"

// DatabaseOption configures database connection pooling settings.
type DatabaseOption struct {
	MaxOpenConns int
	MaxIdleConns int
}

// Apply applies the database option settings to a database connection.
func (o *DatabaseOption) Apply(client *sqlx.DB) {
	if o == nil {
		return
	}
	client.SetMaxOpenConns(o.MaxOpenConns)
	client.SetMaxIdleConns(o.MaxIdleConns)
}

var databaseOptions = map[string]DatabaseOption{
	"sqlite": {
		MaxOpenConns: 10,
		MaxIdleConns: 2,
	},
	"mysql": {
		MaxOpenConns: 10,
		MaxIdleConns: 10,
	},
}

func databaseOption(driver, dsn string) DatabaseOption {
	if driver == "sqlite" && isSQLiteMemoryDSN(dsn) {
		return DatabaseOption{
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		}
	}
	return databaseOptions[driver]
}
