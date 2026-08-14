package ps_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/titpetric/phpscript/stdlib/ps"
)

func TestSessionStorageImplementations(t *testing.T) {
	disk, err := ps.NewSessionStorageDisk(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatalf("NewSessionStorageDisk: %v", err)
	}

	storages := map[string]ps.SessionStorage{
		"disk":   disk,
		"memory": ps.NewSessionStorageMemory(),
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

func TestSessionStorageHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	storage := ps.NewSessionStorageMemory()
	if err := storage.Save(ctx, "id", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
}

func TestSessionStorageDiskRejectsInvalidPathAndID(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.NewSessionStorageDisk(file); err == nil {
		t.Fatal("NewSessionStorageDisk with file path returned nil error")
	}

	storage, err := ps.NewSessionStorageDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Save(context.Background(), "../outside", nil); err == nil {
		t.Fatal("Save with path-traversing ID returned nil error")
	}
}
