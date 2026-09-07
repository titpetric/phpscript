package config

import (
	"fmt"
	"os"

	yaml "github.com/goccy/go-yaml"
)

// Overlay reads filename over base and returns the configuration it describes,
// together with the keys the file itself named.
//
// It is the layering every phpscript.yml gets: the file is unmarshalled over an
// already populated struct, so it only has to name what it changes and inherits
// the rest. A virtual host reads its site's file over the operator's, and a test
// suite reads its folder's file over the one the run started with.
//
// A missing file is an error rather than a fall back to base. The file is what
// the caller went looking for, and a tree served or tested under settings its
// author never wrote is the failure mode worth avoiding.
//
// forbidden names the keys the file may not set, and owned is the clause saying
// who sets them instead. They are rejected rather than dropped, so nobody is
// left believing a key took effect. The declared map is what tells a key the
// file named from one it inherited, which is what a check on an inherited
// default would otherwise get wrong.
func Overlay(base Config, filename string, forbidden []string, owned string) (Config, map[string]any, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return base, nil, err
	}
	return OverlayBytes(base, filename, data, forbidden, owned)
}

// OverlayBytes is Overlay over source already read. A caller holding the file
// as bytes has its own filesystem to read from: the fixture tree is compiled
// into the test binary, and a virtual host root is an absolute path no
// fs.FS spells. filename is only what errors name.
func OverlayBytes(base Config, filename string, data []byte, forbidden []string, owned string) (Config, map[string]any, error) {
	var declared map[string]any
	if err := yaml.Unmarshal(data, &declared); err != nil {
		return base, nil, fmt.Errorf("%s: %w", filename, err)
	}

	for _, key := range forbidden {
		if _, ok := declared[key]; ok {
			return base, declared, fmt.Errorf("%s: %q %s", filename, key, owned)
		}
	}

	result := base
	if err := yaml.Unmarshal(data, &result); err != nil {
		return base, declared, fmt.Errorf("%s: %w", filename, err)
	}
	return result, declared, nil
}

// Declares reports whether a parsed file named the given key path. Validation
// needs it to tell a setting a file asked for from one it inherited through
// Overlay.
func Declares(declared map[string]any, keys ...string) bool {
	for i, key := range keys {
		value, ok := declared[key]
		if !ok {
			return false
		}
		if i == len(keys)-1 {
			return true
		}
		declared, ok = value.(map[string]any)
		if !ok {
			return false
		}
	}
	return false
}
