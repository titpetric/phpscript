package session_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
	"github.com/titpetric/phpscript/stdlib/session"
)

// TestDefaultStoragePathIsPerRoot pins what the no-path spelling of
// Session\Storage\Disk resolves to: a directory of the application's own.
//
// Two roots that differ only in their last segment have to answer two
// directories, and one root has to answer the same directory twice, or a restart
// would lose every session it had.
func TestDefaultStoragePathIsPerRoot(t *testing.T) {
	one := session.DefaultStoragePath("/srv/sites/one")
	two := session.DefaultStoragePath("/srv/sites/two")

	if one == two {
		t.Fatalf("two application roots resolve to one directory: %s", one)
	}
	if again := session.DefaultStoragePath("/srv/sites/one"); again != one {
		t.Fatalf("DefaultStoragePath is not stable: %s then %s", one, again)
	}

	// A relative root is resolved before it is digested, so the same tree named
	// two ways is one site rather than two.
	if session.DefaultStoragePath(".") != session.DefaultStoragePath(mustAbs(t, ".")) {
		t.Fatal("a relative and an absolute spelling of one root resolve to two directories")
	}

	// A host that bound no root has nothing to key on and keeps the bare
	// directory, which is the parent of every scoped one.
	bare := session.DefaultStoragePath("")
	if filepath.Dir(one) != bare {
		t.Fatalf("scoped directory %s is not below the unrooted one %s", one, bare)
	}
}

// TestPruneDoesNotCrossApplicationRoots is the reason that scoping exists.
//
// prune() takes an age, not an ID, so any script that can construct the storage
// can empty it. While every application root shared one directory, one line of
// PHP on any site of a virtual-host server deleted every other site's sessions;
// session IDs being unguessable did not help, because nothing had to be guessed
// and no path had to be named.
//
// The unrooted host is in the table because it writes into the parent of the
// scoped directories, so Prune has to skip them rather than walk in.
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

	// Every site writes the same ID deliberately: a collision in one directory
	// is what turns shared storage into one site reading another's session.
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
	// Prune that does nothing at all.
	if _, err := two.Load(ctx, "a-session-id"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("two Load after its own Prune error = %v, want fs.ErrNotExist", err)
	}
}

// TestRegisteredDiskStorageIsPerRoot drives the path a virtual host takes: the
// no-argument spelling, through the constructor RegisterRoot installs, on two
// application roots.
//
// It asserts on the directories that appear rather than on what PHP prints,
// because the path is the one thing about this class a script cannot observe and
// the whole claim is about where the files land.
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

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
