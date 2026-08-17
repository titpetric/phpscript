// Package composer teaches the runtime to find classes the way a
// composer-managed PHP project expects.
//
// Composer's own vendor/autoload.php is a bootstrap for a ~600 line
// ClassLoader plus generated static maps; it leans on parts of PHP that
// phpscript does not implement, so including it is not a viable path. The
// data behind it is plain JSON, though: the project's composer.json and
// vendor/composer/installed.json describe every PSR-4/PSR-0 prefix and every
// file that should be preloaded. This package reads that metadata and installs
// a Go autoloader on the runtime, which makes `new MiniTPL\Template` resolve
// with no include at all.
//
// Scripts that also have to run under stock PHP keep their
// `include "vendor/autoload.php"` line — guarded by file_exists, since the
// generated file is absent until composer has run.
package composer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Filename is the manifest a project is recognised by.
const Filename = "composer.json"

// defaultVendorDir is composer's vendor-dir when config does not override it.
const defaultVendorDir = "vendor"

// autoload is the shape of an "autoload" / "autoload-dev" stanza. Composer
// accepts a string or a list of strings for each PSR prefix, so the raw values
// stay json.RawMessage until normalised by pathsOf.
type autoload struct {
	PSR4  map[string]json.RawMessage `json:"psr-4"`
	PSR0  map[string]json.RawMessage `json:"psr-0"`
	Files []string                   `json:"files"`
}

// manifest is the subset of composer.json this package reads.
type manifest struct {
	Name        string   `json:"name"`
	Autoload    autoload `json:"autoload"`
	AutoloadDev autoload `json:"autoload-dev"`
	Config      struct {
		VendorDir string `json:"vendor-dir"`
	} `json:"config"`
}

// installedPackage is one entry of vendor/composer/installed.json.
type installedPackage struct {
	Name        string   `json:"name"`
	Autoload    autoload `json:"autoload"`
	InstallPath string   `json:"install-path"`
}

// installed is vendor/composer/installed.json. Composer 1 wrote a bare array of
// packages, composer 2 wraps them in an object; both are accepted.
type installed struct {
	Packages []installedPackage `json:"packages"`
}

// Project is a resolved composer project: the prefix maps of the root package
// and of everything installed under vendor/, with all directories rewritten to
// paths relative to the filesystem the runtime loads from.
type Project struct {
	// Dir is the directory holding composer.json, relative to the FS root.
	Dir string
	// VendorDir is the resolved vendor directory, relative to the FS root.
	VendorDir string
	// PSR4 and PSR0 map a namespace prefix to the directories serving it.
	PSR4 map[string][]string
	PSR0 map[string][]string
	// Files are autoload.files entries, loaded eagerly in declaration order.
	Files []string
}

// Discover walks up from dir looking for a composer.json and resolves the
// autoload metadata it and its vendor directory declare. It reports false when
// no manifest is found, which is not an error: most scripts have no composer
// project and simply run without one.
func Discover(fsys fs.FS, dir string) (*Project, bool, error) {
	root, ok := findManifest(fsys, dir)
	if !ok {
		return nil, false, nil
	}

	var m manifest
	if err := readJSON(fsys, path.Join(root, Filename), &m); err != nil {
		return nil, false, fmt.Errorf("composer: %s: %w", path.Join(root, Filename), err)
	}

	vendorDir := m.Config.VendorDir
	if vendorDir == "" {
		vendorDir = defaultVendorDir
	}

	p := &Project{
		Dir:       root,
		VendorDir: joinDir(root, vendorDir),
		PSR4:      map[string][]string{},
		PSR0:      map[string][]string{},
	}
	// The root package's own autoload paths are relative to composer.json.
	p.add(root, m.Autoload)
	p.add(root, m.AutoloadDev)

	for _, pkg := range readInstalled(fsys, p.VendorDir) {
		p.add(packageDir(fsys, p.VendorDir, pkg), pkg.Autoload)
	}
	return p, true, nil
}

// AutoloadFile is the path of composer's generated bootstrap for this project.
func (p *Project) AutoloadFile() string {
	return joinDir(p.VendorDir, "autoload.php")
}

// findManifest returns the nearest ancestor of dir (dir included) that holds a
// composer.json.
func findManifest(fsys fs.FS, dir string) (string, bool) {
	current := path.Clean(strings.TrimPrefix(path.Clean("/"+dir), "/"))
	if current == "" {
		current = "."
	}
	for {
		if _, err := fs.Stat(fsys, path.Join(current, Filename)); err == nil {
			return current, true
		}
		if current == "." {
			return "", false
		}
		current = path.Dir(current)
	}
}

// readInstalled loads vendor/composer/installed.json, falling back to a scan of
// vendor/<vendor>/<package>/composer.json when the lock metadata is absent — a
// vendor tree assembled by hand still autoloads.
func readInstalled(fsys fs.FS, vendorDir string) []installedPackage {
	var list installed
	err := readJSON(fsys, path.Join(vendorDir, "composer", "installed.json"), &list)
	if err == nil && len(list.Packages) > 0 {
		return list.Packages
	}
	// Composer 1 wrote the packages as a top-level array.
	var legacy []installedPackage
	if err := readJSON(fsys, path.Join(vendorDir, "composer", "installed.json"), &legacy); err == nil && len(legacy) > 0 {
		return legacy
	}
	return scanVendor(fsys, vendorDir)
}

