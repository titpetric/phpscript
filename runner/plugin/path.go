package plugin

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Resolve returns the absolute path of the plugin named by name.
//
// An absolute name is used as given. A relative one is looked for beside base,
// then under the module root, then under the working directory, and the first
// that exists wins; the three candidates are what make one spelling work from
// every directory the tests run from. A name without a .so suffix names a
// directory holding plugin.so. base is the file that named the plugin, not a
// directory, and an empty base means the working directory.
func Resolve(base, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("plugin: empty name")
	}
	name = filepath.Clean(name)
	if !strings.HasSuffix(name, ".so") {
		name = filepath.Join(name, "plugin.so")
	}

	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err != nil {
			return "", fmt.Errorf("plugin %q: %w", name, err)
		}
		return name, nil
	}

	var tried []string
	for _, dir := range searchDirs(base) {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Abs(candidate)
		}
		tried = append(tried, candidate)
	}
	return "", fmt.Errorf("plugin %q: %w (tried %s)", name, fs.ErrNotExist, strings.Join(tried, ", "))
}

// searchDirs returns the directories a relative plugin name is looked for in,
// in order, with duplicates removed so a failure lists each path once.
func searchDirs(base string) []string {
	var dirs []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		if slices.Contains(dirs, dir) {
			return
		}
		dirs = append(dirs, dir)
	}

	baseDir := ""
	if base != "" {
		baseDir = filepath.Dir(base)
		add(baseDir)
	}
	add(moduleRoot(baseDir))
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
		add(moduleRoot(cwd))
	}
	return dirs
}

// moduleRoot returns the first directory at or above dir that holds a go.mod,
// or "" when there is none. An empty dir starts from the working directory.
func moduleRoot(dir string) string {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		dir = cwd
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
