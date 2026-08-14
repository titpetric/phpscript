package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
)

// Storage persists completed request traces beyond process restarts and
// returns them for permalinks. Put stores one completed Request under its
// ULID; Get returns the stored record as a stream so a handler can carry
// it straight into the HTTP response body without decoding.
type Storage interface {
	Put(ctx context.Context, record *Request) error
	Get(ctx context.Context, id string) (io.ReadCloser, error)
}

// ulidRe matches the Crockford base32 ULIDs issued by newULID. Validation
// happens before any path is built, so no request id can escape the
// storage directory.
var ulidRe = regexp.MustCompile(`^[0-9ABCDEFGHJKMNPQRSTVWXYZ]{26}$`)

// DiskStorage stores one <ULID>.json per record under a directory. The
// directory may live on tmpfs (e.g. /dev/shm/phpscript-trace-detail) so
// records survive restarts without disk I/O; cleanup is left to an
// external job over plain files, as ULID filenames sort by time.
type DiskStorage struct {
	dir string
}

// NewDiskStorage creates the directory (0700) if needed and returns the
// driver.
func NewDiskStorage(dir string) (*DiskStorage, error) {
	if dir == "" {
		return nil, fmt.Errorf("status: disk storage requires a path")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("status: disk storage: %w", err)
	}
	return &DiskStorage{dir: dir}, nil
}

func (d *DiskStorage) path(id string) (string, error) {
	if !ulidRe.MatchString(id) {
		return "", fmt.Errorf("status: invalid record id %q", id)
	}
	return filepath.Join(d.dir, id+".json"), nil
}

// Put writes the record atomically (temp file + rename): a permalink
// either sees the whole record or a 404, never a torn file.
func (d *DiskStorage) Put(ctx context.Context, record *Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dst, err := d.path(record.ID)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d.dir, record.ID+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := json.NewEncoder(tmp).Encode(record); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dst)
}

// Get returns the stored record file as an io.ReadCloser. The caller may
// copy it directly into the response body (the ideal state for JSON
// permalinks) or decode it for HTML rendering.
func (d *DiskStorage) Get(ctx context.Context, id string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := d.path(id)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// sampled reports whether one trace should be written out, for a sampling
// percentage between 0 and 100. The trace is always collected in memory;
// sampling only gates the storage write.
func sampled(percent float64) bool {
	if percent >= 100 {
		return true
	}
	if percent <= 0 {
		return false
	}
	return rand.Float64()*100 < percent
}
