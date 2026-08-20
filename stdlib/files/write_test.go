package files_test

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

// TestRenameAndCopy covers the two ways a script relocates a file it owns. Both
// resolve their arguments against the root, so a script names them the way it
// names everything else, and both answer with a bool rather than an error.
func TestRenameAndCopy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
if (rename("a.txt", "b.txt")) { echo "renamed"; } else { echo "rename-failed"; }
if (file_exists("a.txt")) { echo "|source-left"; } else { echo "|source-gone"; }
if (copy("b.txt", "sub/c.txt")) { echo "|copied"; } else { echo "|copy-failed"; }
if (rename("missing.txt", "d.txt")) { echo "|moved-nothing"; } else { echo "|no-source"; }`)

	// The copy targets a directory that does not exist, which fails in PHP too.
	want := "renamed|source-gone|copy-failed|no-source"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	stored, err := os.ReadFile(filepath.Join(root, "b.txt"))
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	if string(stored) != "content" {
		t.Fatalf("renamed file content = %q", stored)
	}
}

// TestCopyKeepsSource pins the difference between copy() and rename(): a copy
// leaves the original where it was.
func TestCopyKeepsSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
if (copy("a.txt", "b.txt")) { echo "copied"; } else { echo "failed"; }
echo "|" . file_get_contents("a.txt") . "|" . file_get_contents("b.txt");`)

	if want := "copied|content|content"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestChmod checks that a mode written the way PHP writes it, as an octal
// literal, reaches the file unchanged.
func TestChmod(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "a.txt")
	if err := os.WriteFile(name, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
if (chmod("a.txt", 0640)) { echo "ok"; } else { echo "failed"; }
if (chmod("missing.txt", 0640)) { echo "|missing-ok"; } else { echo "|missing-failed"; }`)
	if want := "ok|missing-failed"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}

	st, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %o, want %o", got, 0o640)
	}
}

// TestChownSelf checks ownership by name and by id. Changing a file to the
// owner it already has is the one chown any user is allowed to make, so this
// runs the same whether or not the tests are root.
func TestChownSelf(t *testing.T) {
	me, err := user.Current()
	if err != nil {
		t.Skipf("current user: %v", err)
	}
	uid, err := strconv.Atoi(me.Uid)
	if err != nil {
		t.Skipf("non-numeric uid %q", me.Uid)
	}

	root := t.TempDir()
	name := filepath.Join(root, "a.txt")
	if err := os.WriteFile(name, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
if (chown("a.txt", `+strconv.Itoa(uid)+`)) { echo "by-id"; } else { echo "by-id-failed"; }
if (chown("a.txt", "`+me.Username+`")) { echo "|by-name"; } else { echo "|by-name-failed"; }
if (chown("a.txt", "no-such-user-here")) { echo "|unknown-ok"; } else { echo "|unknown-failed"; }
if (chown("missing.txt", `+strconv.Itoa(uid)+`)) { echo "|missing-ok"; } else { echo "|missing-failed"; }`)

	want := "by-id|by-name|unknown-failed|missing-failed"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestChgrpSelf is TestChownSelf for the group half of the ownership.
func TestChgrpSelf(t *testing.T) {
	me, err := user.Current()
	if err != nil {
		t.Skipf("current user: %v", err)
	}
	gid, err := strconv.Atoi(me.Gid)
	if err != nil {
		t.Skipf("non-numeric gid %q", me.Gid)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
if (chgrp("a.txt", `+strconv.Itoa(gid)+`)) { echo "by-id"; } else { echo "by-id-failed"; }
if (chgrp("a.txt", "no-such-group-here")) { echo "|unknown-ok"; } else { echo "|unknown-failed"; }`)

	if want := "by-id|unknown-failed"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestWritesStayInRoot pins the confinement: a relative path that climbs out of
// the root is cleaned back into it, so a script cannot write above the
// directory the host bound.
func TestWritesStayInRoot(t *testing.T) {
	outside := t.TempDir()
	root := filepath.Join(outside, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	out := runFS(t, root, nil, `<?php
if (touch("../escaped.txt")) { echo "touched"; } else { echo "failed"; }`)
	if out != "touched" {
		t.Fatalf("got %q", out)
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("wrote outside the root: err = %v, want not-exist", err)
	}
	if _, err := os.Stat(filepath.Join(root, "escaped.txt")); err != nil {
		t.Fatalf("stat inside the root: %v", err)
	}
}
