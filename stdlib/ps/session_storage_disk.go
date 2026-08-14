package ps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionStorageDisk stores sessions in a local folder.
type SessionStorageDisk struct {
	storagePath string
}

// NewSessionStorageDisk creates the storage folder and verifies that it is
// writable. With no path, it uses the operating system's temporary directory.
func NewSessionStorageDisk(storagePaths ...string) (*SessionStorageDisk, error) {
	storagePath := filepath.Join(os.TempDir(), "phpscript-sessions")
	if len(storagePaths) > 0 {
		storagePath = storagePaths[0]
	}
	if storagePath == "" {
		return nil, errors.New("session storage path is empty")
	}
	if err := os.MkdirAll(storagePath, 0o700); err != nil {
		return nil, fmt.Errorf("create session storage: %w", err)
	}

	probe, err := os.CreateTemp(storagePath, ".writable-")
	if err != nil {
		return nil, fmt.Errorf("open session storage: %w", err)
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probeName)
		return nil, fmt.Errorf("close session storage probe: %w", err)
	}
	if err := os.Remove(probeName); err != nil {
		return nil, fmt.Errorf("remove session storage probe: %w", err)
	}

	return &SessionStorageDisk{storagePath: storagePath}, nil
}

func (s *SessionStorageDisk) sessionPath(id string) (string, error) {
	if id == "" || id == "." || filepath.Base(id) != id {
		return "", fmt.Errorf("invalid session ID %q", id)
	}
	return filepath.Join(s.storagePath, id), nil
}

// Load retrieves a session from disk.
func (s *SessionStorageDisk) Load(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.sessionPath(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// Save atomically writes a session to disk.
func (s *SessionStorageDisk) Save(ctx context.Context, id string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.sessionPath(id)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.storagePath, ".session-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Delete removes a session from disk.
func (s *SessionStorageDisk) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.sessionPath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Prune removes sessions that have not been saved within maxAge.
func (s *SessionStorageDisk) Prune(ctx context.Context, maxAge time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.storagePath)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".session-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(s.storagePath, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

var _ SessionStorage = (*SessionStorageDisk)(nil)
