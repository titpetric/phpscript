package annotations_test

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
)

var testScannerFileSystem = fstest.MapFS{
	"app/deep/nested/handler.php": {Data: []byte(`<?php
// @route GET /nested
echo "nested";
`)},
	"vendor/acme/lib/routes.php": {Data: []byte(`<?php
// @route GET /vendored
echo "vendored";
`)},
	"public/annotated.php": {Data: []byte(`<?php
// @route GET /public
echo "public";
`)},
	"notes.txt":  {Data: []byte("// @route GET /notes")},
	"helper.php": {Data: []byte("<?php\nfunction helper() { return 1; }\n")},
}

// TestScannerReadsOnlyWhatAnApplicationPublishes covers which files discovery
// reads. A .php file is read at any depth, so routes can sit in one folder or
// in a tree of them. A composer install tree is skipped: a dependency does not
// publish routes into the application, and walking it costs a parse of every
// vendored file. A file with no annotation and a file with another extension
// register nothing.
func TestScannerReadsOnlyWhatAnApplicationPublishes(t *testing.T) {
	t.Parallel()
	registrar := &recordingRegistrar{}
	if err := annotations.NewRoute(testScannerFileSystem).Register(registrar); err != nil {
		t.Fatal(err)
	}

	got := slices.Clone(registrar.handled)
	slices.Sort(got)
	want := []string{"GET /nested", "GET /public"}
	if !slices.Equal(got, want) {
		t.Fatalf("registered %v, want %v", got, want)
	}
}

// TestScannerSkipsAnExcludedDirectory covers the directory a host keeps out of
// discovery by name. A document root served as static files holds .php files a
// server already answers for, and registering them a second time as endpoints
// would publish two ways to reach one file.
func TestScannerSkipsAnExcludedDirectory(t *testing.T) {
	t.Parallel()
	registrar := &recordingRegistrar{}
	route := annotations.NewRoute(testScannerFileSystem, annotations.WithExcludedDirectory("public"))
	if err := route.Register(registrar); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(registrar.handled, []string{"GET /nested"}) {
		t.Fatalf("registered %v, want the nested route alone", registrar.handled)
	}
}

// TestScannerRefusesANilRoot covers a host that configured no source tree.
// Discovery walks one filesystem, so there is nothing to scan and nothing to
// serve, and the failure arrives at registration where a deployment sees it.
func TestScannerRefusesANilRoot(t *testing.T) {
	t.Parallel()
	err := annotations.NewRoute(nil).Register(&recordingRegistrar{})
	if err == nil {
		t.Fatal("a nil root filesystem was accepted")
	}
	if got := err.Error(); got != "annotations: nil root filesystem" {
		t.Errorf("error = %q, want it to name the missing filesystem", got)
	}
}
