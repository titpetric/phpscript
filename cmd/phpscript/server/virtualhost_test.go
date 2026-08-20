package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
)

// newVirtualHostServer writes two sites to disk and returns the host mux and
// modules the server would run for them.
//
// shop leaves the document root alone, which is the expected case: public is
// the default and nothing has to say so. blog names web, the reason the setting
// exists at all.
func newVirtualHostServer(t *testing.T) (http.Handler, []platform.Module, config.Config) {
	t.Helper()

	dir := t.TempDir()
	shop := filepath.Join(dir, "shop")
	blog := filepath.Join(dir, "blog")

	write(t, filepath.Join(shop, "phpscript.yml"), `
telemetry:
  enabled: true
  path: /debug/oida
  service_name: shop
env:
  - "PLATFORM_DB_SHOP=sqlite://:memory:"
  - "SHOP_ONLY=shop-value"
`)
	write(t, filepath.Join(shop, "public", "index.php"), `<?php echo "shop";`)
	write(t, filepath.Join(shop, "public", "style.css"), `body { color: red; }`)
	write(t, filepath.Join(shop, "lib", "greet.php"), `<?php function greet() { return "greeted"; }`)
	write(t, filepath.Join(shop, "public", "include.php"), `<?php include "lib/greet.php"; echo greet();`)
	write(t, filepath.Join(shop, "public", "db.php"), `<?php $db = new Database("shop"); echo "connected";`)
	write(t, filepath.Join(shop, "public", "env.php"), `<?php echo "[" . getenv("SHOP_ONLY") . "][" . getenv("PLATFORM_DB_SHOP") . "]";`)
	write(t, filepath.Join(shop, "public", "server.php"), `<?php
foreach (["SERVER_NAME", "SERVER_PORT", "SCRIPT_NAME", "PHP_SELF", "SERVER_SOFTWARE", "DOCUMENT_ROOT"] as $key) {
	echo $key . "=" . $_SERVER[$key] . "\n";
}
`)
	write(t, filepath.Join(shop, "routes", "hello.php"), `<?php
// @route GET /hello/{name}
echo "hello " . $_PATH["name"];
`)
	// A failing @startup belongs to this site and must not stop the others.
	write(t, filepath.Join(shop, "boot.php"), `<?php
// @startup
missing_function();
`)

	write(t, filepath.Join(blog, "phpscript.yml"), `
document_root: web
telemetry:
  enabled: false
env: []
runner:
  writable_paths: ["web/upload"]
`)
	// What a script, and through it a visitor, put in a writable directory.
	write(t, filepath.Join(blog, "web", "upload", "evil.php"), `<?php echo "executed";`)
	write(t, filepath.Join(blog, "web", "upload", "photo.txt"), `a photo`)
	// An annotation in a writable directory must not publish a route either.
	write(t, filepath.Join(blog, "web", "upload", "route.php"), `<?php
// @route GET /uploaded-route
echo "uploaded route";
`)
	write(t, filepath.Join(blog, "web", "index.php"), `<?php echo "blog";`)
	write(t, filepath.Join(blog, "web", "db.php"), `<?php $db = new Database("shop"); echo "connected";`)

	appConfig := config.NewTestConfig()
	appConfig.VirtualHost = []config.VirtualHost{
		{Domain: "shop.example.com", Aliases: []string{"WWW.Shop.Example.com", "shop.test."}, Root: shop},
		{Domain: "Blog.Example.COM", Root: blog},
	}

	handler, modules, err := buildVirtualHosts(context.Background(), appConfig)
	if err != nil {
		t.Fatal(err)
	}
	return handler, modules, appConfig
}

func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, handler http.Handler, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Host = host
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// TestVirtualHostsServeTheirOwnTree pins the split: each domain serves the
// document root of its own application root, and its default is public.
func TestVirtualHostsServeTheirOwnTree(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	for _, test := range []struct {
		host string
		path string
		body string
		code int
	}{
		{host: "shop.example.com", path: "/", body: "shop", code: http.StatusOK},
		{host: "blog.example.com", path: "/", body: "blog", code: http.StatusOK},
		{host: "shop.example.com", path: "/style.css", body: "body { color: red; }", code: http.StatusOK},
		// An include reaching above the document root still resolves inside
		// the application root.
		{host: "shop.example.com", path: "/include.php", body: "greeted", code: http.StatusOK},
		// blog names web as its document root, so public is not a thing it has.
		{host: "blog.example.com", path: "/style.css", code: http.StatusNotFound},
		{host: "other.example.com", path: "/", code: http.StatusNotFound},
	} {
		t.Run(test.host+test.path, func(t *testing.T) {
			response := get(t, handler, test.host, test.path)
			if response.Code != test.code {
				t.Fatalf("status = %d, want %d, body = %q", response.Code, test.code, response.Body.String())
			}
			if test.body != "" && response.Body.String() != test.body {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.body)
			}
		})
	}
}

