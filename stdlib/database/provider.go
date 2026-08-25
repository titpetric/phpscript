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
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]string, 0, len(r.credentials))
	for k := range r.credentials {
		result = append(result, k)
	}
	return result
}

// Register will add a new named credential into the provider.
//
// A host that keeps its connections in a database registers them per request,
// from whichever goroutine is serving it, so the credentials map is guarded
// like the pool cache beside it.
//
// Re-registering a name with the same configuration is free. Re-registering it
// with a different one drops the pool that was opened for the old credentials:
// the cache is keyed by name, and a name that now means a different database
// must not keep answering with the old one.
func (r *DatabaseProvider) Register(name string, config string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.credentials[name]; ok && existing == config {
		return
	}

	r.credentials[name] = config
	if db, ok := r.cache[name]; ok {
		delete(r.cache, name)
		db.Close()
	}
}

// credential reads a named credential under the lock.
func (r *DatabaseProvider) credential(name string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	value, ok := r.credentials[name]
	return value, ok
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
//
// The credential decides which name the pool belongs to, and the cache is read
// and written under that one rather than under the first name the caller asked
// for. Callers name fallbacks: Database\Migrate asks for "app:migrate" before
// "app", and both mean the credential "app" until a deployment registers the
// first. Keying on the name asked for instead would give that caller a pool of
// its own and every caller naming "app" another, and two pools on a DSN that
// names no shared file are two databases, so the schema one applied would not
// be in the one the next script queries.
//
// Reading the cache before the credential has the mirror image of that problem:
// "app:migrate" registered after "app" was opened would answer with the pool of
// "app" and run the migrations under the credential it was registered to avoid.
func (r *DatabaseProvider) cached(connector func(string, string) (*sqlx.DB, error), names ...string) (*sqlx.DB, error) {
	name, value, ok := r.resolve(names)
	if !ok {
		return nil, fmt.Errorf("no configuration found for database: %v", names)
	}

	r.mu.Lock()
	db, cached := r.cache[name]
	r.mu.Unlock()
	if cached {
		return db, nil
	}

	db, err := r.with(connector, value)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[name] = db
	r.mu.Unlock()
	return db, nil
}

// resolve returns the first of names that has a credential, with the credential
// itself. No name is the default connection, which is what a caller that named
// none has always meant.
func (r *DatabaseProvider) resolve(names []string) (string, string, bool) {
	if len(names) == 0 {
		names = []string{"default"}
	}

	for _, name := range names {
		if value, ok := r.credential(name); ok {
			return name, value, true
		}
	}
	return "", "", false
}

// with will create a *sqlx.DB from one credential, given the connector
// (sqlx.Connect/Open).
func (r *DatabaseProvider) with(connector func(string, string) (*sqlx.DB, error), value string) (*sqlx.DB, error) {
	driver, dsn := r.parseCredential(value)
	client, err := connector(driver, dsn)
	if err != nil {
		return nil, err
	}

	opt := databaseOption(driver, dsn)
	opt.Apply(client)
	return client, nil
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
