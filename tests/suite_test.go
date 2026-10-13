package tests

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/config"
)

// writeTree lays a temporary directory down on disk from path/contents pairs,
// and answers where it is. A path naming a subdirectory creates it.
func writeTree(t *testing.T, files ...string) string {
	t.Helper()

	dir := t.TempDir()
	for i := 0; i+1 < len(files); i += 2 {
		name := filepath.Join(dir, filepath.FromSlash(files[i]))
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(files[i+1]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// writeSuite is writeTree with body written to the phpscript.yml a suite root
// configures itself with.
func writeSuite(t *testing.T, body string, files ...string) string {
	t.Helper()
	return writeTree(t, append([]string{SuiteFile, body}, files...)...)
}

// TestSuite covers reading one phpscript.yml off a suite root: the directory
// as the caller spelled it, the settings the file named, and the keys it
// named apart from the ones it inherited. A folder holding no such file is no
// suite root, so the load answers nothing and no error.
func TestSuite(t *testing.T) {
	dir := writeSuite(t, "test:\n  include: prelude.php\n  hooks:\n    setup: hooks/up.php\n    teardown: hooks/down.php\n")

	suite, err := LoadSuite(dir, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if suite == nil {
		t.Fatal("LoadSuite answered nothing for a directory holding a phpscript.yml")
	}
	if suite.Dir != dir {
		t.Errorf("Dir = %q, want %q", suite.Dir, dir)
	}
	if got := suite.Config.Test.Include; got != "prelude.php" {
		t.Errorf("Config.Test.Include = %q, want \"prelude.php\"", got)
	}
	if !config.Declares(suite.Declared, "test") {
		t.Error("Declared does not carry the test key the file named")
	}
	if config.Declares(suite.Declared, "env") {
		t.Error("Declared carries env, a key the file did not name")
	}

	empty, err := LoadSuite(t.TempDir(), config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if empty != nil {
		t.Errorf("LoadSuite answered %+v for a directory holding no phpscript.yml", empty)
	}
}

// TestSuite_run covers the keys a run of the whole command owns. Two folders
// answering differently for parallel would leave the run with nothing to act
// on, so a file discovered below the invocation root is refused and names the
// key.
func TestSuite_run(t *testing.T) {
	dir := writeSuite(t, "test:\n  parallel: 4\n")

	if _, err := LoadSuite(dir, config.New(), true); err == nil {
		t.Fatal("LoadSuite accepted test.parallel below the invocation root")
	} else if !strings.Contains(err.Error(), "test.parallel") {
		t.Errorf("error = %v, want it to name test.parallel", err)
	}

	suite, err := LoadSuite(dir, config.New(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := suite.Config.Test.Parallel; got != 4 {
		t.Errorf("Config.Test.Parallel = %d for the run's own file, want 4", got)
	}
}

// TestLoadSuiteFS covers the same read over a tree that is not on disk. The
// fixtures compiled into the test binary are reached this way, and the
// run-wide keys are refused for all of them: an embedded tree has no
// invocation root above it.
func TestLoadSuiteFS(t *testing.T) {
	root := fstest.MapFS{
		"area/phpscript.yml": &fstest.MapFile{Data: []byte("test:\n  include: prelude.php\n")},
		"area/prelude.php":   &fstest.MapFile{Data: []byte("<?php\n")},
	}

	suite, err := LoadSuiteFS(root, "area", config.New())
	if err != nil {
		t.Fatal(err)
	}
	if suite == nil {
		t.Fatal("LoadSuiteFS answered nothing for a directory holding a phpscript.yml")
	}
	if suite.Dir != "area" {
		t.Errorf("Dir = %q, want \"area\"", suite.Dir)
	}
	if got := suite.Include(); got != "prelude.php" {
		t.Errorf("Include() = %q, want \"prelude.php\"", got)
	}

	empty, err := LoadSuiteFS(root, ".", config.New())
	if err != nil {
		t.Fatal(err)
	}
	if empty != nil {
		t.Errorf("LoadSuiteFS answered %+v for a directory holding no phpscript.yml", empty)
	}
}

// TestSuite_Provider covers the connections the fixtures below a suite root
// resolve. A folder that named no env of its own answers nothing, so the
// binding stays on the process environment; one that named an env gets only
// what it named, the rule a virtual host is already held to.
func TestSuite_Provider(t *testing.T) {
	declared := writeSuite(t, "env:\n  - \"PLATFORM_DB_DEFAULT=sqlite://:memory:\"\n")
	suite, err := LoadSuite(declared, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	provider := suite.Provider()
	if provider == nil {
		t.Fatal("Provider() answered nothing for a suite that declared env")
	}
	if second := suite.Provider(); second != provider {
		t.Error("Provider() built a second provider; the setup hook and the fixtures must share one pool")
	}

	plain := writeSuite(t, "test:\n  include: prelude.php\n")
	inherited, err := LoadSuite(plain, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := inherited.Provider(); got != nil {
		t.Errorf("Provider() = %v for a suite that declared no env, want nothing", got)
	}

	var absent *Suite
	if got := absent.Provider(); got != nil {
		t.Errorf("Provider() = %v on a nil suite, want nothing", got)
	}
}

// TestSuite_Mail covers the mail servers a fixture may name. A suite that
// declared no mail block answers nothing, so a fixture cannot reach the
// operator's credentials; one that declared the key holds the servers it
// named and no others.
func TestSuite_Mail(t *testing.T) {
	declared := writeSuite(t, "mail:\n")
	suite, err := LoadSuite(declared, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	provider := suite.Mail()
	if provider == nil {
		t.Fatal("Mail() answered nothing for a suite that declared the mail key")
	}
	if second := suite.Mail(); second != provider {
		t.Error("Mail() built a second provider")
	}

	plain := writeSuite(t, "test:\n  include: prelude.php\n")
	inherited, err := LoadSuite(plain, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := inherited.Mail(); got != nil {
		t.Errorf("Mail() = %v for a suite that declared no mail block, want nothing", got)
	}

	var absent *Suite
	if got := absent.Mail(); got != nil {
		t.Errorf("Mail() = %v on a nil suite, want nothing", got)
	}
}

// TestSuite_Include covers the prelude the fixtures below a suite root load.
// A nil suite answers "", so a fixture tree with no phpscript.yml takes the
// same path as one whose file named no include.
func TestSuite_Include(t *testing.T) {
	dir := writeSuite(t, "test:\n  include: bootstrap.php\n")
	suite, err := LoadSuite(dir, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := suite.Include(); got != "bootstrap.php" {
		t.Errorf("Include() = %q, want \"bootstrap.php\"", got)
	}

	var absent *Suite
	if got := absent.Include(); got != "" {
		t.Errorf("Include() = %q on a nil suite, want \"\"", got)
	}
}

// TestSuite_Setup covers the hook named for the state the fixtures read. A
// nil suite and a file that named no hook both answer "", which RunSetup
// reads as nothing to run.
func TestSuite_Setup(t *testing.T) {
	dir := writeSuite(t, "test:\n  hooks:\n    setup: hooks/up.php\n")
	suite, err := LoadSuite(dir, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := suite.Setup(); got != "hooks/up.php" {
		t.Errorf("Setup() = %q, want \"hooks/up.php\"", got)
	}
	if got := suite.Teardown(); got != "" {
		t.Errorf("Teardown() = %q for a file that named no teardown, want \"\"", got)
	}

	var absent *Suite
	if got := absent.Setup(); got != "" {
		t.Errorf("Setup() = %q on a nil suite, want \"\"", got)
	}
}

// TestSuite_Teardown covers the hook that releases what the setup laid down.
// It is read the same way Setup is, so a run that ended badly still has a
// name to execute.
func TestSuite_Teardown(t *testing.T) {
	dir := writeSuite(t, "test:\n  hooks:\n    teardown: hooks/down.php\n")
	suite, err := LoadSuite(dir, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := suite.Teardown(); got != "hooks/down.php" {
		t.Errorf("Teardown() = %q, want \"hooks/down.php\"", got)
	}

	var absent *Suite
	if got := absent.Teardown(); got != "" {
		t.Errorf("Teardown() = %q on a nil suite, want \"\"", got)
	}
}

// TestSuite_RunHook covers the runtime one hook executes on. The suite root is
// its filesystem, so a hook names its files the way an application does, and
// the suite's prelude is loaded ahead of it: a hook calling the application's
// own helpers needs what the autoloader declared. A name that is not in the
// tree fails before any of that.
func TestSuite_RunHook(t *testing.T) {
	dir := writeSuite(t,
		"test:\n  include: prelude.php\n  hooks:\n    setup: hooks/up.php\n",
		"prelude.php", "<?php\nfunction suite_tag() { return \"tagged\"; }\n",
		"hooks/up.php", "<?php\necho suite_tag() . \":\" . basename(__FILE__);\n",
	)
	suite, err := LoadSuite(dir, config.New(), true)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := suite.RunHook(t.Context(), suite.Setup(), &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "tagged:up.php"; got != want {
		t.Errorf("hook output = %q, want %q", got, want)
	}

	if err := suite.RunHook(t.Context(), "hooks/absent.php", io.Discard); err == nil {
		t.Error("RunHook answered no error for a hook that is not in the tree")
	}
}

// TestRunSetup covers the order the setup hooks run in: outermost first, so an
// inner suite reads the state its parent laid down. The run stops at the first
// failure, because fixtures with no state to assert against would report the
// same missing table once each.
func TestRunSetup(t *testing.T) {
	suites := []*Suite{
		loadHookSuite(t, "outer", "hooks/up.php", ""),
		loadHookSuite(t, "inner", "hooks/up.php", ""),
	}

	var out strings.Builder
	if err := RunSetup(t.Context(), suites, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "outer\ninner\n"; got != want {
		t.Errorf("setup output = %q, want %q", got, want)
	}

	broken := []*Suite{loadHookSuite(t, "first", "hooks/missing.php", ""), suites[1]}
	var failed strings.Builder
	err := RunSetup(t.Context(), broken, &failed)
	if err == nil {
		t.Fatal("RunSetup answered no error for a hook that is not in the tree")
	}
	if !strings.Contains(err.Error(), "test setup") {
		t.Errorf("error = %v, want it to name the setup hook", err)
	}
	if failed.String() != "" {
		t.Errorf("output = %q after the first failure, want nothing", failed.String())
	}
}

// TestRunTeardown covers the reverse order, and that every hook runs. The
// suites are independent, so a caller learns about all of the failures and not
// the one that happened to be innermost.
func TestRunTeardown(t *testing.T) {
	suites := []*Suite{
		loadHookSuite(t, "outer", "", "hooks/down.php"),
		loadHookSuite(t, "inner", "", "hooks/down.php"),
	}

	var out strings.Builder
	if err := RunTeardown(t.Context(), suites, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "inner\nouter\n"; got != want {
		t.Errorf("teardown output = %q, want %q", got, want)
	}

	broken := []*Suite{
		loadHookSuite(t, "a", "", "hooks/missing.php"),
		loadHookSuite(t, "b", "", "hooks/missing.php"),
	}
	err := RunTeardown(t.Context(), broken, io.Discard)
	if err == nil {
		t.Fatal("RunTeardown answered no error for two hooks that are not in the tree")
	}
	if got, want := strings.Count(err.Error(), "test teardown"), 2; got != want {
		t.Errorf("the joined error names %d teardowns, want %d", got, want)
	}
}

// loadHookSuite is a suite root whose hooks echo tag, for the ordering tests.
func loadHookSuite(t *testing.T, tag, setup, teardown string) *Suite {
	t.Helper()

	body := "test:\n  hooks:\n"
	if setup != "" {
		body += "    setup: " + setup + "\n"
	}
	if teardown != "" {
		body += "    teardown: " + teardown + "\n"
	}

	files := []string{"hooks/up.php", "<?php\necho \"" + tag + "\\n\";\n"}
	files = append(files, "hooks/down.php", "<?php\necho \""+tag+"\\n\";\n")

	suite, err := LoadSuite(writeSuite(t, body, files...), config.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	return suite
}
