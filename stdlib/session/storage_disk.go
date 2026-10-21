package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/files"
)

// StorageDisk stores sessions in a local folder.
type StorageDisk struct {
	storagePath string
}

// newRootedStorageDisk is the Session\Storage\Disk constructor a rooted host
// registers.
//
// The runtime's own rule resolves a path the script named against the root, so
// it names the same file file_get_contents would and cannot climb out, and the
// constructor then holds it to writable_paths. The directory it creates is one
// the script could have written a file into by hand.
//
// With no path it falls through to the host's temporary directory, which is
// outside the root by decision: no script can name it, so no tenant chose it.
// That is the spelling tests/fixtures/bindings/session_manager.phpt uses, and
// DefaultStoragePath keeps two roots out of one directory there.
func newRootedStorageDisk(rt *runner.Runtime, dir string, storagePaths ...string) (*StorageDisk, error) {
	if len(storagePaths) == 0 || storagePaths[0] == "" {
		return NewStorageDisk(DefaultStoragePath(dir))
	}

	target := files.HostPath(rt, dir, storagePaths[0])
	writable := files.WritableRoots(dir, rt.WritablePaths())
	if len(writable) > 0 {
		allowed := false
		for _, w := range writable {
			if files.Within(target, w) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("Session\\Storage\\Disk: %s is outside writable_paths", storagePaths[0])
		}
	}
	return NewStorageDisk(target)
}

// DefaultStoragePath answers the directory a Session\Storage\Disk named no path
// writes to, for an application rooted at dir.
//
// The leaf is a digest of the absolute root, so two roots cannot collide and
// the same root answers the same directory across restarts. An empty dir keeps
// the bare directory, and prune() skips subdirectories.
//
// What a shared directory did to a virtual-host server is docs/security.md,
// "Isolation between sites".
func DefaultStoragePath(dir string) string {
	base := filepath.Join(os.TempDir(), "phpscript-sessions")
	if dir == "" {
		return base
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(base, hex.EncodeToString(sum[:8]))
}

// NewStorageDisk creates the storage folder and verifies it accepts a write.
// With no path, it uses DefaultStoragePath for no application root.
//
// NewStorageDisk takes the path as given. A Go host chooses it, so there is
// nothing to confine it against; the path a PHP script names resolves through
// newRootedStorageDisk.
func NewStorageDisk(storagePaths ...string) (*StorageDisk, error) {
	storagePath := DefaultStoragePath("")
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

	return &StorageDisk{storagePath: storagePath}, nil
}

func (s *StorageDisk) sessionPath(id string) (string, error) {
	if id == "" || id == "." || filepath.Base(id) != id {
		return "", fmt.Errorf("invalid session ID %q", id)
	}
	return filepath.Join(s.storagePath, id), nil
}

// Load retrieves a session from disk.
func (s *StorageDisk) Load(ctx context.Context, id string) ([]byte, error) {
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
func (s *StorageDisk) Save(ctx context.Context, id string, data []byte) error {
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
func (s *StorageDisk) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.sessionPath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// DeleteByUser removes every session file holding userID, counting what it
// took. The walk reads each session because the payload is the only place the
// user appears: a file carries its session id as its name and nothing more.
func (s *StorageDisk) DeleteByUser(ctx context.Context, userID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(s.storagePath)
	if err != nil {
		return 0, err
	}

	taken := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return taken, err
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".session-") {
			continue
		}
		path := filepath.Join(s.storagePath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return taken, err
		}
		if string(data) != userID {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return taken, err
		}
		taken++
	}
	return taken, nil
}

// Prune removes sessions whose last save is older than maxAge.
func (s *StorageDisk) Prune(ctx context.Context, maxAge time.Duration) error {
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

var _ Storage = (*StorageDisk)(nil)
