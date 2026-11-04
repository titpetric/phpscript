package toolchain_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/toolchain"
)

func script(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func names(tools []toolchain.Tool) string {
	var out []string
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return strings.Join(out, ",")
}

func TestBinary(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"/usr/bin/phpscript": "phpscript", "phpx": "phpx", "./bin/phpx.exe": "phpx"} {
		if got := toolchain.Binary(in); got != want {
			t.Errorf("Binary(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDiscoverPrefixIsolation pins the rule: a binary sees only the tools
// carrying its own name as the prefix.
func TestDiscoverPrefixIsolation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script(t, dir, "phpscript-gen", "echo gen")
	script(t, dir, "phpscript-test", "echo test")
	script(t, dir, "phpx-test", "echo xtest")
	script(t, dir, "phpx-cover", "echo cover")
	script(t, dir, "phpxy-nope", "echo nope")
	script(t, dir, "phpscript-", "echo empty")
	if err := os.WriteFile(filepath.Join(dir, "phpscript-data"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := names(toolchain.Discover("phpscript", dir)); got != "gen,test" {
		t.Errorf("phpscript tools = %q, want gen,test", got)
	}
	if got := names(toolchain.Discover("phpx", dir)); got != "cover,test" {
		t.Errorf("phpx tools = %q, want cover,test", got)
	}
	if got := names(toolchain.Discover("php", dir)); got != "" {
		t.Errorf("php tools = %q, want none", got)
	}
}

func TestDiscoverFirstOnPathWinsAndMemoizes(t *testing.T) {
	t.Parallel()
	first, second := t.TempDir(), t.TempDir()
	script(t, first, "phpscript-gen", "echo one")
	script(t, second, "phpscript-gen", "echo two")
	path := first + string(os.PathListSeparator) + second

	tools := toolchain.Discover("phpscript", path)
	if len(tools) != 1 || filepath.Dir(tools[0].Path) != first {
		t.Fatalf("tools = %+v, want the first directory's gen", tools)
	}
	// Memoized: a tool added later stays unseen for the same PATH.
	script(t, first, "phpscript-late", "echo late")
	if got := names(toolchain.Discover("phpscript", path)); got != "gen" {
		t.Errorf("after adding a tool, got %q, want the memoized gen", got)
	}
}

func TestHelpSanitizesAndTitle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script(t, dir, "phpscript-gen", `echo "Generate things"; echo "Usage: phpscript-gen [args]"`)
	script(t, dir, "phpscript-quiet", "true")
	tools := toolchain.Discover("phpscript", dir)
	gen, ok := toolchain.Find(tools, "gen")
	if !ok {
		t.Fatal("gen not found")
	}
	help := toolchain.Help("phpscript", gen, tools)
	if !strings.Contains(help, "Usage: phpscript gen [args]") || strings.Contains(help, "phpscript-gen") {
		t.Errorf("help not sanitized: %q", help)
	}
	if got := toolchain.Title("phpscript", gen, tools); got != "Generate things" {
		t.Errorf("Title = %q", got)
	}
	quiet, _ := toolchain.Find(tools, "quiet")
	if got := toolchain.Title("phpscript", quiet, tools); !strings.Contains(got, "phpscript-quiet") {
		t.Errorf("fallback Title = %q", got)
	}
	if _, ok := toolchain.Find(tools, "none"); ok {
		t.Error("Find invented a tool")
	}
}

func TestSanitize(t *testing.T) {
	t.Parallel()
	tools := []toolchain.Tool{{Name: "gen"}, {Name: "gen-x"}}
	got := toolchain.Sanitize("phpscript-gen and phpscript-gen-x, phpx-gen", "phpscript", tools)
	if want := "phpscript gen and phpscript gen-x, phpx-gen"; got != want {
		t.Errorf("Sanitize = %q, want %q", got, want)
	}
}

func TestExecExitCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script(t, dir, "phpscript-fail", "exit 3")
	script(t, dir, "phpscript-ok", "exit 0")
	tools := toolchain.Discover("phpscript", dir)
	fail, _ := toolchain.Find(tools, "fail")
	ok, _ := toolchain.Find(tools, "ok")
	if got := toolchain.Exec(fail, nil); got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	if got := toolchain.Exec(ok, nil); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
	if got := toolchain.Exec(toolchain.Tool{Path: filepath.Join(dir, "missing")}, nil); got != 127 {
		t.Errorf("missing exit = %d, want 127", got)
	}
}
