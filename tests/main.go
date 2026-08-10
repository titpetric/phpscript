package tests

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	setTestEnv()
	os.Exit(m.Run())
}

func setTestEnv() {
	env := map[string]string{
		"DB_DSN_SQLITE_TEST":   "sqlite://file:phpscript-test?mode=memory&cache=shared",
		"DB_DSN_POSTGRES_TEST": "postgres://postgres:test@localhost:15432/postgres?sslmode=disable",
		"DB_DSN_MYSQL_TEST":    "mysql://root:test@tcp(localhost:13306)/mysql",
	}
	for key, value := range env {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}
}
