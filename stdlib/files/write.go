package files

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
//
// A path outside writable_paths is the exception to that: it returns an error,
// which the runtime turns into a catchable exception. A write the operating
// system refused is a runtime condition a script handles by checking the
// result; a write the configuration never allowed is a mistake worth raising.
func registerWrites(rt *runner.Runtime, r root) {
	rt.RegisterFunc("mkdir", func(p string, _ ...any) (bool, error) {
		name, err := r.resolveWrite("mkdir", p)
		if err != nil {
			return false, err
		}
		return os.MkdirAll(name, 0o755) == nil, nil
	})
	rt.RegisterFunc("unlink", func(p string) (bool, error) {
		name, err := r.resolveWrite("unlink", p)
		if err != nil {
			return false, err
		}
		return os.Remove(name) == nil, nil
	})
	rt.RegisterFunc("touch", func(p string, mtime ...int64) (bool, error) {
		name, err := r.resolveWrite("touch", p)
		if err != nil {
			return false, err
		}
		f, err := os.OpenFile(name, os.O_CREATE, 0o644)
		if err != nil {
			return false, nil
		}
		if err := f.Close(); err != nil {
			return false, nil
		}

		t := time.Now()
		if len(mtime) > 0 {
			t = time.Unix(mtime[0], 0)
		}
		return os.Chtimes(name, t, t) == nil, nil
	})

	// rename modifies both ends: it removes the source as well as creating the
	// destination, so both have to be writable.
	rt.RegisterFunc("rename", func(from, to string) (bool, error) {
		src, err := r.resolveWrite("rename", from)
		if err != nil {
			return false, err
		}
		dst, err := r.resolveWrite("rename", to)
		if err != nil {
			return false, err
		}
		return moveFile(src, dst) == nil, nil
	})
	// copy only reads the source, so only the destination is checked.
	rt.RegisterFunc("copy", func(from, to string) (bool, error) {
		dst, err := r.resolveWrite("copy", to)
		if err != nil {
			return false, err
		}
		return copyFile(r.resolve(from), dst) == nil, nil
	})

	// A PHP mode argument is a raw Unix mode, usually written as the octal
	// literal 0644, and runner.FileMode is the same number: both ends of the
	// configuration and the script agree on what a mode is.
	rt.RegisterFunc("chmod", func(p string, mode int64) (bool, error) {
		name, err := r.resolveWrite("chmod", p)
		if err != nil {
			return false, err
		}
		return os.Chmod(name, runner.FileMode(mode).Mode()) == nil, nil
	})
	// PHP takes either a name or a numeric id for both of these, and leaves the
	// other half of the ownership alone, which is what -1 means to Chown.
	rt.RegisterFunc("chown", func(p string, owner any) (bool, error) {
		name, err := r.resolveWrite("chown", p)
		if err != nil {
			return false, err
		}
		uid, err := lookupUser(owner)
		if err != nil {
			return false, nil
		}
		return os.Chown(name, uid, -1) == nil, nil
	})
	rt.RegisterFunc("chgrp", func(p string, group any) (bool, error) {
		name, err := r.resolveWrite("chgrp", p)
		if err != nil {
			return false, err
		}
		gid, err := lookupGroup(group)
		if err != nil {
			return false, nil
		}
		return os.Chown(name, -1, gid) == nil, nil
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
