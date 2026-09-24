package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
)

// precompileSite writes an application root holding two files that parse, one
// that does not, and one that is not PHP at all.
func precompileSite(t testing.TB) string {
	t.Helper()

	root := t.TempDir()
	write(t, filepath.Join(root, "public", "index.php"), `<?php echo "home";`)
	write(t, filepath.Join(root, "lib", "greet.php"), `<?php function greet() { return "greeted"; }`)
	write(t, filepath.Join(root, "lib", "broken.php"), `<?php function ( { `)
	write(t, filepath.Join(root, "public", "style.css"), `body { color: red; }`)
	return root
}

// newPrecompileHandler builds the file handler of that root under an options
// block that either asked for precompilation or did not.
func newPrecompileHandler(t testing.TB, root string, on bool) *handler {
	t.Helper()

	files, err := newHandler(os.DirFS(root), root, DefaultDocumentRoot, runner.Options{Precompile: on}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestPrecompile covers the setting deciding when a tree is parsed, and the
// broken file that must not stop a site coming up: it is left out of the cache
// and the two files that parse are in it.
func TestPrecompile(t *testing.T) {
	root := precompileSite(t)

	for _, test := range []struct {
		name string
		on   bool
		want int
	}{
		{name: "on", on: true, want: 2},
		{name: "off", on: false, want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := newPrecompileHandler(t, root, test.on)
			precompile(files, root)
			if files.includeCache.Len() != test.want {
				t.Fatalf("include cache holds %d, want %d", files.includeCache.Len(), test.want)
			}
		})
	}
}

// TestPrecompileServesAfterABrokenFile covers what the site does once the walk
// has skipped one: the page still answers, and so does a request for the file
// that did not parse, with the error it fails with lazily.
func TestPrecompileServesAfterABrokenFile(t *testing.T) {
	root := precompileSite(t)
	files := newPrecompileHandler(t, root, true)
	precompile(files, root)

	if response := get(t, files, "localhost", "/"); response.Code != http.StatusOK || response.Body.String() != "home" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}

	write(t, filepath.Join(root, "public", "broken.php"), `<?php function ( { `)
	if response := get(t, files, "localhost", "/broken.php"); response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

// TestSharedCaches covers the pair a site's routed endpoints run off: the ones
// its file handler holds, so the one walk of the tree covers both. The proof is
// an endpoint answering out of the cache after its file is gone from disk.
func TestSharedCaches(t *testing.T) {
	root := precompileSite(t)
	write(t, filepath.Join(root, "routes", "hello.php"), "<?php\n// @route GET /hello\necho \"hello\";\n")

	files := newPrecompileHandler(t, root, true)
	options := sharedCaches(files, []annotations.Option{
		annotations.WithRunnerOptions(files.runnerOptions),
		annotations.WithExcludedDirectory(DefaultDocumentRoot),
	})
	routes := annotations.NewRoute(files.root, options...)

	router := chi.NewRouter()
	if err := routes.Mount(context.Background(), router); err != nil {
		t.Fatal(err)
	}
	precompile(files, root)

	if err := os.Remove(filepath.Join(root, "routes", "hello.php")); err != nil {
		t.Fatal(err)
	}
	if response := get(t, router, "localhost", "/hello"); response.Code != http.StatusOK || response.Body.String() != "hello" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

// TestVirtualHostPrecompilesUnderItsOwnConfiguration covers the per-site half:
// the setting is read from each site's phpscript.yml, a site holding a file
// that does not parse still builds, and both answer.
func TestVirtualHostPrecompilesUnderItsOwnConfiguration(t *testing.T) {
	dir := t.TempDir()
	eager, lazy := filepath.Join(dir, "eager"), filepath.Join(dir, "lazy")

	write(t, filepath.Join(eager, "phpscript.yml"), "telemetry:\n  enabled: false\nenv: []\n")
	write(t, filepath.Join(eager, "public", "index.php"), `<?php echo "eager";`)
	write(t, filepath.Join(eager, "lib", "broken.php"), `<?php function ( { `)

	write(t, filepath.Join(lazy, "phpscript.yml"), "telemetry:\n  enabled: false\nenv: []\nrunner:\n  precompile: false\n")
	write(t, filepath.Join(lazy, "public", "index.php"), `<?php echo "lazy";`)

	appConfig := config.NewTestConfig()
	appConfig.VirtualHost = []config.VirtualHost{
		{Domain: "eager.example.com", Root: eager},
		{Domain: "lazy.example.com", Root: lazy},
	}

	handler, _, err := buildVirtualHosts(context.Background(), appConfig, &flags.Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for host, body := range map[string]string{"eager.example.com": "eager", "lazy.example.com": "lazy"} {
		response := get(t, handler, host, "/")
		if response.Code != http.StatusOK || response.Body.String() != body {
			t.Fatalf("%s: status = %d, body = %q", host, response.Code, response.Body.String())
		}
	}
}

// benchPage is the entrypoint the benchmark serves. It carries enough
// expressions that the closure compiles a request repeats without the setting
// are visible next to the parse of the file itself.
const benchPage = `<?php
$rows = [];
for ($i = 0; $i < 20; $i++) {
	$rows[] = ["id" => $i, "name" => "row " . $i, "even" => $i % 2 === 0];
}
foreach ($rows as $row) {
	echo $row["id"] . ":" . strtoupper($row["name"]) . ($row["even"] ? "+" : "-") . "\n";
}
`

// BenchmarkServeEntrypoint measures one request against a site's document root
// with the setting off and on, on each engine. The difference is what an
// entrypoint pays while it is parsed per request: the parse itself, and the
// expression closures and the bytecode that are keyed by the AST it produced.
func BenchmarkServeEntrypoint(b *testing.B) {
	root := b.TempDir()
	write(b, filepath.Join(root, "public", "index.php"), benchPage)

	// The pass logs one line per site, which is not what a benchmark run is
	// reporting.
	log.SetOutput(io.Discard)
	b.Cleanup(func() { log.SetOutput(os.Stderr) })

	for _, test := range []struct {
		name string
		on   bool
		flat bool
	}{
		{name: "interpreter/lazy"},
		{name: "interpreter/precompiled", on: true},
		{name: "flatstack/lazy", flat: true},
		{name: "flatstack/precompiled", on: true, flat: true},
	} {
		b.Run(test.name, func(b *testing.B) {
			options := runner.Options{Precompile: test.on}
			files, err := newHandler(os.DirFS(root), root, DefaultDocumentRoot, options, test.flat, false)
			if err != nil {
				b.Fatal(err)
			}
			precompile(files, root)

			b.ReportAllocs()
			for b.Loop() {
				if response := get(b, files, "localhost", "/"); response.Code != http.StatusOK {
					b.Fatalf("status = %d", response.Code)
				}
			}
		})
	}
}
