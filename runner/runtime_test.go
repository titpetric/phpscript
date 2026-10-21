package runner

import (
	"io"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner/coverage"
)

// coverageBlock finds the block starting at line in file, so a test asserts on
// position and not on ordering.
func coverageBlock(t *testing.T, blocks []coverage.Block, file string, line int) coverage.Block {
	t.Helper()
	for _, b := range blocks {
		if b.File == file && b.StartLine == line {
			return b
		}
	}
	t.Fatalf("no block for %s:%d in %+v", file, line, blocks)
	return coverage.Block{}
}

// TestCoverageCounts covers what the interpreter charges: a loop body once per
// iteration, the loop header once, and nothing for an untaken branch.
func TestCoverageCounts(t *testing.T) {
	t.Parallel()
	src := `<?php
$total = 0;
for ($i = 0; $i < 3; $i++) {
	$total = $total + $i;
}
if ($total > 100) {
	echo "big";
}
echo $total;
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := New(&out, Options{})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "3" {
		t.Fatalf("output = %q", out.String())
	}

	blocks := cov.Blocks()
	if got := coverageBlock(t, blocks, "main.php", 2).Count; got != 1 {
		t.Errorf("assignment count = %d, want 1", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 3); got.Count != 1 || got.EndLine != 3 {
		t.Errorf("for header = %+v, want count 1 clamped to its own line", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 4).Count; got != 3 {
		t.Errorf("loop body count = %d, want 3", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 7).Count; got != 0 {
		t.Errorf("untaken branch count = %d, want 0", got)
	}
}

// TestCoverageIncludedFiles covers the zero-count baseline: an included file
// registers every statement, so a function body the fixture never calls shows
// up at zero under the include's own filename.
func TestCoverageIncludedFiles(t *testing.T) {
	t.Parallel()
	root := fstest.MapFS{
		"lib.php": &fstest.MapFile{Data: []byte(`<?php
function used()
{
	return "used";
}

function unused()
{
	return "unused";
}
`)},
	}
	prog, err := parser.Parse(`<?php
include "lib.php";
echo used();
`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := New(&out, Options{RootFS: root})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "used" {
		t.Fatalf("output = %q", out.String())
	}

	blocks := cov.Blocks()
	if got := coverageBlock(t, blocks, "lib.php", 4).Count; got != 1 {
		t.Errorf("used() body count = %d, want 1", got)
	}
	if got := coverageBlock(t, blocks, "lib.php", 9).Count; got != 0 {
		t.Errorf("unused() body count = %d, want 0", got)
	}
	for _, b := range blocks {
		if b.File == "lib.php" && b.StartLine == 2 {
			t.Errorf("function declaration registered as a block: %+v", b)
		}
	}
}

// TestCoverageForcesInterpreter holds the flatstack boundary: a runtime created
// with NewFlatStack executes interpreted while it holds a collector, so the
// counts exist and the bytecode backend needs no coverage support.
func TestCoverageForcesInterpreter(t *testing.T) {
	t.Parallel()
	prog, err := parser.Parse(`<?php
echo "covered";
`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := NewFlatStack(&out, Options{})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "covered" {
		t.Fatalf("output = %q", out.String())
	}
	if got := coverageBlock(t, cov.Blocks(), "main.php", 2).Count; got != 1 {
		t.Errorf("echo count = %d, want 1", got)
	}
}

// stubDatabase and stubMail stand in for the providers a host configures. Each
// embeds the interface and declares no method: these tests read which value the
// runtime answers with and never call through it, and implementing Open would
// put the sql driver in this package's test binary for nothing.
type stubDatabase struct{ model.DatabaseProvider }

type stubMail struct{ model.MailProvider }

// TestRuntimeAnswersForItsOptions covers the accessors a binding reads its
// configuration through. Each answers what the host passed and nothing it
// inferred: a runtime with no source root answers nil and no working
// directory, and no provider and no default, because the binding settles what
// a missing one falls back to.
func TestRuntimeAnswersForItsOptions(t *testing.T) {
	t.Parallel()
	bare := New(io.Discard, Options{})
	if got := bare.SAPI(); got != "" {
		t.Errorf("SAPI() = %q on a runtime that named none, want empty", got)
	}
	if got := bare.FS(); got != nil {
		t.Errorf("FS() = %v with no source root, want nil", got)
	}
	if got := bare.Database(); got != nil {
		t.Errorf("Database() = %v with none configured, want nil", got)
	}
	if got := bare.Mail(); got != nil {
		t.Errorf("Mail() = %v with none configured, want nil", got)
	}
	if got := bare.MemoryLimit(); got != 0 {
		t.Errorf("MemoryLimit() = %s with none configured, want no limit", got)
	}
	if bare.Precompiled() {
		t.Error("Precompiled() is true on a runtime that was not configured to")
	}
	if got := bare.IncludePath(); got != "." {
		t.Errorf("IncludePath() = %q, want the working directory", got)
	}

	root := fstest.MapFS{"app/index.php": &fstest.MapFile{Data: []byte(`<?php`)}}
	stdin := strings.NewReader("piped")
	database, mail := &stubDatabase{}, &stubMail{}
	rt := New(io.Discard, Options{
		SAPI:          "cgi-phpscript",
		RootFS:        root,
		Stdin:         stdin,
		WorkDir:       "app",
		MemoryLimit:   8 << 20,
		Precompile:    true,
		Database:      database,
		Mail:          mail,
		WritablePaths: []string{"var", "tmp"},
	})

	if got := rt.SAPI(); got != "cgi-phpscript" {
		t.Errorf("SAPI() = %q, want the configured one", got)
	}
	if got := rt.FS(); got == nil {
		t.Error("FS() = nil with a source root configured")
	}
	if got := rt.Stdin(); got != stdin {
		t.Errorf("Stdin() = %v, want the configured reader", got)
	}
	if got := rt.Database(); got != model.DatabaseProvider(database) {
		t.Errorf("Database() = %v, want the configured provider", got)
	}
	if got := rt.Mail(); got != model.MailProvider(mail) {
		t.Errorf("Mail() = %v, want the configured provider", got)
	}
	if got := rt.MemoryLimit().Bytes(); got != 8<<20 {
		t.Errorf("MemoryLimit() = %d, want %d", got, 8<<20)
	}
	if !rt.Precompiled() {
		t.Error("Precompiled() is false on a runtime configured to precompile")
	}

	// The working directory has two spellings: the fs.FS path a binding
	// resolves against, and the one a script reads out of getcwd().
	if got := rt.WorkDir(); got != "app" {
		t.Errorf("WorkDir() = %q, want the fs path", got)
	}
	if got := rt.WorkDirPath(); got != "/app" {
		t.Errorf("WorkDirPath() = %q, want the path a script reads", got)
	}

	// WritablePaths answers a copy, so a binding that sorts or filters what
	// it holds does not edit the host's configuration.
	paths := rt.WritablePaths()
	if len(paths) != 2 {
		t.Fatalf("WritablePaths() = %v, want both", paths)
	}
	paths[0] = "edited"
	if again := rt.WritablePaths(); again[0] != "var" {
		t.Errorf("WritablePaths() = %v after the caller edited its copy, want the configured list", again)
	}
}

// TestRuntime_SetWorkDir covers chdir(): per-runtime state, so a script moving
// it moves nothing another request sees and os.Chdir is never called. A
// relative path resolves against the current directory, an absolute one
// against the source root, and neither climbs out of it. A path naming no
// directory is the false php documents.
func TestRuntime_SetWorkDir(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{RootFS: fstest.MapFS{
		"app/lib/helper.php": &fstest.MapFile{Data: []byte(`<?php`)},
		"app/index.php":      &fstest.MapFile{Data: []byte(`<?php`)},
	}})

	if !rt.SetWorkDir("app") {
		t.Fatal("SetWorkDir(app) refused a directory the root holds")
	}
	if got := rt.WorkDir(); got != "app" {
		t.Errorf("WorkDir() = %q, want app", got)
	}
	// A relative path resolves through the directory that is current now, so a
	// script's include and its file_get_contents name the same file.
	if got := rt.ResolvePath("index.php"); got != "app/index.php" {
		t.Errorf("ResolvePath(index.php) = %q, want app/index.php", got)
	}

	if !rt.SetWorkDir("lib") {
		t.Fatal("SetWorkDir(lib) refused a directory below the current one")
	}
	if got := rt.ResolvePath("helper.php"); got != "app/lib/helper.php" {
		t.Errorf("ResolvePath(helper.php) = %q, want app/lib/helper.php", got)
	}

	// SetWorkDir reads an absolute path from the source root and no host path.
	if !rt.SetWorkDir("/app") {
		t.Fatal("SetWorkDir(/app) refused the root-relative spelling")
	}
	if got := rt.WorkDirPath(); got != "/app" {
		t.Errorf("WorkDirPath() = %q, want /app", got)
	}

	// SetWorkDir refuses a path naming no directory and leaves the current one
	// alone, which is the false php documents.
	for _, dir := range []string{"nope", "index.php", "app/index.php"} {
		if rt.SetWorkDir(dir) {
			t.Errorf("SetWorkDir(%q) was accepted", dir)
		}
		if got := rt.WorkDir(); got != "app" {
			t.Fatalf("SetWorkDir(%q) moved the directory to %q", dir, got)
		}
	}

	// SetWorkDir clamps a climb and never refuses it: the path resolves to the
	// source root, so a script that walked up as far as it liked is standing at
	// the top of the tree the host gave it and never above one.
	if !rt.SetWorkDir("../../..") {
		t.Fatal("SetWorkDir refused to climb to the root")
	}
	if got := rt.WorkDir(); got != "." {
		t.Errorf("WorkDir() = %q after climbing, want the source root", got)
	}
	if got := rt.ResolvePath("app/index.php"); got != "app/index.php" {
		t.Errorf("ResolvePath = %q from the root, want app/index.php", got)
	}
}

// TestRuntime_IncludeFile covers the include a host performs itself, the call
// composer's autoloader pulls a declaration file in with: the file evaluates
// in a fresh scope the way an `include` statement would, and the runtime
// records it among the files it included, so the count a script reads and the
// count it publishes agree.
func TestRuntime_IncludeFile(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(io.Discard, Options{RootFS: fstest.MapFS{
		"lib/site.php": &fstest.MapFile{Data: []byte(`<?php class Site {} return "loaded";`)},
	}})
	rt.Observe(recorder)

	value, err := rt.IncludeFile("lib/site.php")
	if err != nil {
		t.Fatalf("IncludeFile: %v", err)
	}
	if value != "loaded" {
		t.Errorf("IncludeFile answered %v, want the file's return value", value)
	}
	if declared, err := rt.ClassExists("Site", false); err != nil || !declared {
		t.Error("the class the included file declares is not defined")
	}

	// The runtime writes the record from the source filesystem's root, the
	// spelling every path a script reads carries.
	included := rt.IncludedFiles()
	if !slices.Contains(included, "/lib/site.php") {
		t.Errorf("IncludedFiles = %v, want the file among them", included)
	}
	// The snapshot is a copy, so a caller walking it cannot edit the record.
	included[0] = "edited"
	if again := rt.IncludedFiles(); slices.Contains(again, "edited") {
		t.Errorf("IncludedFiles = %v after the caller edited its copy", again)
	}

	// UpdateIncludedFiles publishes the count to an observer; a host calls it
	// for the files it included outside the interpreter.
	rt.UpdateIncludedFiles(len(rt.IncludedFiles()))
	if len(recorder.included) == 0 {
		t.Fatal("no included-file count reached the observer")
	}
	if got := recorder.included[len(recorder.included)-1]; got != len(rt.IncludedFiles()) {
		t.Errorf("the observer was told %d files, want %d", got, len(rt.IncludedFiles()))
	}

	if _, err := rt.IncludeFile("lib/absent.php"); err == nil {
		t.Error("IncludeFile of a file the root does not hold reported no error")
	}
}

// TestRuntime_Reset covers what a pool does between two programs: ResetSession
// plus the caches keyed by AST node, which hold the previous tree alive. A
// runtime handed to the next program must not answer for the last one's
// classes, constants or parsed files.
func TestRuntime_Reset(t *testing.T) {
	t.Parallel()
	var first strings.Builder
	rt := New(&first, Options{
		Precompile: true,
		RootFS: fstest.MapFS{
			"lib.php": &fstest.MapFile{Data: []byte(`<?php class Site {}`)},
		},
	})
	// The constants a host froze are what a reset returns to, so the bindings
	// stay defined and whatever the program added does not. There is no
	// define() to use here: the bindings that register one import this
	// package.
	rt.SetConst("STDLIB_CONST", int64(1))
	rt.FreezeStdlib()
	rt.SetConst("LOADED", int64(2))

	program, err := rt.LoadFile("lib.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if programs, _, _ := rt.CompiledTree(); programs == 0 {
		t.Fatal("nothing was cached, so the clear below proves nothing")
	}

	var second strings.Builder
	rt.Reset(&second, nil)

	if programs, expressions, _ := rt.CompiledTree(); programs != 0 || expressions != 0 {
		t.Errorf("CompiledTree = %d programs, %d expressions after Reset, want both cleared", programs, expressions)
	}
	if declared, err := rt.ClassExists("Site", false); err != nil || declared {
		t.Error("a class the previous program declared survived Reset")
	}
	if _, defined := rt.Const("LOADED"); defined {
		t.Error("a constant added past the freeze survived Reset")
	}
	if _, defined := rt.Const("STDLIB_CONST"); !defined {
		t.Error("a frozen constant was cleared by Reset, so the bindings would be undefined")
	}

	// Output goes to the writer Reset holds, so the next program's text
	// does not join the previous one's.
	echo, err := rt.Load(`<?php echo "second";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(echo); err != nil {
		t.Fatal(err)
	}
	if got := second.String(); got != "second" {
		t.Errorf("the second program wrote %q to the new writer, want %q", got, "second")
	}
	if strings.Contains(first.String(), "second") {
		t.Error("the second program wrote to the first program's writer")
	}
}