// scanVendor reads every vendor/<vendor>/<package>/composer.json it can find.
func scanVendor(fsys fs.FS, vendorDir string) []installedPackage {
	vendors, err := fs.ReadDir(fsys, vendorDir)
	if err != nil {
		return nil
	}
	var out []installedPackage
	for _, vendor := range vendors {
		if !vendor.IsDir() || vendor.Name() == "composer" {
			continue
		}
		packages, err := fs.ReadDir(fsys, path.Join(vendorDir, vendor.Name()))
		if err != nil {
			continue
		}
		for _, pkg := range packages {
			if !pkg.IsDir() {
				continue
			}
			name := vendor.Name() + "/" + pkg.Name()
			var m manifest
			if err := readJSON(fsys, path.Join(vendorDir, name, Filename), &m); err != nil {
				continue
			}
			out = append(out, installedPackage{Name: name, Autoload: m.Autoload})
		}
	}
	return out
}

// packageDir resolves where a package's files live. vendor/<name> is preferred
// because it stays inside the filesystem root even when composer installed a
// path repository as a symlink; install-path is the fallback, and is recorded
// relative to vendor/composer.
func packageDir(fsys fs.FS, vendorDir string, pkg installedPackage) string {
	if pkg.Name != "" {
		dir := path.Join(vendorDir, pkg.Name)
		if info, err := fs.Stat(fsys, dir); err == nil && info.IsDir() {
			return dir
		}
	}
	if pkg.InstallPath != "" {
		return joinDir(path.Join(vendorDir, "composer"), pkg.InstallPath)
	}
	return path.Join(vendorDir, pkg.Name)
}

// add merges one autoload stanza, resolving its relative paths against base.
func (p *Project) add(base string, a autoload) {
	for prefix, raw := range a.PSR4 {
		p.PSR4[normalizePrefix(prefix)] = append(p.PSR4[normalizePrefix(prefix)], resolve(base, raw)...)
	}
	for prefix, raw := range a.PSR0 {
		p.PSR0[normalizePrefix(prefix)] = append(p.PSR0[normalizePrefix(prefix)], resolve(base, raw)...)
	}
	for _, file := range a.Files {
		p.Files = append(p.Files, joinDir(base, file))
	}
}

// resolve turns a raw psr-4/psr-0 value (a string or a list) into directories
// relative to the filesystem root.
func resolve(base string, raw json.RawMessage) []string {
	var dirs []string
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		dirs = []string{single}
	} else if err := json.Unmarshal(raw, &dirs); err != nil {
		return nil
	}
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, joinDir(base, dir))
	}
	return out
}

// Resolve returns the candidate files that could declare class, in the order
// composer's ClassLoader would try them: PSR-4 longest prefix first, then PSR-0.
func (p *Project) Resolve(class string) []string {
	class = strings.TrimPrefix(class, "\\")
	if class == "" {
		return nil
	}
	var out []string
	// PSR-4 strips the matched prefix, so the longest match wins; ordering the
	// candidates that way keeps a nested prefix from being shadowed.
	for _, prefix := range longestFirst(p.PSR4, class) {
		suffix := strings.ReplaceAll(class[len(prefix):], "\\", "/")
		for _, dir := range p.PSR4[prefix] {
			out = append(out, joinDir(dir, suffix+".php"))
		}
	}
	// PSR-0 keeps the whole class name in the path and expands underscores in
	// the class part, the pre-namespace convention it was written for.
	for _, prefix := range longestFirst(p.PSR0, class) {
		for _, dir := range p.PSR0[prefix] {
			out = append(out, joinDir(dir, psr0Path(class)))
		}
	}
	return out
}

// longestFirst returns the prefixes of m matching class, longest first.
func longestFirst(m map[string][]string, class string) []string {
	var matched []string
	for prefix := range m {
		if strings.HasPrefix(class, prefix) {
			matched = append(matched, prefix)
		}
	}
	// The map is small (one entry per package, typically), so an insertion sort
	// beats pulling in sort machinery for a hot autoload path.
	for i := 1; i < len(matched); i++ {
		for j := i; j > 0 && len(matched[j]) > len(matched[j-1]); j-- {
			matched[j], matched[j-1] = matched[j-1], matched[j]
		}
	}
	return matched
}

// psr0Path maps a class name onto its PSR-0 file path: namespace separators
// become directories, and underscores in the final segment do too.
func psr0Path(class string) string {
	namespace, name := "", class
	if i := strings.LastIndex(class, "\\"); i >= 0 {
		namespace, name = class[:i], class[i+1:]
	}
	name = strings.ReplaceAll(name, "_", "/")
	if namespace == "" {
		return name + ".php"
	}
	return strings.ReplaceAll(namespace, "\\", "/") + "/" + name + ".php"
}

// normalizePrefix drops the trailing separator composer writes on psr-4 keys so
// prefix matching can use plain string prefixes.
func normalizePrefix(prefix string) string {
	return strings.TrimPrefix(prefix, "\\")
}

// joinDir joins two FS-relative path fragments, yielding "" for the root rather
// than path.Join's ".".
func joinDir(base, rel string) string {
	joined := path.Join(base, rel)
	if joined == "." {
		return ""
	}
	return joined
}

func readJSON(fsys fs.FS, name string, target any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
