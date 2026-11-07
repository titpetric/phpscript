package io_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/internal/gotest"
	"github.com/titpetric/phpscript/runner"
	goio "github.com/titpetric/phpscript/stdlib/go/io"
)

// TestSet covers the area over the one reader a script reaches: the body of a
// response, which `Runtime\Go\Net\HTTP` answers with. The bytes come back as
// php's string, as every `[]byte` does.
func TestSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()

	const header = `use Runtime\Go; import "net/http"; import "io"; `
	gotest.Run(t, goio.Namespace, []gotest.Case{
		{
			Src:  header + fmt.Sprintf(`$r = http.Get(%q); echo $r->StatusCode, io.ReadAll($r->Body);`, server.URL),
			Want: "200hello",
		},
		{
			Src:  header + fmt.Sprintf(`$r = http.Get(%q); echo io.ReadFull($r->Body, 4);`, server.URL),
			Want: "hell",
		},
		{Src: `echo function_exists('Runtime\Go\IO\ReadAll') ? "carried" : "absent";`, Want: "absent"},
	})
}

// TestNamespace pins the registration an import resolves to.
func TestNamespace(t *testing.T) {
	if goio.Namespace != `Runtime\Go\IO` {
		t.Errorf("the area is %s", goio.Namespace)
	}
	if _, ok := runner.NamespaceInstaller(goio.Namespace); !ok {
		t.Error("the area is not registered")
	}
}
