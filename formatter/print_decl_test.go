package formatter_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/formatter"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
)

func TestCommentsImmediatelyPrecedeClassAndFunctionDeclarations(t *testing.T) {
	t.Parallel()
	in := `<?php

/** Class docs. */


class Example {
	// Method docs.

	public function run() {}
}

/* Function
 * docs.
 */

function helper() {}
`
	want := `<?php

/** Class docs. */
class Example {
	// Method docs.
	public function run() {
	}
}

/* Function
 * docs.
 */
function helper() {
}
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != want {
		t.Fatalf("unexpected declaration comments:\n--- got ---\n%s--- want ---\n%s", out, want)
	}
	again, err := formatter.Source(out)
	if err != nil || again != out {
		t.Fatalf("output is not idempotent: %v\nfirst: %q\nsecond: %q", err, out, again)
	}
}

func TestDropCloseTagForClassOnlyFile(t *testing.T) {
	t.Parallel()
	in := "<?php\nclass Foo\n{\n}\n?>\n"
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "?>") {
		t.Fatalf("?> not removed:\n%s", out)
	}
}

func TestClassModifiersRoundTrip(t *testing.T) {
	t.Parallel()
	in := `<?php
namespace App;

final class Sealed {
	function id() {
		return 1;
	}
}

abstract class Base {
}

readonly class Frozen {
}

readonly final class Both {
}
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"final class Sealed {",
		"abstract class Base {",
		"readonly class Frozen {",
		// The printer puts modifiers in PHP's canonical order.
		"final readonly class Both {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	again, err := formatter.Source(out)
	if err != nil {
		t.Fatalf("formatted output does not reformat: %v\n%s", err, out)
	}
	if again != out {
		t.Fatalf("output is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

func TestClassHeritageRoundTrip(t *testing.T) {
	t.Parallel()
	in := `<?php
namespace App;

use Vendor\Framework\TestCase;

final class Suite extends TestCase implements \Countable, Local {
	function count(): int {
		return 1;
	}
}
`
	out, err := formatter.Source(in)
	if err != nil {
		t.Fatal(err)
	}
	// A parent imported from another namespace prints as the file imported
	// it. Expanding it would leave the use statement above with nothing
	// referring to it, and rewrite code the author wrote one way into
	// another. A name with no import still prints fully qualified, which is
	// what \Countable shows here.
	want := `final class Suite extends TestCase implements \Countable, Local {`
	if !strings.Contains(out, want) {
		t.Fatalf("missing %q in:\n%s", want, out)
	}
	again, err := formatter.Source(out)
	if err != nil {
		t.Fatalf("formatted output does not reformat: %v\n%s", err, out)
	}
	if again != out {
		t.Fatalf("output is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
	cd := parseClassDecl(t, out)
	if cd.Parent != `Vendor\Framework\TestCase` {
		t.Errorf("reparsed parent = %q, want Vendor\\Framework\\TestCase", cd.Parent)
	}
	if len(cd.Implements) != 2 || cd.Implements[0] != "Countable" || cd.Implements[1] != `App\Local` {
		t.Errorf("reparsed implements = %v, want [Countable App\\Local]", cd.Implements)
	}
}

// parseClassDecl returns the single class declaration in src.
func parseClassDecl(t *testing.T, src string) *model.ClassDecl {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	for _, s := range prog.Stmts {
		if cd, ok := s.(*model.ClassDecl); ok {
			return cd
		}
	}
	t.Fatalf("no class declaration in:\n%s", src)
	return nil
}
