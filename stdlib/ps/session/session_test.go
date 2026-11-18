package session_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/ps/session"
)

// TestDefaultStoragePathIsPerRoot pins the directory the no-path spelling of
// Session\Storage\Disk resolves to: one of the application's own.
//
// Two roots differing only in their last segment must answer two
// directories. One root must answer the same directory twice, or a restart
// would lose every session it had.
func TestDefaultStoragePathIsPerRoot(t *testing.T) {
	t.Parallel()
	one := session.DefaultStoragePath("/srv/sites/one")
	two := session.DefaultStoragePath("/srv/sites/two")

	if one == two {
		t.Fatalf("two application roots resolve to one directory: %s", one)
	}
	if again := session.DefaultStoragePath("/srv/sites/one"); again != one {
		t.Fatalf("DefaultStoragePath is not stable: %s then %s", one, again)
	}

	// DefaultStoragePath resolves a relative root before digesting it. Two
	// spellings of one tree must resolve to one site, never two.
	if session.DefaultStoragePath(".") != session.DefaultStoragePath(mustAbs(t, ".")) {
		t.Fatal("a relative and an absolute spelling of one root resolve to two directories")
	}

	// A host that bound no root keeps the bare directory, the parent of every
	// scoped directory; it has nothing to key on.
	bare := session.DefaultStoragePath("")
	if filepath.Dir(one) != bare {
		t.Fatalf("scoped directory %s is not below the unrooted one %s", one, bare)
	}
}

// TestPruneDoesNotCrossApplicationRoots is the reason that scoping exists.
//
// prune() takes an age to identify what to remove. Any script that can
// construct the storage can empty it. Once all application roots shared one directory, one line of
// PHP on any site of a virtual-host server deleted sessions belonging to other
// sites. Unguessable session IDs did not help, because that line guessed no ID
// and named no path.
//
// The unrooted host is in the table, because it writes into the parent of the
// scoped directories. Prune must skip those directories, never walking in.
func TestPruneDoesNotCrossApplicationRoots(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	ctx := context.Background()

	open := func(root string) *session.StorageDisk {
		t.Helper()
		storage, err := session.NewStorageDisk(session.DefaultStoragePath(root))
		if err != nil {
			t.Fatalf("NewStorageDisk(%q): %v", root, err)
		}
		return storage
	}

	one, two, unrooted := open("/srv/sites/one"), open("/srv/sites/two"), open("")
	for name, storage := range map[string]*session.StorageDisk{"one": one, "two": two, "unrooted": unrooted} {
		if err := storage.Save(ctx, "a-session-id", []byte(name)); err != nil {
			t.Fatalf("%s Save: %v", name, err)
		}
	}

	// Every site writes the same ID. A collision in one directory
	// turns shared storage into a site reading another's session.
	if err := two.Prune(ctx, 0); err != nil {
		t.Fatalf("two Prune: %v", err)
	}
	if err := unrooted.Prune(ctx, 0); err != nil {
		t.Fatalf("unrooted Prune: %v", err)
	}

	data, err := one.Load(ctx, "a-session-id")
	if err != nil {
		t.Fatalf("one Load after another site pruned: %v", err)
	}
	if string(data) != "one" {
		t.Fatalf("one Load = %q, want %q; the sites share a directory", data, "one")
	}

	// The site that pruned did prune, or the assertion above would hold for a
	// Prune that removes no session.
	if _, err := two.Load(ctx, "a-session-id"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("two Load after its own Prune error = %v, want fs.ErrNotExist", err)
	}
}