// TestVirtualHostRoutesDoNotLeakAcrossDomains is the point of giving each site
// a router of its own: an @route one site declares is not reachable on another.
func TestVirtualHostRoutesDoNotLeakAcrossDomains(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	response := get(t, handler, "shop.example.com", "/hello/Ada")
	if response.Code != http.StatusOK || response.Body.String() != "hello Ada" {
		t.Fatalf("shop: status = %d, body = %q", response.Code, response.Body.String())
	}

	response = get(t, handler, "blog.example.com", "/hello/Ada")
	if response.Code != http.StatusNotFound {
		t.Fatalf("blog: status = %d, body = %q", response.Code, response.Body.String())
	}
}

// TestVirtualHostTelemetryIsItsOwn pins that a site's debug front end answers
// on its domain and nowhere else, and that a site with telemetry off has none.
func TestVirtualHostTelemetryIsItsOwn(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	response := get(t, handler, "shop.example.com", "/debug/oida/traces")
	if response.Code != http.StatusOK {
		t.Fatalf("shop dashboard: status = %d, body = %q", response.Code, response.Body.String())
	}

	response = get(t, handler, "blog.example.com", "/debug/oida/traces")
	if response.Code != http.StatusNotFound {
		t.Fatalf("blog dashboard: status = %d, body = %q", response.Code, response.Body.String())
	}
}

// TestVirtualHostRecordsIntoItsOwnTracer pins that a request on one domain is
// recorded by that domain's recorder, which is what makes the front ends
// separate rather than two views of one buffer.
func TestVirtualHostRecordsIntoItsOwnTracer(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	if response := get(t, handler, "shop.example.com", "/"); response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response := get(t, handler, "blog.example.com", "/"); response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}

	traces := get(t, handler, "shop.example.com", "/debug/oida/traces")
	if traces.Code != http.StatusOK {
		t.Fatalf("status = %d", traces.Code)
	}
	// The listing names the host each trace was recorded for. shop's page has
	// to show shop's traffic, or the absence of blog's below proves nothing.
	if !strings.Contains(traces.Body.String(), "shop.example.com") {
		t.Fatal("shop dashboard does not show its own traffic")
	}
	if strings.Contains(traces.Body.String(), "blog.example.com") {
		t.Fatal("shop dashboard shows blog traffic")
	}
}

// TestVirtualHostDatabasesAreIsolated is the guarantee a shared execution
// environment rests on: a site can open the connections its own env names, and
// no others. blog runs a byte-identical script and cannot reach shop's.
func TestVirtualHostDatabasesAreIsolated(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	response := get(t, handler, "shop.example.com", "/db.php")
	if response.Code != http.StatusOK || response.Body.String() != "connected" {
		t.Fatalf("shop: status = %d, body = %q", response.Code, response.Body.String())
	}

	response = get(t, handler, "blog.example.com", "/db.php")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("blog: status = %d, body = %q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "no configuration found for database") {
		t.Fatalf("blog: body = %q, want the connection to be unknown to it", response.Body.String())
	}
}

// TestVirtualHostAliasesReachTheSameSite pins that an alias is another name for
// a site, not another copy of it: one build, one set of modules, and the same
// handler behind every name.
func TestVirtualHostAliasesReachTheSameSite(t *testing.T) {
	handler, modules, _ := newVirtualHostServer(t)

	for _, host := range []string{
		"shop.example.com",
		"www.shop.example.com",
		"WWW.Shop.Example.COM:8080",
		"shop.test",
		"shop.test.",
	} {
		t.Run(host, func(t *testing.T) {
			response := get(t, handler, host, "/")
			if response.Code != http.StatusOK || response.Body.String() != "shop" {
				t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
			}
		})
	}

	// An alias registers no second site, so the module count is unchanged:
	// two sites, one startup and one schedule module each.
	if len(modules) != 4 {
		t.Fatalf("modules = %d, want 4; an alias must not build a second site", len(modules))
	}

	// A route declared by the site answers on its aliases too.
	if response := get(t, handler, "shop.test", "/hello/Ada"); response.Body.String() != "hello Ada" {
		t.Fatalf("body = %q", response.Body.String())
	}
}

