// Package strings binds the day-to-day functions of Go's strings package as
// the Runtime\Go\Strings namespace. A script imports it with
// `use Runtime\Go\Strings;` and calls `Strings\Fields($s)`; the default
// runtime defines none of these names.
package strings

import (
	gostrings "strings"

	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
)

// repeatLimit caps the bytes Repeat builds, 256 MiB.
const repeatLimit = 256 << 20

// Namespace is the name a script imports.
const Namespace = modelutil.GoNamespace + `\Strings`

func init() {
	runner.RegisterNamespace(Namespace, Register)
}

func name(fn string) string { return Namespace + `\` + fn }

// Register installs the Runtime\Go\Strings functions on a runtime that imported them.
func Register(rt *runner.Runtime) {
	// Runtime\Go\Strings\Fields splits $s around runs of white space.
	rt.RegisterFunc(name("Fields"), func(s string) []string { return gostrings.Fields(s) })
	// Runtime\Go\Strings\Split slices $s into all substrings separated by $sep.
	rt.RegisterFunc(name("Split"), func(s, sep string) []string { return gostrings.Split(s, sep) })
	// Runtime\Go\Strings\Contains reports whether $substr is within $s.
	rt.RegisterFunc(name("Contains"), func(s, substr string) bool { return gostrings.Contains(s, substr) })
	// Runtime\Go\Strings\ContainsAny reports whether any character of $chars is within $s.
	rt.RegisterFunc(name("ContainsAny"), func(s, chars string) bool { return gostrings.ContainsAny(s, chars) })
	// Runtime\Go\Strings\HasPrefix reports whether $s begins with $prefix.
	rt.RegisterFunc(name("HasPrefix"), func(s, prefix string) bool { return gostrings.HasPrefix(s, prefix) })
	// Runtime\Go\Strings\HasSuffix reports whether $s ends with $suffix.
	rt.RegisterFunc(name("HasSuffix"), func(s, suffix string) bool { return gostrings.HasSuffix(s, suffix) })
	// Runtime\Go\Strings\Index returns the byte index of the first $substr in $s, or -1.
	rt.RegisterFunc(name("Index"), func(s, substr string) int { return gostrings.Index(s, substr) })
	// Runtime\Go\Strings\LastIndex returns the byte index of the last $substr in $s, or -1.
	rt.RegisterFunc(name("LastIndex"), func(s, substr string) int { return gostrings.LastIndex(s, substr) })
	// Runtime\Go\Strings\Count counts the non-overlapping instances of $substr in $s.
	rt.RegisterFunc(name("Count"), func(s, substr string) int { return gostrings.Count(s, substr) })
	// Runtime\Go\Strings\Repeat returns $s repeated $count times.
	rt.RegisterFunc(name("Repeat"), func(s string, count int) (string, error) {
		if count < 0 {
			return "", runner.NewRuntimeException("strings: negative Repeat count", 0)
		}
		if len(s) > 0 && count > repeatLimit/len(s) {
			return "", runner.NewRuntimeException("strings: Repeat output too large", 0)
		}
		return gostrings.Repeat(s, count), nil
	})
	// Runtime\Go\Strings\ReplaceAll replaces every $old in $s with $new.
	rt.RegisterFunc(name("ReplaceAll"), func(s, old, replacement string) string {
		return gostrings.ReplaceAll(s, old, replacement)
	})
	// Runtime\Go\Strings\ToUpper returns $s with its letters in upper case.
	rt.RegisterFunc(name("ToUpper"), func(s string) string { return gostrings.ToUpper(s) })
	// Runtime\Go\Strings\ToLower returns $s with its letters in lower case.
	rt.RegisterFunc(name("ToLower"), func(s string) string { return gostrings.ToLower(s) })
	// Runtime\Go\Strings\TrimSpace returns $s without leading and trailing white space.
	rt.RegisterFunc(name("TrimSpace"), func(s string) string { return gostrings.TrimSpace(s) })
	// Runtime\Go\Strings\Trim returns $s without the leading and trailing characters of $cutset.
	rt.RegisterFunc(name("Trim"), func(s, cutset string) string { return gostrings.Trim(s, cutset) })
	// Runtime\Go\Strings\TrimPrefix returns $s without the leading $prefix.
	rt.RegisterFunc(name("TrimPrefix"), func(s, prefix string) string { return gostrings.TrimPrefix(s, prefix) })
	// Runtime\Go\Strings\TrimSuffix returns $s without the trailing $suffix.
	rt.RegisterFunc(name("TrimSuffix"), func(s, suffix string) string { return gostrings.TrimSuffix(s, suffix) })
	// Runtime\Go\Strings\EqualFold reports whether $s and $t are equal under Unicode case-folding.
	rt.RegisterFunc(name("EqualFold"), func(s, t string) bool { return gostrings.EqualFold(s, t) })
}
