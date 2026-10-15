package database

import (
	"context"
	"io"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRegister covers the surface a script reaches once the installer has run:
// the Database class, the Database\Migrate class, and the two connection
// statics. Each one is registered under the name a script types, so the
// runtime's own lookup is what answers for it.
func TestRegister(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{Database: New(nil)})
	Register(rt)

	for _, name := range []string{"Database", "Database\\Migrate"} {
		if _, ok := rt.LookupConstructor(name); !ok {
			t.Errorf("new %s resolves to nothing after Register", name)
		}
	}
	for _, name := range []string{"Database::connections", "Database::register"} {
		if _, ok := rt.LookupFunc(name); !ok {
			t.Errorf("%s() resolves to nothing after Register", name)
		}
	}
}

// TestRegisterConnections covers the two statics an application extends its
// connection set with. Connection names normally come from the environment the
// host started with, and an application keeping its connections in a table has
// nowhere else to put them: the provider is built before the first request,
// and putenv writes to the script environment, which no provider reads.
func TestRegisterConnections(t *testing.T) {
	t.Parallel()
	rt := runner.New(io.Discard, runner.Options{Database: New(nil)})
	RegisterConnections(rt)

	if list := connectionsOf(t, rt); len(list) != 1 || list[0] != "default" {
		t.Fatalf("Database::connections() = %v, want the built-in default alone", list)
	}

	register, ok := rt.LookupFunc("Database::register")
	if !ok {
		t.Fatal("Database::register() resolves to nothing")
	}
	added, err := register.(func(string, string) (bool, error))("Shop", "sqlite://:memory:")
	if err != nil {
		t.Fatalf("Database::register(Shop): %v", err)
	}
	if !added {
		t.Error("Database::register() returned false for a name it accepted")
	}

	// The name is lowercased on the way in, so a script writing "Shop" and one
	// writing "shop" reach one connection. The list comes back sorted, so a
	// script can compare it.
	if list := connectionsOf(t, rt); len(list) != 2 || list[0] != "default" || list[1] != "shop" {
		t.Errorf("Database::connections() = %v, want default and shop", list)
	}
}

// TestRegisterMigrate covers the names Database\Migrate resolves against. Each
// name is tried as "<name>:migrate" before "<name>", so a deployment can point
// migrations at a user allowed to alter tables while the script queries
// through one that is not. The project stays the first name the script passed,
// whichever credential answered, and a script naming none migrates "default".
func TestRegisterMigrate(t *testing.T) {
	t.Parallel()
	provider := New(nil)
	provider.Register("shop", "sqlite://:memory:")
	rt := runner.New(io.Discard, runner.Options{Database: provider})
	RegisterMigrate(rt)

	ctor, ok := rt.LookupConstructor("Database\\Migrate")
	if !ok {
		t.Fatal("new Database\\Migrate resolves to nothing after RegisterMigrate")
	}
	build, ok := ctor.(func(context.Context, ...string) (*DatabaseMigrate, error))
	if !ok {
		t.Fatalf("the registered constructor is %T", ctor)
	}

	named, err := build(t.Context(), "shop")
	if err != nil {
		t.Fatalf("new Database\\Migrate(shop): %v", err)
	}
	if named.project != "shop" {
		t.Errorf("project = %q, want %q", named.project, "shop")
	}
	if named.database == nil {
		t.Error("the handle holds no connection")
	}

	unnamed, err := build(t.Context())
	if err != nil {
		t.Fatalf("new Database\\Migrate(): %v", err)
	}
	if unnamed.project != "default" {
		t.Errorf("project = %q for an unnamed handle, want %q", unnamed.project, "default")
	}
}

// connectionsOf calls the Database::connections static and returns the names
// it listed.
func connectionsOf(t *testing.T, rt *runner.Runtime) []string {
	t.Helper()

	connections, ok := rt.LookupFunc("Database::connections")
	if !ok {
		t.Fatal("Database::connections() resolves to nothing")
	}
	list, err := connections.(func() ([]string, error))()
	if err != nil {
		t.Fatalf("Database::connections(): %v", err)
	}
	return list
}