// TestRuntime_Exit covers the value `exit` ends a script with: an error, so it
// unwinds the interpreter, and an ending and no failure, which a host reads
// through IsExit without naming the type.
func TestRuntime_Exit(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	err := rt.Exit(3)
	if err == nil {
		t.Fatal("Exit answered no error, so nothing would unwind")
	}
	exit, ok := IsExit(err)
	if !ok {
		t.Fatalf("IsExit(%v) = false, want the exit recognised", err)
	}
	if exit.Code != 3 {
		t.Errorf("exit code = %d, want 3", exit.Code)
	}
	if got, want := err.Error(), "exit(3)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	// A zero is the ordinary ending and still an ExitError, so a host reading
	// the status does not have to tell the two apart.
	if exit, ok := IsExit(rt.Exit(0)); !ok || exit.Code != 0 {
		t.Error("exit(0) is not recognised as an exit")
	}
}

// TestScriptEnvironment covers what getenv() reads. A nil environment is the
// process one, as a CLI run has it; a host that configured one passes it
// instead, and a virtual host always does, so a site sees the variables it
// declared and none of the operator's. ScriptEnvironment holds the
// infrastructure names back either way.
func TestScriptEnvironment(t *testing.T) {
	configured := ScriptEnvironment([]string{
		"APP_ENV=production",
		"EMPTY=",
		InfrastructurePrefix + "SECRET=held back",
		"=no name",
		"HAS=an=equals",
	})

	if got := configured["APP_ENV"]; got != "production" {
		t.Errorf("APP_ENV = %q, want production", got)
	}
	if value, present := configured["EMPTY"]; !present || value != "" {
		t.Errorf("EMPTY = %q, %v, want an empty value that is present", value, present)
	}
	if got := configured["HAS"]; got != "an=equals" {
		t.Errorf("HAS = %q, want everything past the first equals", got)
	}
	if _, present := configured[InfrastructurePrefix+"SECRET"]; present {
		t.Error("an infrastructure variable reached the script environment")
	}
	if _, present := configured[""]; present {
		t.Error("an entry with no name reached the script environment")
	}

	// A configured environment is the whole of what a script sees, so a
	// variable the process carries and the host did not declare is absent.
	t.Setenv("PHPSCRIPT_TEST_ONLY_IN_PROCESS", "1")
	if _, present := configured["PHPSCRIPT_TEST_ONLY_IN_PROCESS"]; present {
		t.Error("a process variable reached a configured environment")
	}
	if _, present := ScriptEnvironment(nil)["PHPSCRIPT_TEST_ONLY_IN_PROCESS"]; !present {
		t.Error("a nil environment did not read the process one")
	}
}
