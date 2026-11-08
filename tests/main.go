package tests

import (
	"log"
	"os"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/ps/database"
)

// TestMain seeds the database connections before any test runs: registration reads one.
func TestMain(m *testing.M) {
	setTestEnv()
	os.Exit(m.Run())
}

func setTestEnv() {
	// The process environment comes first, as it does where the platform
	// package sets up the connections; the test DSNs override it.
	env := append(append([]string{}, os.Environ()...),
		"PLATFORM_DB_SQLITE_TEST=sqlite://file:phpscript-test?mode=memory&cache=shared",
		"PLATFORM_DB_POSTGRES_TEST=postgres://postgres:test@localhost:15432/postgres?sslmode=disable",
		"PLATFORM_DB_MYSQL_TEST=mysql://root:test@tcp(localhost:13306)/mysql",
		// scaffold is also named by tests/fixtures/scaffold/phpscript.yml,
		// the provider a fixture there resolves through. This list repeats it
		// for a caller that reaches the connection without the suite. The
		// list matches .env.testing, the file the command reads.
		"PLATFORM_DB_SCAFFOLD=sqlite://file:phpscript-scaffold?mode=memory&cache=shared",
	)

	database.Default = database.New(env)

	if val, ok := database.Default.(model.ExtendedDatabaseProvider); ok {
		connectionList := val.List()
		log.Println("connections", len(connectionList))
		for k, v := range connectionList {
			log.Println(k, v)
		}
	}
}
