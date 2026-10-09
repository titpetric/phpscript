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
// A path the script named is resolved against the root through the runtime's own
// rule, so it names the same file file_get_contents would and cannot climb out,
// and is then held to writable_paths. The directory it creates is therefore one
// the script could have written a file into by hand.
//
// With no path it falls through to the host's temporary directory, which is
// outside the root by decision: no script can name it, so it is no path a
// tenant chose. That is the spelling tests/fixtures/bindings/session_manager.phpt
// uses, and DefaultStoragePath keeps two roots out of one directory
// there.
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
// One directory per root, and no single one for the process. prune() takes an age and not
// an ID, so with every root sharing a directory, `new Session\Storage\Disk`
// followed by prune() on any site of a virtual-host server deleted every other
// site's sessions. Nothing had to be guessed and no path had to be named, which
// is why rooting the path a script writes does not reach this. The leaf is a
// digest of the absolute root: two roots cannot collide, the same root answers
// the same directory across restarts, and the name tells a reader nothing about
// its neighbours.
//
// An empty dir has nothing to key on and keeps the bare directory. Prune skips
// subdirectories, so a host that bound no root cannot reach a scoped one either.
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

// NewStorageDisk creates the storage folder and verifies that it is writable.
// With no path, it uses DefaultStoragePath for no application root.
//
// The path is taken as given. A Go host chooses it, so there is nothing to
// confine it against; the path a PHP script names resolves through
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

// Prune removes sessions that have not been saved within maxAge.
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
