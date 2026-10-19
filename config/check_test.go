package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestConfig_Validate covers everything that would stop a server from
// starting, without starting one: the check opens no listener and builds no
// trace store. A command runs it ahead of the server so a bad file fails the
// invocation and not the first request.
func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	t.Run("a single application root", func(t *testing.T) {
		root := writeSite(t, "routes:\n  enabled: true\n")
		if err := config.New().Check("config.yml", root); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an empty root is the working directory", func(t *testing.T) {
		// Check falls back to ".", so a server started with no root
		// validates the tree it runs in. This package directory holds
		// no public, so the document root check is what reports it, against a
		// path relative to the working directory.
		err := config.New().Check("config.yml", "")
		if err == nil {
			t.Fatal("a tree with no document root validated")
		}
		if !strings.Contains(err.Error(), "document root") {
			t.Errorf("err = %q, want the document root reported", err.Error())
		}
		if !strings.Contains(err.Error(), "stat public:") {
			t.Errorf("err = %q, want the path joined against the working directory", err.Error())
		}
	})

	t.Run("a bad test block fails before the root is read", func(t *testing.T) {
		result := config.New()
		result.Test.Cache = "request"
		err := result.Check("config.yml", filepath.Join(t.TempDir(), "absent"))
		if err == nil {
			t.Fatal("an unknown cache validated")
		}
		if !strings.Contains(err.Error(), "test.cache") {
			t.Errorf("err = %q, want the test block reported first", err.Error())
		}
	})

	t.Run("a bad telemetry driver fails", func(t *testing.T) {
		result := config.New()
		result.Telemetry.Driver = "s3"
		if err := result.Check("config.yml", writeSite(t, "autoindex: false\n")); err == nil {
			t.Fatal("an unknown telemetry driver validated")
		}
	})

	t.Run("a virtual host list is checked as a list", func(t *testing.T) {
		result := config.New()
		result.VirtualHost = []config.VirtualHost{
			{Domain: "one.test", Root: writeSite(t, "routes:\n  enabled: true\n")},
			{Domain: "two.test", Root: writeSite(t, "routes:\n  enabled: false\n")},
		}
		if err := result.Check("config.yml", ""); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a root beside a virtual host list is refused", func(t *testing.T) {
		result := config.New()
		result.VirtualHost = []config.VirtualHost{
			{Domain: "one.test", Root: writeSite(t, "routes:\n  enabled: true\n")},
		}
		err := result.Check("config.yml", "/srv/site")
		if err == nil {
			t.Fatal("a root validated beside a virtual host list")
		}
		if err.Error() != config.ErrRootWithVirtualHosts("/srv/site").Error() {
			t.Errorf("err = %q, want the refusal ErrRootWithVirtualHosts carries", err.Error())
		}
	})

	t.Run("a bad virtual host entry fails", func(t *testing.T) {
		result := config.New()
		result.VirtualHost = []config.VirtualHost{{Root: writeSite(t, "routes:\n  enabled: true\n")}}
		if err := result.Check("config.yml", ""); err == nil {
			t.Fatal("an entry with no domain validated")
		}
	})
}

// TestConfig_ValidateRoot covers the single application root the same way the
// cases above cover an entry of the virtual host list. Config.Check falls
// through to it when the configuration lists no sites. fs.Sub does not stat,
// so a document root that is not there otherwise answers 404 on the first
// request.
func TestConfig_ValidateRoot(t *testing.T) {
	t.Parallel()
	t.Run("a root holding its document root", func(t *testing.T) {
		if err := config.New().CheckRoot(writeSite(t, "routes:\n  enabled: true\n")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("public without a configured document root", func(t *testing.T) {
		// A Config assembled in Go carries no document root, and public is
		// the literal that covers it.
		if err := (config.Config{}).CheckRoot(writeSite(t, "autoindex: false\n")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a document root the file renamed", func(t *testing.T) {
		result := config.New()
		result.DocumentRoot = "htdocs"
		if err := result.CheckRoot(writeSite(t, "autoindex: false\n", "htdocs")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a root that is not there", func(t *testing.T) {
		err := config.New().CheckRoot(filepath.Join(t.TempDir(), "absent"))
		if err == nil {
			t.Fatal("a root that does not exist validated")
		}
		if !strings.Contains(err.Error(), "root:") {
			t.Errorf("err = %q, want it to name the root", err.Error())
		}
	})

	t.Run("a root with no document root", func(t *testing.T) {
		err := config.New().CheckRoot(t.TempDir())
		if err == nil {
			t.Fatal("a root with no document root validated")
		}
		if !strings.Contains(err.Error(), "document root") {
			t.Errorf("err = %q", err.Error())
		}
	})
}

// TestErrRootWithVirtualHosts covers the wording the refusal carries. The
// virtual host entries name their own roots, so a root typed on the command
// line has no entry to belong to, and the message quotes it back.
func TestErrRootWithVirtualHosts(t *testing.T) {
	t.Parallel()
	err := config.ErrRootWithVirtualHosts("/srv/site")
	want := `server: the configuration lists virtual hosts, which name their own roots; "/srv/site" on the command line has no virtual host to belong to`
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}
