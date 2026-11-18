package runner

import (
	"testing"
)

// TestFuncSite covers the shapes a declaration site prints in. PHP's fatal
// error for a redeclared function names the first site. For a script with two
// declarations through a doubled include, that file is the only thing naming
// the include.
//
// A site holding no file arrives when a host built the AST itself. A site
// holding no line is a program whose line table is empty. Both answer something a
// caller can write a message around.
func TestFuncSite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		site FuncSite
		want string
	}{
		{FuncSite{File: "/srv/app/lib.php", Line: 42}, "/srv/app/lib.php:42"},
		{FuncSite{File: "/srv/app/lib.php"}, "/srv/app/lib.php"},
		{FuncSite{}, "an earlier declaration"},
		{FuncSite{Line: 42}, "an earlier declaration"},
	} {
		if got := tc.site.String(); got != tc.want {
			t.Errorf("FuncSite%+v.String() = %q, want %q", tc.site, got, tc.want)
		}
	}
}