// TestVirtualHostEnvironmentIsItsOwn pins that getenv() reads the site's own
// env, not the process the operator started, and never the infrastructure
// variables that carry connection strings.
func TestVirtualHostEnvironmentIsItsOwn(t *testing.T) {
	t.Setenv("OPERATOR_SECRET", "hunter2")
	handler, _, _ := newVirtualHostServer(t)

	response := get(t, handler, "shop.example.com", "/env.php")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	// The site's own variable is readable; the PLATFORM_ entry beside it in
	// the same env block is not, because it configures a connection.
	if got := response.Body.String(); got != "[shop-value][]" {
		t.Fatalf("body = %q, want [shop-value][]", got)
	}
}

// TestVirtualHostStartupFailureDoesNotStopTheServer pins the reason the
// startup module is wrapped: one tenant's broken job is that tenant's problem.
func TestVirtualHostStartupFailureDoesNotStopTheServer(t *testing.T) {
	_, modules, _ := newVirtualHostServer(t)

	// shop's boot.php calls a function that does not exist. Every module has
	// to start clean anyway, or the platform aborts the process and takes
	// blog down with it.
	for _, module := range modules {
		if err := module.Start(context.Background()); err != nil {
			t.Fatalf("module %s returned %v, want the failure recorded and not returned", module.Name(), err)
		}
	}
}

// TestVirtualHostServerVars pins the $_SERVER entries only the server can fill:
// they name the site the request reached, not the process it shares.
func TestVirtualHostServerVars(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	response := get(t, handler, "www.shop.example.com:8080", "/server.php")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		"SERVER_NAME=www.shop.example.com",
		"SERVER_PORT=8080",
		"SCRIPT_NAME=/server.php",
		"PHP_SELF=/server.php",
		"SERVER_SOFTWARE=phpscript",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %s", body, want)
		}
	}
	// The document root is this site's, under its own application root.
	if !strings.Contains(body, "/shop/public") {
		t.Fatalf("body = %q, want shop's document root", body)
	}
}

// TestWritablePathsAreNotExecuted is the reason writable_paths reaches the
// router at all: a directory a script can write to is a directory a visitor can
// get content into, and running what lands there turns an upload form into a
// way to execute code.
func TestWritablePathsAreNotExecuted(t *testing.T) {
	handler, _, _ := newVirtualHostServer(t)

	// The .php file is served as bytes, not run.
	response := get(t, handler, "blog.example.com", "/upload/evil.php")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if response.Body.String() == "executed" {
		t.Fatal("a .php file in a writable directory was executed")
	}
	if !strings.Contains(response.Body.String(), "<?php") {
		t.Fatalf("body = %q, want the file served verbatim", response.Body.String())
	}

	// Everything else in the directory is served as before.
	if response := get(t, handler, "blog.example.com", "/upload/photo.txt"); response.Body.String() != "a photo" {
		t.Fatalf("body = %q", response.Body.String())
	}

	// And an annotation in there publishes nothing.
	if response := get(t, handler, "blog.example.com", "/uploaded-route"); response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want an uploaded @route to be ignored", response.Code)
	}

	// A site that configures no writable_paths keeps executing its own PHP.
	if response := get(t, handler, "shop.example.com", "/"); response.Body.String() != "shop" {
		t.Fatalf("body = %q", response.Body.String())
	}
}

// TestVirtualHostModulesAreNamedPerSite pins that platform.Options.Modules can
// still address one site's modules, which identical names would prevent.
func TestVirtualHostModulesAreNamedPerSite(t *testing.T) {
	_, modules, _ := newVirtualHostServer(t)

	got := map[string]bool{}
	for _, module := range modules {
		if got[module.Name()] {
			t.Fatalf("module %q is registered twice", module.Name())
		}
		got[module.Name()] = true
	}
	for _, name := range []string{
		"phpstartup:shop.example.com",
		"phpschedule:shop.example.com",
		"phpstartup:blog.example.com",
		"phpschedule:blog.example.com",
	} {
		if !got[name] {
			t.Fatalf("module %q is missing, got %v", name, got)
		}
	}
}

// TestVirtualHostRejectsARootOnTheCommandLine pins that the roots come from the
// configuration once virtual hosts are configured: an argument naming one more
// root has no site to belong to.
func TestVirtualHostRejectsARootOnTheCommandLine(t *testing.T) {
	_, _, appConfig := newVirtualHostServer(t)

	err := Run(context.Background(), []string{"."}, appConfig)
	if err == nil || !strings.Contains(err.Error(), "no virtual host to belong to") {
		t.Fatalf("err = %v, want the command line root to be rejected", err)
	}
}
