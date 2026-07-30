package list_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/list"
)

func TestFileRoutesAndClasses(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "include", "Database.php")
	mustWrite(t, db, `<?php
namespace App;

class Database
{
}
`)
	routeFile := filepath.Join(dir, "get-user-profile.php")
	mustWrite(t, routeFile, `<?php
// @route GET /users/{id}
echo $_PATH["id"];
`)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	rows, err := list.Paths([]string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	md := list.Markdown(rows)
	if !strings.Contains(md, "| Route | Filename | Classes |") {
		t.Fatalf("missing header: %s", md)
	}
	if !strings.Contains(md, "App\\Database") && !strings.Contains(md, `App\Database`) {
		t.Fatalf("missing class: %s", md)
	}
	if !strings.Contains(md, "GET /users/{id}") {
		t.Fatalf("missing route: %s", md)
	}
	if !strings.Contains(md, "[include/Database.php](./include/Database.php)") {
		t.Fatalf("missing linked filename: %s", md)
	}
	if !strings.Contains(md, "<none>") {
		t.Fatalf("expected <none> placeholder: %s", md)
	}
}

func TestUnsupportedExtendsStillListsClass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Child.php")
	mustWrite(t, path, `<?php
namespace App\Model;

class Child extends Parent
{
}
`)
	rows, err := list.File(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Classes != `App\Model\Child` {
		t.Fatalf("rows = %+v", rows)
	}
}
