package test_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/titpetric/phpscript/cmd/phpscript/test"
)

// coverProfileLine finds the profile entry for file starting at line.
func coverProfileLine(t *testing.T, profile, file string, line int) string {
	t.Helper()
	prefix := fmt.Sprintf("%s:%d.", file, line)
	for _, entry := range strings.Split(profile, "\n") {
		if strings.HasPrefix(entry, prefix) {
			return entry
		}
	}
	t.Fatalf("profile has no entry for %s:%d:\n%s", file, line, profile)
	return ""
}

// TestRunCommandCover covers --cover, --coverfile and --split end to end: the
// merged profile is the go cover format under mode count, an included file
// appears with its unexecuted statements at zero, and the split profile lands
// next to the fixture.
func TestRunCommandCover(t *testing.T) {
	tmp := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("suite/lib/functions.php", `<?php

function covered($n)
{
	return $n;
}

function uncovered()
{
	return "never";
}
`)
	write("suite/covered.phpt", `name: coverage fixture
description: exercises entrypoint and include coverage
---
<?php

include "lib/functions.php";

for ($i = 0; $i < 3; $i++) {
	echo covered($i);
}
---
012
`)

	t.Chdir(tmp)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	errRun := test.Run(context.Background(), []string{"suite"}, test.Options{Cover: true, Split: true})
	w.Close()
	os.Stdout = old
	var stdout bytes.Buffer
	_, _ = stdout.ReadFrom(r)
	if errRun != nil {
		t.Fatalf("run with --cover --split: %v\n%s", errRun, stdout.String())
	}
	if !strings.Contains(stdout.String(), "% of statements") {
		t.Errorf("stdout is missing the coverage summary:\n%s", stdout.String())
	}

	data, err := os.ReadFile("phpscript.cov")
	if err != nil {
		t.Fatalf("read merged profile: %v", err)
	}
	profile := string(data)
	if !strings.HasPrefix(profile, "mode: count\n") {
		t.Fatalf("profile does not open with mode count:\n%s", profile)
	}

	// The echo line of the .phpt runs once per loop iteration; lines count from
	// the start of the PHP section, and the columns span the statement text.
	if got := coverProfileLine(t, profile, "suite/covered.phpt", 6); got != "suite/covered.phpt:6.2,6.19 1 3" {
		t.Errorf("entrypoint echo entry = %q", got)
	}
	// The include registers whole: the called function's body counts, and the
	// never-called one is present at zero.
	if got := coverProfileLine(t, profile, "suite/lib/functions.php", 5); !strings.HasSuffix(got, " 1 3") {
		t.Errorf("covered() body entry = %q, want count 3", got)
	}
	if got := coverProfileLine(t, profile, "suite/lib/functions.php", 10); !strings.HasSuffix(got, " 1 0") {
		t.Errorf("uncovered() body entry = %q, want count 0", got)
	}

	split, err := os.ReadFile("suite/covered.cov")
	if err != nil {
		t.Fatalf("read split profile: %v", err)
	}
	if !strings.HasPrefix(string(split), "mode: count\n") || !strings.Contains(string(split), "suite/lib/functions.php:") {
		t.Errorf("split profile is missing its content:\n%s", split)
	}
}

// TestRunCommandCoverConflicts holds the flag contract: coverage counts one run
// per fixture, so the benchmark loops are refused.
func TestRunCommandCoverConflicts(t *testing.T) {
	if err := test.Run(context.Background(), nil, test.Options{Cover: true, Count: 5}); err == nil || !strings.Contains(err.Error(), "cover") {
		t.Errorf("cover with count: err = %v, want a cover conflict", err)
	}
	if err := test.Run(context.Background(), nil, test.Options{CoverFile: "x.cov", Time: time.Second}); err == nil || !strings.Contains(err.Error(), "cover") {
		t.Errorf("coverfile with time: err = %v, want a cover conflict", err)
	}
}
