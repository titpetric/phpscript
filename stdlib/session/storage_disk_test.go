package session_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/session"
)

// TestStorageDisk_Delete covers logging a session out. Delete removes the file,
// so a later Load reports fs.ErrNotExist and a second Delete does too: the
// manager reads the same condition from either backend.
func TestStorageDisk_Delete(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "sessions")
	storage, err := session.NewStorageDisk(dir)
	if err != nil {
		t.Fatalf("NewStorageDisk: %v", err)
	}
	ctx := context.Background()

	if err := storage.Save(ctx, "abc", []byte("stored")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "abc")); err != nil {
		t.Fatalf("Stat after Save: %v", err)
	}

	if err := storage.Delete(ctx, "abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "abc")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat after Delete = %v, want fs.ErrNotExist", err)
	}
	if _, err := storage.Load(ctx, "abc"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load after Delete = %v, want fs.ErrNotExist", err)
	}
	if err := storage.Delete(ctx, "abc"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a second Delete = %v, want fs.ErrNotExist", err)
	}

	// An ID holding a path separator names a file outside the directory, and
	// Delete refuses it before removing anything.
	if err := storage.Delete(ctx, "../outside"); err == nil {
		t.Error("Delete accepted a path-traversing ID")
	}
}

// TestStorageDiskPruneSweepsByModificationTime covers the sweep that bounds how
// large the directory grows. A session rewritten on every request keeps a
// current modification time, so Prune reads age off the file and never off the
// ID.
//
// Prune skips the temporary files Save writes, so a sweep running beside a save
// cannot remove the file that save is about to rename into place.
func TestStorageDiskPruneSweepsByModificationTime(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "sessions")
	storage, err := session.NewStorageDisk(dir)
	if err != nil {
		t.Fatalf("NewStorageDisk: %v", err)
	}
	ctx := context.Background()

	if err := storage.Save(ctx, "abc", []byte("stored")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	inFlight := filepath.Join(dir, ".session-123")
	if err := os.WriteFile(inFlight, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	// A maxAge longer than the test has been running keeps the session.
	if err := storage.Prune(ctx, time.Hour); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := storage.Load(ctx, "abc"); err != nil {
		t.Errorf("Load after a one hour Prune = %v, want the session", err)
	}

	// A negative maxAge puts the cutoff in the future, so the session is past it.
	if err := storage.Prune(ctx, -time.Second); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := storage.Load(ctx, "abc"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load after Prune = %v, want fs.ErrNotExist", err)
	}
	if _, err := os.Stat(inFlight); err != nil {
		t.Errorf("Stat of the in-flight temporary file = %v, want it left alone", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("Stat of the subdirectory = %v, want it left alone", err)
	}
}
