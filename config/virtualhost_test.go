package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/config"
)

// writeSite creates an application root holding a phpscript.yml and a document
// root, and returns the application root.
func writeSite(t *testing.T, source string, documentRoots ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.VirtualHostConfigFile), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(documentRoots) == 0 {
		documentRoots = []string{"public"}
	}
	for _, name := range documentRoots {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestDefaultConfigVirtualHosts pins what config/config.yml says about virtual
// hosting: no sites, and public as the document root every site gets without
// naming one.
func TestDefaultConfigVirtualHosts(t *testing.T) {
	result := config.New()
	if result.DocumentRoot != "public" {
		t.Errorf("document root = %q, want public", result.DocumentRoot)
	}
	if len(result.VirtualHost) != 0 {
		t.Errorf("virtualhost = %+v, want none", result.VirtualHost)
	}
}

// TestVirtualHostLoadLayersOverBase pins the overlay a site gets: a key the
// site file does not name keeps the operator's value, one it names wins.
func TestVirtualHostLoadLayersOverBase(t *testing.T) {
	root := writeSite(t, `
routes:
  enabled: false
telemetry:
  path: "/debug/site"
`)
	base := config.New()
	host := config.VirtualHost{Domain: "site.test", Root: root}

	result, err := host.Load(base)
	if err != nil {
		t.Fatal(err)
	}
	if result.Routes.Enabled {
		t.Error("routes.enabled = true, the site file turned it off")
	}
	if result.Telemetry.Path != "/debug/site" {
		t.Errorf("telemetry path = %q, want /debug/site", result.Telemetry.Path)
	}
	// Keys the site left out keep what the base configuration says.
	if result.Telemetry.ServiceName != base.Telemetry.ServiceName {
		t.Errorf("service name = %q, want %q", result.Telemetry.ServiceName, base.Telemetry.ServiceName)
	}
	if result.Runner.WorkDir != base.Runner.WorkDir {
		t.Errorf("work dir = %q, want %q", result.Runner.WorkDir, base.Runner.WorkDir)
	}
}

// TestVirtualHostLoadRejectsServer pins that a site cannot move the listen
// address, and that it is told so rather than having the block dropped.
func TestVirtualHostLoadRejectsServer(t *testing.T) {
	root := writeSite(t, `
server:
  addr: "127.0.0.1:9999"
`)
	host := config.VirtualHost{Domain: "site.test", Root: root}

	_, err := host.Load(config.New())
	if err == nil {
		t.Fatal("a site moved the listen address")
	}
	want := `virtualhost "site.test": ` + filepath.Join(root, config.VirtualHostConfigFile) + `: "server" is set by the operator, not by the site`
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

// TestVirtualHostLoadRejectsNestedVirtualHosts pins that a site cannot declare
// sites of its own.
func TestVirtualHostLoadRejectsNestedVirtualHosts(t *testing.T) {
	root := writeSite(t, `
virtualhost:
  - domain: "nested.test"
    root: "."
`)
	host := config.VirtualHost{Domain: "site.test", Root: root}

	_, err := host.Load(config.New())
	if err == nil {
		t.Fatal("a site nested a virtual host")
	}
	if !strings.Contains(err.Error(), `"virtualhost" is set by the operator`) {
		t.Errorf("err = %q", err.Error())
	}
}

// TestVirtualHostLoadRequiresConfigFile pins that a root without a
// phpscript.yml fails, rather than serving the site under the operator's
// defaults.
func TestVirtualHostLoadRequiresConfigFile(t *testing.T) {
	root := t.TempDir()
	host := config.VirtualHost{Domain: "site.test", Root: root}

	_, err := host.Load(config.New())
	if err == nil {
		t.Fatal("a root without a configuration file loaded")
	}
	if !strings.Contains(err.Error(), config.VirtualHostConfigFile) {
		t.Errorf("err = %q, want it to name %s", err.Error(), config.VirtualHostConfigFile)
	}
}

// TestVirtualHostServerBlockIsHeldToBase pins the guard after the rejection:
// the loaded configuration carries the operator's server block and no nested
// sites whatever the file said.
func TestVirtualHostServerBlockIsHeldToBase(t *testing.T) {
	root := writeSite(t, "routes:\n  enabled: false\n")
	base := config.New()
	base.Server.Addr = "127.0.0.1:8081"
	host := config.VirtualHost{Domain: "site.test", Root: root}

	result, err := host.Load(base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Server, base.Server) {
		t.Errorf("server = %+v, want %+v", result.Server, base.Server)
	}
	if result.VirtualHost != nil {
		t.Errorf("virtualhost = %+v, want none", result.VirtualHost)
	}
}

// TestVirtualHostDocumentRootPrecedence pins the chain: the operator's entry,
// then the site file, then public.
func TestVirtualHostDocumentRootPrecedence(t *testing.T) {
	base := config.New()

	t.Run("operator entry wins", func(t *testing.T) {
		root := writeSite(t, "document_root: www\n", "www", "htdocs")
		host := config.VirtualHost{Domain: "site.test", Root: root, DocumentRoot: "htdocs"}
		result, err := host.Load(base)
		if err != nil {
			t.Fatal(err)
		}
		if result.DocumentRoot != "htdocs" {
			t.Errorf("document root = %q, want htdocs", result.DocumentRoot)
		}
	})

	t.Run("site file next", func(t *testing.T) {
		root := writeSite(t, "document_root: www\n", "www")
		host := config.VirtualHost{Domain: "site.test", Root: root}
		result, err := host.Load(base)
		if err != nil {
			t.Fatal(err)
		}
		if result.DocumentRoot != "www" {
			t.Errorf("document root = %q, want www", result.DocumentRoot)
		}
	})

	t.Run("public by default", func(t *testing.T) {
		root := writeSite(t, "routes:\n  enabled: true\n")
		host := config.VirtualHost{Domain: "site.test", Root: root}
		result, err := host.Load(base)
		if err != nil {
			t.Fatal(err)
		}
		if result.DocumentRoot != "public" {
			t.Errorf("document root = %q, want public", result.DocumentRoot)
		}
	})

	t.Run("public without a base value", func(t *testing.T) {
		root := writeSite(t, "routes:\n  enabled: true\n")
		host := config.VirtualHost{Domain: "site.test", Root: root}
		result, err := host.Load(config.Config{})
		if err != nil {
			t.Fatal(err)
		}
		if result.DocumentRoot != "public" {
			t.Errorf("document root = %q, want public", result.DocumentRoot)
		}
	})
}

// TestVirtualHostNormalize pins the comparison a Host header gets: lowercase,
// no trailing dot.
func TestVirtualHostNormalize(t *testing.T) {
	host := config.VirtualHost{Domain: " Site.TEST. "}.Normalize()
	if host.Domain != "site.test" {
		t.Errorf("domain = %q, want site.test", host.Domain)
	}
}

// TestValidateVirtualHosts covers the checks over the list as a whole. Each
// case names the domain it is about, which is what the error has to carry.
func TestValidateVirtualHosts(t *testing.T) {
	site := func(t *testing.T, source string, documentRoots ...string) string {
		return writeSite(t, source, documentRoots...)
	}

	t.Run("valid list", func(t *testing.T) {
		base := config.New()
		base.VirtualHost = []config.VirtualHost{
			{Domain: "one.test", Root: site(t, "routes:\n  enabled: true\n")},
			{Domain: "two.test", Root: site(t, "routes:\n  enabled: false\n")},
		}
		if err := base.ValidateVirtualHosts(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("no virtual hosts", func(t *testing.T) {
		if err := config.New().ValidateVirtualHosts(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("empty domain", func(t *testing.T) {
		base := config.New()
		base.VirtualHost = []config.VirtualHost{{Root: site(t, "routes:\n  enabled: true\n")}}
		if err := base.ValidateVirtualHosts(); err == nil {
			t.Fatal("an entry with no domain validated")
		}
	})

	t.Run("duplicate domains", func(t *testing.T) {
		base := config.New()
		base.VirtualHost = []config.VirtualHost{
			{Domain: "one.test", Root: site(t, "routes:\n  enabled: true\n")},
			{Domain: "ONE.test.", Root: site(t, "routes:\n  enabled: true\n")},
		}
		err := base.ValidateVirtualHosts()
		if err == nil {
			t.Fatal("a domain was configured twice")
		}
		if !strings.Contains(err.Error(), "one.test") {
			t.Errorf("err = %q, want it to name the domain", err.Error())
		}
	})

	t.Run("missing root", func(t *testing.T) {
		base := config.New()
		base.VirtualHost = []config.VirtualHost{{Domain: "one.test", Root: filepath.Join(t.TempDir(), "absent")}}
		err := base.ValidateVirtualHosts()
		if err == nil {
			t.Fatal("a root that does not exist validated")
		}
		if !strings.Contains(err.Error(), "one.test") {
			t.Errorf("err = %q, want it to name the domain", err.Error())
		}
	})

	t.Run("root is a file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "site")
		if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}
		base := config.New()
		base.VirtualHost = []config.VirtualHost{{Domain: "one.test", Root: file}}
		err := base.ValidateVirtualHosts()
		if err == nil {
			t.Fatal("a file validated as an application root")
		}
		if !strings.Contains(err.Error(), "is not a directory") {
			t.Errorf("err = %q", err.Error())
		}
	})

	t.Run("missing document root", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, config.VirtualHostConfigFile), []byte("routes:\n  enabled: true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		base := config.New()
		base.VirtualHost = []config.VirtualHost{{Domain: "one.test", Root: root}}
		err := base.ValidateVirtualHosts()
		if err == nil {
			t.Fatal("a site with no document root validated")
		}
		if !strings.Contains(err.Error(), "document root") {
			t.Errorf("err = %q", err.Error())
		}
	})

	t.Run("colliding disk storage path", func(t *testing.T) {
		storage := t.TempDir()
		source := "telemetry:\n  driver: disk\n  storage_path: " + storage + "\n"
		base := config.New()
		base.VirtualHost = []config.VirtualHost{
			{Domain: "one.test", Root: site(t, source)},
			{Domain: "two.test", Root: site(t, source)},
		}
		err := base.ValidateVirtualHosts()
		if err == nil {
			t.Fatal("two sites shared a trace store")
		}
		if !strings.Contains(err.Error(), "storage_path") || !strings.Contains(err.Error(), "two.test") {
			t.Errorf("err = %q", err.Error())
		}
	})

	t.Run("distinct disk storage paths", func(t *testing.T) {
		base := config.New()
		base.VirtualHost = []config.VirtualHost{
			{Domain: "one.test", Root: site(t, "telemetry:\n  driver: disk\n  storage_path: "+t.TempDir()+"\n  path: /debug/one\n")},
			{Domain: "two.test", Root: site(t, "telemetry:\n  driver: disk\n  storage_path: "+t.TempDir()+"\n  path: /debug/two\n")},
		}
		if err := base.ValidateVirtualHosts(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("telemetry path collides with the base path", func(t *testing.T) {
		base := config.New()
		source := "telemetry:\n  path: " + base.Telemetry.Path + "\n"
		base.VirtualHost = []config.VirtualHost{{Domain: "one.test", Root: site(t, source)}}
		err := base.ValidateVirtualHosts()
		if err == nil {
			t.Fatal("a site asked for the path the server mounts its own dashboard on")
		}
		if !strings.Contains(err.Error(), "one.test") || !strings.Contains(err.Error(), base.Telemetry.Path) {
			t.Errorf("err = %q", err.Error())
		}
	})

	t.Run("telemetry path collides with a disabled base", func(t *testing.T) {
		base := config.New()
		base.Telemetry.Enabled = false
		source := "telemetry:\n  path: " + base.Telemetry.Path + "\n"
		base.VirtualHost = []config.VirtualHost{{Domain: "one.test", Root: site(t, source)}}
		// Nothing is mounted on the root router, so nothing is shadowed.
		if err := base.ValidateVirtualHosts(); err != nil {
			t.Fatal(err)
		}
	})
}
