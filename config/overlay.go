package config

import (
	"fmt"
	"os"
)

// Overlay reads filename over base and returns the keys the file itself named.
// A missing file is an error; the read does not fall back to base.
func Overlay(base Config, filename string, forbidden []string, owned string) (Config, map[string]any, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return base, nil, err
	}
	return OverlayBytes(base, filename, data, forbidden, owned)
}

// OverlayBytes is Overlay over source already read. A caller holding the file
// as bytes supplies the filesystem: the build compiles the fixture tree into
// the test binary; a virtual host root is an absolute path no fs.FS spells.
// filename serves only error messages.
func OverlayBytes(base Config, filename string, data []byte, forbidden []string, owned string) (Config, map[string]any, error) {
	var declared map[string]any
	if err := Unmarshal(data, &declared); err != nil {
		return base, nil, fmt.Errorf("%s: %w", filename, err)
	}

	for _, key := range forbidden {
		if _, ok := declared[key]; ok {
			return base, declared, fmt.Errorf("%s: %q %s", filename, key, owned)
		}
	}

	if err := CheckMailDeclaration(filename, declared); err != nil {
		return base, declared, err
	}

	result := base
	if err := Unmarshal(data, &result); err != nil {
		return base, declared, fmt.Errorf("%s: %w", filename, err)
	}

	// A mapping decoded into a map replaces the map. A file that names servers
	// holds only the servers it named; a site cannot inherit the operator's
	// credentials. One spelling does not behave that way: `mail:` whose key
	// holds nothing reads as a null node. The decoder leaves the inherited
	// value. A file that declares the key and names no servers would inherit
	// the operator's. Naming the key and leaving it empty means no servers.
	//
	// This sits here, not with the virtual host checks: a test suite's
	// phpscript.yml overlays through this same function and needs the same
	// guarantee.
	if servers, ok := declared["mail"]; ok && servers == nil {
		result.Mail = nil
	}

	return result, declared, nil
}

// Declares reports whether a parsed file named the given key path. Validation
// needs it to distinguish a named setting from one inherited.
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
