package session_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/session"
)

func TestSessionStorageImplementations(t *testing.T) {
	t.Parallel()
	disk, err := session.NewStorageDisk(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewStorageDisk: %v", err)
	}

	storages := map[string]session.Storage{
		"disk":   disk,
		"memory": session.NewStorageMemory(),
	}
	for name, storage := range storages {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			input := []byte("session data")
			if err := storage.Save(ctx, "abc123", input); err != nil {
				t.Fatalf("Save: %v", err)
			}
			input[0] = 'X'

			got, err := storage.Load(ctx, "abc123")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !bytes.Equal(got, []byte("session data")) {
				t.Fatalf("Load = %q, want %q", got, "session data")
			}

			if err := storage.Delete(ctx, "abc123"); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, err := storage.Load(ctx, "abc123"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("Load after Delete error = %v, want fs.ErrNotExist", err)
			}

			if err := storage.Save(ctx, "expired", []byte("data")); err != nil {
				t.Fatalf("Save for prune: %v", err)
			}
			if err := storage.Prune(ctx, -time.Second); err != nil {
				t.Fatalf("Prune: %v", err)
			}
			if _, err := storage.Load(ctx, "expired"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("Load after Prune error = %v, want fs.ErrNotExist", err)
			}
		})
	}
}

// TestSessionStorageDeleteByUser covers the sweep a removed account runs
// through: every session holding that user goes, another user's stays, and the
// count is what the caller reports.
func TestSessionStorageDeleteByUser(t *testing.T) {
	t.Parallel()
	disk, err := session.NewStorageDisk(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewStorageDisk: %v", err)
	}

	storages := map[string]session.Storage{
		"disk":   disk,
		"memory": session.NewStorageMemory(),
	}
	for name, storage := range storages {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			// Two clients of one user, and a second user beside them.
			for id, user := range map[string]string{
				"aaa": "01JBQ0USER1",
				"bbb": "01JBQ0USER1",
				"ccc": "01JBQ0USER2",
			} {
				if err := storage.Save(ctx, id, []byte(user)); err != nil {
					t.Fatalf("Save %s: %v", id, err)
				}
			}

			taken, err := storage.DeleteByUser(ctx, "01JBQ0USER1")
			if err != nil {
				t.Fatalf("DeleteByUser: %v", err)
			}
			if taken != 2 {
				t.Errorf("DeleteByUser took %d sessions, want 2", taken)
			}
			for _, id := range []string{"aaa", "bbb"} {
				if _, err := storage.Load(ctx, id); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("session %s survived the sweep, error = %v", id, err)
				}
			}
			if _, err := storage.Load(ctx, "ccc"); err != nil {
				t.Errorf("the other user's session went with it: %v", err)
			}

			// A user holding none is no error and no count.
			taken, err = storage.DeleteByUser(ctx, "01JBQ0USER1")
			if err != nil || taken != 0 {
				t.Errorf("a second sweep took %d, %v, want 0 and no error", taken, err)
			}
		})
	}
}

func TestSessionStorageHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	storage := session.NewStorageMemory()
	if err := storage.Save(ctx, "id", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
}

func TestSessionStorageDiskRejectsInvalidPathAndID(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewStorageDisk(file); err == nil {
		t.Fatal("NewStorageDisk with file path returned nil error")
	}

	storage, err := session.NewStorageDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Save(context.Background(), "../outside", nil); err == nil {
		t.Fatal("Save with path-traversing ID returned nil error")
	}
}
