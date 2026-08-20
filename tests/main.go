package tests

import (
	"log"
	"os"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/stdlib/database"
)

func TestMain(m *testing.M) {
	setTestEnv()
	os.Exit(m.Run())
}

func setTestEnv() {
	// The process environment comes first, as it did when the connections
	// were set up by the platform package; the test DSNs override it.
	env := append(append([]string{}, os.Environ()...),
		"PLATFORM_DB_SQLITE_TEST=sqlite://file:phpscript-test?mode=memory&cache=shared",
		"PLATFORM_DB_POSTGRES_TEST=postgres://postgres:test@localhost:15432/postgres?sslmode=disable",
		"PLATFORM_DB_MYSQL_TEST=mysql://root:test@tcp(localhost:13306)/mysql",
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
