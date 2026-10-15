package runner

import (
	"testing"
)

// TestFuncSite covers the three shapes a declaration site prints in. PHP's
// fatal error for a redeclared function names the first site, and a script
// carrying two declarations through a doubled include is told the file, which
// is the only thing naming the include.
//
// A site with no file is what a host passes in when it built the AST itself,
// and a site with no line is a program carrying no line table. Both answer
// something a message can be written around.
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
