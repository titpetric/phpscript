package list

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExpandFiles resolves path arguments to a sorted list of PHP file paths.
// Path forms match Go's package patterns:
//
//   - "." or "dir": .php/.phpt files in that directory only (non-recursive)
//   - "./..." or "dir/...": that directory and all subdirectories
//   - "file.php": that file
//
// When paths is empty, "." is used.
func ExpandFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	seen := map[string]struct{}{}
	var out []string
	for _, p := range paths {
		files, err := expandOne(p)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if _, ok := seen[f]; ok {
				continue
			}
			seen[f] = struct{}{}
			out = append(out, f)
		}
	}
	return out, nil
}

func expandOne(path string) ([]string, error) {
	recursive := false
	if strings.HasSuffix(path, "/...") {
		recursive = true
		path = strings.TrimSuffix(path, "/...")
		if path == "" {
			path = "."
		}
	} else if path == "..." {
		recursive = true
		path = "."
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !isPHP(path) {
			return nil, fmt.Errorf("%s: not a PHP file", path)
		}
		return []string{filepath.Clean(path)}, nil
	}

	if recursive {
		return walkRecursive(path)
	}
	return listDir(path)
}

func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if isPHP(p) {
			out = append(out, filepath.Clean(p))
		}
	}
	return out, nil
}

func walkRecursive(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// vendor/ is composer's install tree: third-party sources, not the
			// application's own files, and listing them buries the project.
			if d.Name() == ".git" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if isPHP(p) {
			out = append(out, filepath.Clean(p))
		}
		return nil
	})
	return out, err
}

func isPHP(path string) bool {
	return strings.HasSuffix(path, ".php") || strings.HasSuffix(path, ".phpt")
}
