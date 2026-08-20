package fs

import (
	"io"
	"os"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// registerWrites installs the write side: creating, removing, moving and
// copying files, and the mode and ownership functions that decide who can read
// what a script just wrote. All of them resolve against the root, and all of
// them report failure the way PHP does, by returning false.
func registerWrites(rt *runner.Runtime, r root) {
	rt.RegisterFunc("mkdir", func(p string, _ ...any) bool {
		return os.MkdirAll(r.resolve(p), 0o755) == nil
	})
	rt.RegisterFunc("unlink", func(p string) bool {
		return os.Remove(r.resolve(p)) == nil
	})
	rt.RegisterFunc("touch", func(p string, mtime ...int64) bool {
		name := r.resolve(p)
		f, err := os.OpenFile(name, os.O_CREATE, 0o644)
		if err != nil {
			return false
		}
		if err := f.Close(); err != nil {
			return false
		}

		t := time.Now()
		if len(mtime) > 0 {
			t = time.Unix(mtime[0], 0)
		}
		return os.Chtimes(name, t, t) == nil
	})

	rt.RegisterFunc("rename", func(from, to string) bool {
		return moveFile(r.resolve(from), r.resolve(to)) == nil
	})
	rt.RegisterFunc("copy", func(from, to string) bool {
		return copyFile(r.resolve(from), r.resolve(to)) == nil
	})

	// A PHP mode argument is a raw Unix mode, usually written as the octal
	// literal 0644, and runner.FileMode is the same number: both ends of the
	// configuration and the script agree on what a mode is.
	rt.RegisterFunc("chmod", func(p string, mode int64) bool {
		return os.Chmod(r.resolve(p), runner.FileMode(mode).Mode()) == nil
	})
	// PHP takes either a name or a numeric id for both of these, and leaves the
	// other half of the ownership alone, which is what -1 means to Chown.
	rt.RegisterFunc("chown", func(p string, owner any) bool {
		uid, err := lookupUser(owner)
		if err != nil {
			return false
		}
		return os.Chown(r.resolve(p), uid, -1) == nil
	})
	rt.RegisterFunc("chgrp", func(p string, group any) bool {
		gid, err := lookupGroup(group)
		if err != nil {
			return false
		}
		return os.Chown(r.resolve(p), -1, gid) == nil
	})
}

// moveFile renames src to dst, copying instead when the two are on different
// filesystems, which is where rename stops working. PHP's rename() crosses that
// boundary, so this one has to as well.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile writes the contents of src to dst, leaving src in place.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
