package session_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/session"
)

// TestStorageMemory_Load covers the copy Load returns. The data lives in a map
// the storage owns, so handing out the stored slice would let a caller mutate
// another request's session through it.
func TestStorageMemory_Load(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	ctx := context.Background()

	if err := storage.Save(ctx, "abc", []byte("stored")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	first, err := storage.Load(ctx, "abc")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !bytes.Equal(first, []byte("stored")) {
		t.Fatalf("Load = %q, want %q", first, "stored")
	}

	first[0] = 'X'
	second, err := storage.Load(ctx, "abc")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !bytes.Equal(second, []byte("stored")) {
		t.Errorf("Load = %q after the caller wrote to the first copy, want %q", second, "stored")
	}

	// An ID nothing saved reports fs.ErrNotExist, the same error the disk
	// backend returns, so a manager reads one condition from either storage.
	if _, err := storage.Load(ctx, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load of an unknown ID = %v, want fs.ErrNotExist", err)
	}
}

// TestStorageMemory_Delete covers logging a session out. Deleting an ID that is
// not there reports fs.ErrNotExist, so a caller can tell a session it removed
// from one that had already gone.
func TestStorageMemory_Delete(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	ctx := context.Background()

	if err := storage.Save(ctx, "abc", []byte("stored")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := storage.Delete(ctx, "abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := storage.Load(ctx, "abc"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load after Delete = %v, want fs.ErrNotExist", err)
	}
	if err := storage.Delete(ctx, "abc"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a second Delete = %v, want fs.ErrNotExist", err)
	}
}

// TestStorageMemory_Prune covers the sweep that bounds how long the map grows.
// Prune measures age from the last Save, so a session written again survives a
// sweep that would have taken the version before it.
func TestStorageMemory_Prune(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	ctx := context.Background()

	for _, id := range []string{"old", "fresh"} {
		if err := storage.Save(ctx, id, []byte(id)); err != nil {
			t.Fatalf("Save(%s): %v", id, err)
		}
	}

	// A maxAge longer than the test has been running keeps everything.
	if err := storage.Prune(ctx, time.Hour); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	for _, id := range []string{"old", "fresh"} {
		if _, err := storage.Load(ctx, id); err != nil {
			t.Errorf("Load(%s) after a one hour Prune = %v, want the session", id, err)
		}
	}

	// A negative maxAge puts the cutoff in the future, so every session is past
	// it and Prune empties the map.
	if err := storage.Prune(ctx, -time.Second); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	for _, id := range []string{"old", "fresh"} {
		if _, err := storage.Load(ctx, id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Load(%s) after Prune = %v, want fs.ErrNotExist", id, err)
		}
	}
}

// TestStorageMemory_DeleteByUser covers the sweep a removed account runs
// through: the scan matches on the payload, where the user is, and another
// user's session stays.
func TestStorageMemory_DeleteByUser(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	ctx := context.Background()
	if err := storage.Save(ctx, "abc", []byte("user-42")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := storage.Save(ctx, "def", []byte("user-7")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	taken, err := storage.DeleteByUser(ctx, "user-42")
	if err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if taken != 1 {
		t.Errorf("DeleteByUser took %d sessions, want 1", taken)
	}
	if _, err := storage.Load(ctx, "abc"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load after the sweep = %v, want fs.ErrNotExist", err)
	}
	if _, err := storage.Load(ctx, "def"); err != nil {
		t.Errorf("the other user's session went with it: %v", err)
	}
}

// TestStorageMemoryHonorsACanceledContext covers the check each method opens
// with. A request that has gone away does not write a session, so the backend
// reports the cancellation and leaves the map alone.
func TestStorageMemoryHonorsACanceledContext(t *testing.T) {
	t.Parallel()
	storage := session.NewStorageMemory()
	if err := storage.Save(context.Background(), "abc", []byte("stored")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for name, call := range map[string]func() error{
		"Load":   func() error { _, err := storage.Load(ctx, "abc"); return err },
		"Save":   func() error { return storage.Save(ctx, "abc", nil) },
		"Delete": func() error { return storage.Delete(ctx, "abc") },
		"Prune":  func() error { return storage.Prune(ctx, -time.Second) },
	} {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s = %v, want context.Canceled", name, err)
		}
	}

	// A cancelled context removes nothing.
	if _, err := storage.Load(context.Background(), "abc"); err != nil {
		t.Errorf("Load = %v, want the session a cancelled Delete left alone", err)
	}
}
