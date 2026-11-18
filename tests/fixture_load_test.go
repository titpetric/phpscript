package tests

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestParseGolden covers the second fixture form: a _test.php body php can run
// where it lies, and the _test.txt beside it. ParseGolden takes both as they
// are, with CRLF normalised so a file checked out on Windows compares equal,
// and the trailing newlines of the expected output dropped.
func TestParseGolden(t *testing.T) {
	fx := ParseGolden(
		[]byte("<?php\r\necho \"ok\";\r\n"),
		[]byte("ok\r\n\n"),
		filepath.FromSlash("fixtures/area/echo_test.php"),
	)

	if fx.Name != "echo" {
		t.Errorf("Name = %q, want \"echo\"", fx.Name)
	}
	if want := "<?php\necho \"ok\";\n"; fx.PHP != want {
		t.Errorf("PHP = %q, want %q", fx.PHP, want)
	}
	if fx.Expected != "ok" {
		t.Errorf("Expected = %q, want \"ok\"", fx.Expected)
	}
	if want := filepath.FromSlash("fixtures/area/echo_test.php"); fx.Path != want {
		t.Errorf("Path = %q, want %q", fx.Path, want)
	}
}

// TestFindFixtures covers discovery on a real tree: both fixture forms, in
// path order, leaving out a _test.php whose expected output is missing. A path
// ending in /... walks the subdirectories; a plain path reads only the named
// directory.
func TestFindFixtures(t *testing.T) {
	dir := writeTree(t,
		"doc.phpt", "name: doc\ndescription: a document\n---\n<?php echo \"ok\";\n---\nok",
		"body_test.php", "<?php echo \"ok\";\n",
		"body_test.txt", "ok\n",
		// A body whose expected output is missing stays an ordinary php file
		// in the tree; the finder skips it.
		"loose_test.php", "<?php echo \"loose\";\n",
		"nested/deep.phpt", "name: deep\ndescription: a nested document\n---\n<?php echo \"ok\";\n---\nok",
	)

	flat, err := FindFixtures([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixtureNames(flat), []string{"body", "doc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindFixtures(dir) = %v, want %v", got, want)
	}

	deep, err := FindFixtures([]string{filepath.Join(dir, "...")})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixtureNames(deep), []string{"body", "deep", "doc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindFixtures(dir/...) = %v, want %v", got, want)
	}

	one, err := FindFixtures([]string{filepath.Join(dir, "doc.phpt")})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixtureNames(one), []string{"doc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindFixtures(file) = %v, want %v", got, want)
	}
}
