package sql_test

import (
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/ps/database"
)

func run(t *testing.T, src string) string {
	t.Helper()
	var out strings.Builder
	rt := runner.New(&out, runner.Options{Database: database.New(nil)})
	stdlib.Register(rt)
	program, err := rt.Load("<?php " + src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// TestRegister covers the connection a host registered, opened through the
// namespace, and the refusal of a name nobody registered.
func TestRegister(t *testing.T) {
	got := run(t, `use Runtime\Go\Database\SQL;
Database::register("mem", "sqlite://file:go_sql_test?mode=memory&cache=shared");
$db = SQL\Open("mem");
$db->query("create table t (id integer, name text)");
$db->query("insert into t values (1, 'a')");
echo $db->get("select name from t where id = ?", 1)["name"];
try { SQL\Open("nope"); } catch (Exception $e) { echo "|refused"; }`)
	if got != "a|refused" {
		t.Fatalf("got %q", got)
	}
	if run(t, `echo function_exists('Runtime\Go\Database\SQL\Open') ? "installed" : "none";`) != "none" {
		t.Fatal("the namespace is installed without an import")
	}
}
