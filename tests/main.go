package tests

import (
	"log"
	"os"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/platform"
)

func TestMain(m *testing.M) {
	setTestEnv()
	os.Exit(m.Run())
}

func setTestEnv() {
	env := []string{
		"PLATFORM_DB_SQLITE_TEST=sqlite://file:phpscript-test?mode=memory&cache=shared",
		"PLATFORM_DB_POSTGRES_TEST=postgres://postgres:test@localhost:15432/postgres?sslmode=disable",
		"PLATFORM_DB_MYSQL_TEST=mysql://root:test@tcp(localhost:13306)/mysql",
	}

	platform.SetupConnections(env)

	if val, ok := platform.Database.(model.ExtendedDatabaseProvider); ok {
		connectionList := val.List()
		log.Println("connections", len(connectionList))
		for k, v := range connectionList {
			log.Println(k, v)
		}
	}
}
