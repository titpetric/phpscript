// Package database holds the named sql connections a phpscript runtime
// resolves through. It is a copy of the provider in
// github.com/titpetric/platform, which lives in an internal package and
// cannot be imported, so that a provider can be scoped to a virtual host
// instead of being process global.
package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"

	"github.com/titpetric/phpscript/model"
)

// Provider is what a runtime resolves named connections through. The interface
// is declared in model, which a runtime can name in its options without
// depending on this package.
type Provider = model.DatabaseProvider

// Default is the provider used when a runtime's options name none. It is the
// process environment, which is what a CLI run has.
var Default Provider = New(os.Environ())

var (
	_ Provider                       = (*DatabaseProvider)(nil)
	_ model.ExtendedDatabaseProvider = (*DatabaseProvider)(nil)
)

// DatabaseProvider holds a list of named sql connection credentials.
type DatabaseProvider struct {
	open func(string, string) (*sqlx.DB, error)

	mu          sync.Mutex
	cache       map[string]*sqlx.DB
	credentials map[string]string
}

// NewDatabaseProvider will allocate a valid `*DatabaseProvider` and return it.
func NewDatabaseProvider(open func(string, string) (*sqlx.DB, error)) *DatabaseProvider {
	return &DatabaseProvider{
		open:        open,
		cache:       make(map[string]*sqlx.DB),
		credentials: make(map[string]string, 1),
	}
}

// New returns a provider holding only the connections named in
// environment, in PLATFORM_DB_<NAME>=<dsn> form, plus the built-in
// default. A provider built this way sees nothing but what it was given,
// which is what keeps one virtual host out of another's databases.
func New(environment []string) *DatabaseProvider {
	provider := NewDatabaseProvider(Open)

	connections := map[string]string{
		"default": "sqlite://:memory:",
	}

	for _, e := range environment {
		if clean, ok := strings.CutPrefix(e, "PLATFORM_DB_"); ok {
			pair := strings.SplitN(clean, "=", 2)
			if len(pair) != 2 {
				continue
			}
			connections[strings.ToLower(pair[0])] = pair[1]
		}
	}

	for name, dsn := range connections {
		provider.Register(name, dsn)
	}

	return provider
}

// List will return the list of credential names.
func (r *DatabaseProvider) List() []string {
	result := make([]string, 0, len(r.credentials))
	for k := range r.credentials {
		result = append(result, k)
	}
	return result
}

// Register will add a new named credential into the provider.
// The function is not concurrency safe, database credentials
// can't be changed during the lifetime of the provider.
func (r *DatabaseProvider) Register(name string, config string) {
	r.credentials[name] = config
}

// Connect issues a PingContext to verify a live connection before returning.
// The context is used to propagate tracing detail so ping is grouped correctly.
func (r *DatabaseProvider) Connect(ctx context.Context, names ...string) (*sqlx.DB, error) {
	db, err := r.Open(ctx, names...)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, err
}

// Open is the same as sql.Open. It creates a client from a named connection.
func (r *DatabaseProvider) Open(_ context.Context, names ...string) (*sqlx.DB, error) {
	db, err := r.cached(r.open, names...)
	return db, err
}

// cached will return a singleton *sqlx.DB from a named connection.
func (r *DatabaseProvider) cached(connector func(string, string) (*sqlx.DB, error), names ...string) (*sqlx.DB, error) {
	if len(names) == 0 {
		names = []string{"default"}
	}

	for _, name := range names {
		r.mu.Lock()
		db, ok := r.cache[name]
		r.mu.Unlock()
		if ok {
			return db, nil
		}
	}

	db, err := r.with(connector, names...)
	if err == nil {
		r.mu.Lock()
		r.cache[names[0]] = db
		r.mu.Unlock()
	}
	return db, err
}

// with will create a *sqlx.DB given the connector (sqlx.Connect/Open).
func (r *DatabaseProvider) with(connector func(string, string) (*sqlx.DB, error), names ...string) (*sqlx.DB, error) {
	if len(names) == 0 {
		names = []string{"default"}
	}

	for _, name := range names {
		if value, ok := r.credentials[name]; ok {
			driver, dsn := r.parseCredential(value)
			client, err := connector(driver, dsn)
			if err != nil {
				return nil, err
			}

			opt := databaseOption(driver, dsn)
			opt.Apply(client)
			return client, nil
		}
	}
	return nil, fmt.Errorf("no configuration found for database: %v", names)
}

func (r *DatabaseProvider) parseCredential(credential string) (driver string, dsn string) {
	driver, dsn = "mysql", credential

	// allow specifying the driver with url notation,
	// in the following form: <driver>://<dsn>.
	if sepIndex := strings.Index(dsn, "://"); sepIndex != -1 {
		driver = dsn[:sepIndex]
		dsn = dsn[sepIndex+3:]
		if driver == "postgres" || driver == "postgresql" {
			driver = "pgx"
			dsn = "postgres://" + dsn
		}
	}

	return driver, cleanDSN(driver, dsn)
}
