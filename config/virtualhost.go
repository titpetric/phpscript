package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "github.com/goccy/go-yaml"
)

// VirtualHostConfigFile is the file in an application root that configures
// the virtual host rooted there.
const VirtualHostConfigFile = "phpscript.yml"

// tenantKeys are the keys a site may not set in its own configuration file.
// The listen address belongs to the operator and a site cannot contain further
// sites.
var tenantKeys = []string{"server", "virtualhost"}

// VirtualHost routes a set of domains to an application root. The operator owns
// these fields; everything else about the site comes from the phpscript.yml in
// Root, which the site's own author writes.
type VirtualHost struct {
	// Domain is the Host header this entry answers for.
	Domain string `yaml:"domain"`

	// Aliases are further Host headers reaching the same site. They are the
	// same site, not copies of it: one application root, one configuration,
	// one set of connections and one recorder, no matter which name a
	// request arrived under. The usual case is a bare domain and its www
	// form.
	Aliases []string `yaml:"aliases"`

	// Root is the application root, the directory holding the site's
	// phpscript.yml.
	Root string `yaml:"root"`

	// DocumentRoot is the directory under Root served over HTTP. It defaults
	// to public and rarely needs setting: it is here for a site whose tree
	// already names that directory something else, not a field an operator is
	// expected to fill in.
	DocumentRoot string `yaml:"document_root"`
}

// Normalize lowercases every name the entry answers for and trims a trailing
// dot, so a Host header compares equal to a configured domain.
func (v VirtualHost) Normalize() VirtualHost {
	v.Domain = normalizeDomain(v.Domain)
	if len(v.Aliases) > 0 {
		aliases := make([]string, 0, len(v.Aliases))
		for _, alias := range v.Aliases {
			aliases = append(aliases, normalizeDomain(alias))
		}
		v.Aliases = aliases
	}
	return v
}

// Domains returns every name this entry answers for, the domain first.
func (v VirtualHost) Domains() []string {
	return append([]string{v.Domain}, v.Aliases...)
}

func normalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

// Load reads Root/phpscript.yml over base and returns the configuration the
// virtual host runs under, with the document root resolved.
func (v VirtualHost) Load(base Config) (Config, error) {
	result, _, err := v.load(base)
	return result, err
}

// load is Load, also returning the keys the file named. Validation needs to
// tell a setting the site asked for from one it inherited from base.
func (v VirtualHost) load(base Config) (Config, map[string]any, error) {
	filename := filepath.Join(v.Root, VirtualHostConfigFile)

	// A missing file is an error rather than a fall back to the operator's
	// defaults. The file is the site's contract, and a site served under
	// settings it never wrote is the failure mode worth avoiding.
	data, err := os.ReadFile(filename)
	if err != nil {
		return base, nil, fmt.Errorf("virtualhost %q: %w", v.Domain, err)
	}

	var declared map[string]any
	if err := yaml.Unmarshal(data, &declared); err != nil {
		return base, nil, fmt.Errorf("virtualhost %q: %s: %w", v.Domain, filename, err)
	}

	// Reject rather than drop what a site may not set, so a site author never
	// believes they moved the listen address or nested a site of their own.
	for _, key := range tenantKeys {
		if _, ok := declared[key]; ok {
			return base, declared, fmt.Errorf("virtualhost %q: %s: %q is set by the operator, not by the site", v.Domain, filename, key)
		}
	}

	// The same overlay the base file gets: unmarshal over an already populated
	// struct, so the file only has to name what it changes.
	result := base
	if err := yaml.Unmarshal(data, &result); err != nil {
		return base, declared, fmt.Errorf("virtualhost %q: %s: %w", v.Domain, filename, err)
	}

	// Belt and braces over the rejection above: whatever the file contained,
	// the server block is the operator's and a site holds no sites.
	result.Server = base.Server
	result.VirtualHost = nil

	// The operator's entry wins, because where a site sits on disk is the
	// operator's call. public is the default and config.yml carries it; the
	// literal here only covers a Config assembled in Go without it.
	documentRoot := v.DocumentRoot
	if documentRoot == "" {
		documentRoot = result.DocumentRoot
	}
	if documentRoot == "" {
		documentRoot = "public"
	}
	result.DocumentRoot = documentRoot

	return result, declared, nil
}

// ValidateVirtualHosts checks the whole list before any site is built, so a
// bad entry fails the server rather than one request. Every entry is loaded
// here, because the document root and the telemetry block two of the checks
// need come from the site's phpscript.yml and not from the entry.
func (c Config) ValidateVirtualHosts() error {
	if len(c.VirtualHost) == 0 {
		return nil
	}

	domains := make(map[string]bool, len(c.VirtualHost))
	storagePaths := make(map[string]string, len(c.VirtualHost))

	for _, host := range c.VirtualHost {
		host = host.Normalize()
		if host.Domain == "" {
			return fmt.Errorf("virtualhost: domain is required")
		}
		// An alias is a name like any other: two entries cannot both claim
		// one, and an entry cannot list its own domain twice, because the
		// mux has one handler per name and the second would win silently.
		for _, domain := range host.Domains() {
			if domain == "" {
				return fmt.Errorf("virtualhost %q: alias is empty", host.Domain)
			}
			if domains[domain] {
				return fmt.Errorf("virtualhost %q: domain %q is configured twice", host.Domain, domain)
			}
			domains[domain] = true
		}

		if host.Root == "" {
			return fmt.Errorf("virtualhost %q: root is required", host.Domain)
		}
		if err := statDir(host.Root); err != nil {
			return fmt.Errorf("virtualhost %q: root: %w", host.Domain, err)
		}

		loaded, declared, err := host.load(c)
		if err != nil {
			return err
		}

		if err := statDir(filepath.Join(host.Root, loaded.DocumentRoot)); err != nil {
			return fmt.Errorf("virtualhost %q: document root: %w", host.Domain, err)
		}

		if strings.EqualFold(strings.TrimSpace(loaded.Telemetry.Driver), "disk") {
			path := loaded.Telemetry.StoragePath
			if other, ok := storagePaths[path]; ok {
				return fmt.Errorf("virtualhost %q: telemetry storage_path %q is already used by %q", host.Domain, path, other)
			}
			storagePaths[path] = host.Domain
		}

		// The platform mounts its dashboard on the root router, which shadows
		// that path prefix on every host, so a site that asks for the same
		// path gets a dashboard nothing can reach. A site that names no path
		// of its own is not asking for one and is left alone.
		if c.Telemetry.Enabled && declares(declared, "telemetry", "path") && loaded.Telemetry.Path == c.Telemetry.Path {
			return fmt.Errorf("virtualhost %q: telemetry path %q is the path the server mounts its own dashboard on", host.Domain, loaded.Telemetry.Path)
		}
	}

	return nil
}

// declares reports whether the file named the given key path.
func declares(declared map[string]any, keys ...string) bool {
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

// statDir reports whether path exists and is a directory.
func statDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
