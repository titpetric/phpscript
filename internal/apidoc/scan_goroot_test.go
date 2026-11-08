package apidoc

import (
	"strings"
	"testing"
)

// TestGorootDocs covers what the Go distribution answers about a standard
// library type a class binds. HTTP\Request holds 22 properties and 20
// methods whose godoc is in the distribution and not in this module. The
// scan of the module tree reaches none of it.
func TestGorootDocs(t *testing.T) {
	t.Parallel()
	docs := gorootDocs("net/http", "Request")
	if docs == nil {
		t.Skip("the Go distribution is not on disk, and the generator runs where it is")
	}

	method, ok := docs.Methods["AddCookie"]
	if !ok {
		t.Fatalf("net/http.Request documents no AddCookie, methods = %d", len(docs.Methods))
	}
	// The scan rewrites the leading Go name to the PHP one, and the
	// published line reads as PHP documentation and not as Go's.
	if len(method.Comment) == 0 || !strings.HasPrefix(method.Comment[0], "add_cookie ") {
		t.Errorf("AddCookie documents %q, want it to open on the PHP spelling", method.Comment)
	}
	// The parameter name is the one the distribution declares. Reflection
	// invents $value2 here, which reads as the argument's real spelling.
	if got, want := strings.Join(method.Params, ", "), "c"; got != want {
		t.Errorf("AddCookie parameters = %q, want %q", got, want)
	}

	if lines := docs.Fields["Method"]; len(lines) == 0 {
		t.Error("net/http.Request documents no Method property")
	}
	if got, ok := docs.Fields["ProtoMajor"]; ok {
		t.Errorf("the trailing note on ProtoMajor is published as a docblock: %q", got)
	}
}

// TestGorootDocsUnknown covers the two misses. A package not in the
// distribution and a type it does not declare both answer nil, and the listing
// then prints the structure reflection sees and no prose.
func TestGorootDocsUnknown(t *testing.T) {
	t.Parallel()
	if got := gorootDocs("github.com/titpetric/phpscript/stdlib/ps/http", "Client"); got != nil {
		t.Errorf("a module package answered %+v, want nothing", got)
	}
	if got := gorootDocs("net/http", "NoSuchTypeHere"); got != nil {
		t.Errorf("an undeclared type answered %+v, want nothing", got)
	}
}
