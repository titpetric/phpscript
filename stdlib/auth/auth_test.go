package auth_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/auth"
)

// bcryptLine is one stored hash of "hunter2hunter2", written by
//
//	php -r 'echo password_hash("hunter2hunter2", PASSWORD_BCRYPT, ["cost" => 4]);'
//
// The package's own tests cover the format table; these need one line a login
// can succeed against.
const bcryptLine = "admin:$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW"

// run executes source against a runtime whose stores resolve inside root, and
// answers what the script printed. It mounts the whole standard library,
// because the assertions read var_dump and the registration under test is the
// one stdlib.RegisterFS performs.
func run(t *testing.T, root, source string) (string, error) {
	t.Helper()

	prog, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)
	err = rt.Run(prog)
	return out.String(), err
}

// write lays down a password file inside root and answers root.
func write(t *testing.T, name string, lines ...string) string {
	t.Helper()

	root := t.TempDir()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestRegisterReadsTheDefaultFile covers the spelling a site uses: no
// argument, meaning .htpasswd in the application root. Session\Manager takes
// the same spelling by default and the whole of a login is the manager.
func TestRegisterReadsTheDefaultFile(t *testing.T) {
	t.Parallel()

	root := write(t, auth.DefaultPasswordFile, bcryptLine)
	out, err := run(t, root, `<?php
$users = new Auth\Storage\PasswordFile;
var_dump($users->exists());
$user = $users->authenticate("admin", "hunter2hunter2");
echo $user->get_id(), " ", $user, "\n";
var_dump($users->authenticate("admin", "wrong"));
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}

	// The id is the one crypto.ULIDFrom computes for the name, and `echo
	// $user` prints the username with no method call, through the record's
	// String.
	want := "bool(true)\n4CD5VEBDA10GAVVT88QN6YW5EZ admin\nNULL\n"
	if out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
}

// TestRegisterRoot covers the boundary a virtual host rests on: a path a
// script names resolves inside the application root and no spelling climbs out
// of it, which is the rule every filesystem binding follows.
//
// It installs the area directly, and not through stdlib.RegisterFS, because
// what is under test is the registration this package performs.
func TestRegisterRoot(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ name, spelled, at string }{
		{name: "relative", spelled: "users.htpasswd", at: "users.htpasswd"},
		{name: "absolute names the root", spelled: "/users.htpasswd", at: "users.htpasswd"},
		{name: "climbing is clamped", spelled: "../../users.htpasswd", at: "users.htpasswd"},
		{name: "nested", spelled: "config/users", at: filepath.Join("config", "users")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			prog, err := parser.Parse(`<?php
$users = new Auth\Storage\PasswordFile(` + strconv.Quote(test.spelled) + `);
echo $users->authenticate("admin", "hunter2hunter2");
`)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			var out strings.Builder
			rt := runner.New(&out, runner.Options{})
			auth.RegisterRoot(rt, write(t, test.at, bcryptLine))
			if err := rt.Run(prog); err != nil {
				t.Fatalf("run: %v (output %q)", err, out.String())
			}
			if out.String() != "admin" {
				t.Errorf("printed %q, want admin", out.String())
			}
		})
	}
}

// TestRegisterMissingFile covers a tree with no user database. The store
// constructs and a default one cannot fail a site that has no login at all,
// and exists() is the question that answers for it.
func TestRegisterMissingFile(t *testing.T) {
	t.Parallel()

	out, err := run(t, t.TempDir(), `<?php
$users = new Auth\Storage\PasswordFile;
var_dump($users->exists());
try {
	$users->authenticate("admin", "hunter2hunter2");
} catch (Throwable $e) {
	echo "refused\n";
}
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}
	if want := "bool(false)\nrefused\n"; out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
}

// TestRegisterRefusesAFileItCannotRead covers the loud failure: the
// constructor refuses a file carrying a line in no format this build reads,
// names that line, and raises an ordinary catchable error.
func TestRegisterRefusesAFileItCannotRead(t *testing.T) {
	t.Parallel()

	root := write(t, auth.DefaultPasswordFile, bcryptLine, "reader:plaintext")
	out, err := run(t, root, `<?php
try {
	new Auth\Storage\PasswordFile;
} catch (Throwable $e) {
	echo $e->getMessage(), "\n";
}
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}
	if !strings.Contains(out, "line 2") || !strings.Contains(out, "unreadable password hash") {
		t.Errorf("printed %q, want the line and what to write instead", out)
	}
}

// TestRegisterIsTheProcessRoot covers Register itself, the installer
// stdlib.Register runs before a host names a root: the store resolves against
// the working directory, the way the filesystem shims do.
func TestRegisterIsTheProcessRoot(t *testing.T) {
	t.Chdir(write(t, auth.DefaultPasswordFile, bcryptLine))

	prog, err := parser.Parse(`<?php
$users = new Auth\Storage\PasswordFile;
echo $users->authenticate("admin", "hunter2hunter2");
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	auth.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}
	if out.String() != "admin" {
		t.Errorf("printed %q, want admin", out.String())
	}
}