// TestRegisteredDiskStorageIsPerRoot drives the path a virtual host takes: the
// no-argument spelling, using the constructor RegisterRoot installs, across
// two application roots.
//
// It asserts on the directories that appear. PHP's printed output is not
// checked. A script cannot observe this class's path; the claim concerns
// where the files land.
func TestRegisteredDiskStorageIsPerRoot(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)

	for _, root := range []string{filepath.Join(temp, "site-one"), filepath.Join(temp, "site-two")} {
		var out strings.Builder
		prog, err := parser.Parse(`<?php $st = new Session\Storage\Disk; $st->save("id", "data");`)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		rt := runner.New(&out, runner.Options{})
		stdlib.Register(rt)
		stdlib.RegisterFS(rt, root)
		if err := rt.Run(prog); err != nil {
			t.Fatalf("run for %s: %v", root, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(temp, "phpscript-sessions"))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	if len(dirs) != 2 {
		t.Fatalf("two application roots produced %d directories (%v), want 2", len(dirs), dirs)
	}
}

// TestRegister covers three classes the stdlib wiring installs, rooted at
// the process working directory as the filesystem shims are. Memory storage
// round-trips through a script call. A manager is constructible over that
// storage; a script reaches the whole surface without a host naming a root.
func TestRegister(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	session.Register(rt)

	prog, err := parser.Parse(`<?php
$memory = new Session\Storage\Memory();
$memory->save("abc", "stored");
echo $memory->load("abc");

$disk = new Session\Storage\Disk();
$disk->save("abc", "on disk");
echo " " . $disk->load("abc");

$manager = new Session\Manager($memory);
echo " " . $manager->session_cookie_name;
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run: %v (output %q)", err, out.String())
	}
	if got, want := out.String(), "stored on disk session"; got != want {
		t.Errorf("echoed %q, want %q", got, want)
	}
}

// TestRegisterRoot covers the confinement a virtual host depends on. A
// $storage_path the script names resolves inside dir and cannot climb out.
// The constructor creates a directory the script could also have written a
// file into.
//
// Before the clamp the constructor made whatever the script named. A site
// could read and write session files anywhere the process could write, while
// file_exists() on the same path answered false.
func TestRegisterRoot(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		path  string
		under string
	}{
		{name: "relative", path: "sessions", under: "sessions"},
		{name: "absolute names the root", path: "/sessions", under: "sessions"},
		{name: "climbing is clamped", path: "../../sessions", under: "sessions"},
		{name: "nested", path: "var/sessions", under: filepath.Join("var", "sessions")},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()

			var out strings.Builder
			rt := runner.New(&out, runner.Options{})
			session.RegisterRoot(rt, root)

			prog, err := parser.Parse(`<?php
$disk = new Session\Storage\Disk(` + strconv.Quote(test.path) + `);
$disk->save("abc", "stored");
echo $disk->load("abc");
`)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := rt.Run(prog); err != nil {
				t.Fatalf("run: %v (output %q)", err, out.String())
			}
			if got := out.String(); got != "stored" {
				t.Errorf("echoed %q, want stored", got)
			}

			// The file landed below the root, at the path the clamp resolves to.
			if _, err := os.Stat(filepath.Join(root, test.under, "abc")); err != nil {
				t.Errorf("Stat: %v, want the session under %s", err, test.under)
			}
		})
	}
}

// TestRegisteredManagerTakesTheDefaults covers the spelling a site writes:
// `new Session\Manager` without arguments, meaning disk storage and the
// application's own .htpasswd. A login is then the manager alone: the script
// names no store and reads no hash.
func TestRegisteredManagerTakesTheDefaults(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	root := t.TempDir()
	passwords := "admin:$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW\n"
	if err := os.WriteFile(filepath.Join(root, ".htpasswd"), []byte(passwords), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runRooted(t, root, `<?php
$session = new Session\Manager;
var_dump($session->login("admin", "hunter2hunter2"));
echo $session->get(), "\n";
var_dump($session->valid());
$session->destroy();
var_dump($session->valid());
var_dump($session->login("admin", "wrong"));
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}

	// The session holds the id computed from the username, the value
	// crypto.ULIDFrom("admin") answers.
	want := "bool(true)\n4CD5VEBDA10GAVVT88QN6YW5EZ\nbool(true)\nbool(false)\nbool(false)\n"
	if out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
}

// TestRegisteredManagerWithoutAPasswordFile covers a site without a user
// database. The default store still constructs, since a manager holds a
// cookie and storage before any login; the login alone fails.
func TestRegisteredManagerWithoutAPasswordFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	out, err := runRooted(t, t.TempDir(), `<?php
$session = new Session\Manager;
$session->start("row-1");
echo $session->get(), "\n";
try {
	$session->login("admin", "hunter2hunter2");
} catch (Throwable $e) {
	echo "no user database\n";
}
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}
	if want := "row-1\nno user database\n"; out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
}

// TestRegisteredManagerDefersThePasswordFileParse covers the cost the default
// store adds to a request: `new Session\Manager` opens the password file and
// parses nothing. A page avoiding login() does not pay for the account list.
// A file with a bad line refuses the login that reads it, naming the line an
// operator fixes.
func TestRegisteredManagerDefersThePasswordFileParse(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	root := t.TempDir()
	passwords := "admin:$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW\nreader:plaintext\n"
	if err := os.WriteFile(filepath.Join(root, ".htpasswd"), []byte(passwords), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runRooted(t, root, `<?php
$session = new Session\Manager;
$session->start("row-1");
echo $session->get(), "\n";
try {
	$session->login("admin", "hunter2hunter2");
} catch (Throwable $e) {
	echo $e->getMessage(), "\n";
}
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}
	if !strings.HasPrefix(out, "row-1\n") {
		t.Errorf("printed %q, want the session before any login", out)
	}
	if want := `line 2: "reader": unreadable password hash`; !strings.Contains(out, want) {
		t.Errorf("printed %q, want the refusal carrying %q", out, want)
	}
}

// TestDefaultStorageFallsBackToMemory covers the degradation and the line that
// reports it.
//
// Memory storage makes a different promise from disk. It lives as long as the
// runtime. A server forks a storage per worker. A session minted on one
// worker is invisible to the next. A login looks like it failed at random.
// A silent fallback would leave the symptom as the only clue; the warning
// names the path and the reason.
//
// This test runs serial: it replaces the default slog logger.
func TestDefaultStorageFallsBackToMemory(t *testing.T) {
	// A regular file in place of the temporary directory: MkdirAll cannot
	// create a directory inside the file. That refusal matches a deployment's read
	// from a read-only or unwritable mount.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", blocked)

	var logged strings.Builder
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	defer slog.SetDefault(previous)

	// The session still works: the server answers a request that signs in;
	// only the durability of its writes drops.
	out, err := runRooted(t, t.TempDir(), `<?php
$session = new Session\Manager;
$session->start("row-1");
echo $session->get(), "\n";
`)
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}
	if out != "row-1\n" {
		t.Errorf("printed %q, want row-1", out)
	}

	line := logged.String()
	for _, want := range []string{"level=WARN", "session disk storage is unavailable", "falling back to memory", "path=", "error="} {
		if !strings.Contains(line, want) {
			t.Errorf("logged %q, want it to carry %q", line, want)
		}
	}
}

// runRooted executes source on a runtime rooted at root, with a request
// context registered: a session needs one, since Start stages a header on the
// request it answers.
func runRooted(t *testing.T, root, source string) (string, error) {
	t.Helper()

	prog, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	stdlib.RegisterFS(rt, root)
	runner.NewContext().Register(rt)
	err = rt.Run(prog)
	return out.String(), err
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
