package annotations

import (
	"fmt"
	"io/fs"
	"path"
)

// scanner walks a PHP source tree in path order, reading every .php file it is
// allowed to see. Route and Startup share it: discovery is one responsibility,
// what an annotation means is another.
type scanner struct {
	root     fs.FS
	excluded map[string]struct{}
}

// walk calls fn with the path and source of every scanned .php file.
func (s scanner) walk(fn func(file string, src []byte) error) error {
	if s.root == nil {
		return fmt.Errorf("annotations: nil root filesystem")
	}
	return fs.WalkDir(s.root, ".", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if _, excluded := s.excluded[file]; excluded {
				return fs.SkipDir
			}
			// composer's install tree holds third-party sources. A dependency
			// does not get to publish routes into the application or to run at
			// startup, and walking it costs a parse of every vendored file.
			if entry.Name() == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if path.Ext(file) != ".php" {
			return nil
		}
		src, err := fs.ReadFile(s.root, file)
		if err != nil {
			return err
		}
		return fn(file, src)
	})
}
