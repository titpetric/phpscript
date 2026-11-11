package http_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
	"github.com/titpetric/phpscript/runner"
	gohttp "github.com/titpetric/phpscript/stdlib/go/net/http"
)

// TestSet covers the area through a script: the names the import binds, the
// constants beside them, and the types the signatures reach, which answer as
// classes and name their fields.
func TestSet(t *testing.T) {
	const header = `use Runtime\Go; import "net/http"; `
	gotest.Run(t, gohttp.Namespace, []gotest.Case{
		{Src: header + `echo http.StatusText(404);`, Want: "Not Found"},
		{Src: header + `echo http.StatusNotFound, "|", http.MethodPost;`, Want: "404|POST"},
		{Src: header + `echo http.CanonicalHeaderKey("content-type");`, Want: "Content-Type"},
		{Src: header + `echo http.DetectContentType("<html>");`, Want: "text/html; charset=utf-8"},
		// A request the signature reaches: the class, the fields, the nested
		// value and a method on it.
		{Src: header + `$r = http.NewRequest("GET", "https://example.com/a?b=1", null); echo $r->Method, $r->URL->Path;`, Want: "GET/a"},
		{Src: header + `$r = http.NewRequest("GET", "https://example.com/", null); $r->Method = "PUT"; echo $r->Method;`, Want: "PUT"},
		{Src: header + `$r = new http.Request; var_dump($r->Method);`, Want: "string(0) \"\"\n"},
		{Src: header + `echo len(make([]http.Request, 2));`, Want: "2"},
		// A url a script cannot parse throws, as the error of a binding does.
		{Src: header + `try { http.NewRequest("GET", "://", null); } catch (Throwable $e) { echo "threw"; }`, Want: "threw"},
		// The default runtime defines none of it.
		{Src: `echo function_exists('Runtime\Go\Net\HTTP\StatusText') ? "defined" : "absent";`, Want: "absent"},
	})
}

// TestClientDo covers the round trip a script makes through the class the walk
// found and no bound function names: `new http.Client`, a header written into
// the map a request holds, then the body of the response.
func TestClientDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Method, " ", r.URL.Path, " ", r.Header.Get("Accept"))
	}))
	defer server.Close()

	const header = `use Runtime\Go; import "net/http"; import "io"; `
	gotest.Run(t, gohttp.Namespace, []gotest.Case{
		{
			Src: header + fmt.Sprintf(`
$client = new http.Client;
$req = http.NewRequest("GET", %q, null);
$req->Header->Set("Accept", "application/json");
$resp = $client->Do($req);
echo $resp->StatusCode, "|", io.ReadAll($resp->Body);`, server.URL+"/a"),
			Want: "200|GET /a application/json",
		},
		{
			Src:  header + fmt.Sprintf(`$resp = (new http.Client)->Head(%q); echo $resp->StatusCode;`, server.URL),
			Want: "200",
		},
		// A host nothing answers on throws, as the error of a binding does.
		{
			Src:  header + `try { (new http.Client)->Get("http://127.0.0.1:1/"); } catch (Throwable $e) { echo "threw"; }`,
			Want: "threw",
		},
	})
}

// TestNamespace pins the registration an import resolves to, and the classes
// the type walk enumerated.
func TestNamespace(t *testing.T) {
	if gohttp.Namespace != `Runtime\Go\Net\HTTP` {
		t.Errorf("the area is %s", gohttp.Namespace)
	}
	if _, ok := runner.NamespaceInstaller(gohttp.Namespace); !ok {
		t.Error("the area is not registered")
	}
	rt := runner.New(io.Discard, runner.Options{})
	gohttp.Set.Register(rt)
	for _, name := range []string{"Request", "Response", "Cookie", "Client"} {
		if _, ok := rt.LookupConstructor(gohttp.Namespace + `\` + name); !ok {
			t.Errorf("%s is no class of the area", name)
		}
	}
	// A named map reaches a script as the value a binding answered with, and
	// `new http.Header` would answer a nil of it.
	if _, ok := rt.LookupConstructor(gohttp.Namespace + `\Header`); ok {
		t.Error("the header map is a class")
	}
}
