// Package database holds the named sql connections a phpscript runtime
// resolves through. It copies the provider in
// github.com/titpetric/platform, which lives in an internal package no
// outside module reaches and that a virtual host can hold a provider of its
// own in place of a process global.
package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
)

// Provider is what a runtime resolves named connections through. model declares
// the interface and a runtime can name it in its options without depending on
// this package.
type Provider = model.DatabaseProvider

// Default is the provider a runtime whose options name none resolves through.
// It comes from the process environment, as a CLI run has it.
var Default Provider = New(os.Environ())

var (
	_ Provider                       = (*DatabaseProvider)(nil)
	_ model.ExtendedDatabaseProvider = (*DatabaseProvider)(nil)
)

// DatabaseProvider holds a list of named sql connection credentials.
type DatabaseProvider struct {
	open Connector

	// root is the directory a relative sqlite path resolves against. Empty
	// leaves the path alone, which resolves against the process working
	// directory as any relative path does.
	root string

	mu          sync.Mutex
	cache       map[string]*DB
	credentials map[string]string
}

// NewDatabaseProvider returns a ready `*DatabaseProvider`.
func NewDatabaseProvider(open Connector) *DatabaseProvider {
	return &DatabaseProvider{
		open:        open,
		cache:       make(map[string]*DB),
		credentials: make(map[string]string, 1),
	}
}

// New returns a provider holding only the connections named in
// environment, in PLATFORM_DB_<NAME>=<dsn> form, plus the built-in
// default. A provider built this way sees that list alone, which
// keeps one virtual host out of another's databases.
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

// NewAt is New with relative sqlite paths resolved against root. A virtual
// host builds its connections through it: the site wrote its DSN against its
// own tree, and the server's working directory is not the site's to know.
func NewAt(root string, environment []string) *DatabaseProvider {
	provider := New(environment)
	provider.root = root
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

// Register adds a named credential to the provider.
//
// A host that keeps its connections in a database registers them per request,
// from whichever goroutine is serving it and one mutex guards the credentials
// map and the pool cache beside it.
//
// Re-registering a name with the same configuration is free. Re-registering it
// with a different one drops the pool of the old credentials: the cache keys
// on the name, and a name that now means a different database must not keep
// answering with the old one.
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
// The context propagates tracing detail and the trace groups the ping right.
func (r *DatabaseProvider) Connect(ctx context.Context, names ...string) (*DB, error) {
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
func (r *DatabaseProvider) Open(_ context.Context, names ...string) (*DB, error) {
	db, err := r.cached(r.open, names...)
	return db, err
}

// cached will return a singleton *DB from a named connection.
//
// The credential picks which name the pool belongs to, and cached reads and
// writes the cache under that one, and not under the first name the caller
// passed.
// Callers name fallbacks: Database\Migrate names "app:migrate" before
// "app", and both mean the credential "app" until a deployment registers the
// first. Keying on the name passed would open that caller a pool of
// its own and every caller naming "app" another, and two pools on a DSN that
// names no shared file are two databases and the schema one applied would not
// be in the one the next script queries.
//
// Reading the cache before the credential has the mirror image of that problem:
// "app:migrate" registered behind an open "app" would answer with the pool of
// "app" and run the migrations under the credential it exists to avoid.
func (r *DatabaseProvider) cached(connector Connector, names ...string) (*DB, error) {
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
// itself. No name is the default connection, which a caller that named
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

// with will create a *DB from one credential, given the connector.
func (r *DatabaseProvider) with(connector Connector, value string) (*DB, error) {
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

	if driver == "sqlite" {
		dsn = resolveSQLiteDSN(r.root, dsn)
	}

	return driver, cleanDSN(driver, dsn)
}
