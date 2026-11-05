package pexec_test

import (
	"strings"
	"testing"
)

// TestExecReturnsTheLastLine covers what exec() answers with and what it
// collects: the return is the last line of stdout and exec() appends to the
// $output array the caller passed. Every expectation here came from running the
// same call through php.
func TestExecReturnsTheLastLine(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$lines = [];
$code = -1;
$last = exec('printf "a\nb\n"', $lines, $code);
echo "[", $last, "][", count($lines), "][", $lines[0], ",", $lines[1], "][", $code, "]";`)
	if want := "[b][2][a,b][0]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestExecAppendsToTheArray covers the by-reference half: PHP appends to an
// array that already holds something and a second call into the same variable
// adds to it and never replaces it.
func TestExecAppendsToTheArray(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$lines = ["kept"];
exec('printf "a\n"', $lines);
echo implode(",", $lines);`)
	if want := "kept,a"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestExecReportsTheExitStatus covers a command that ran and failed: it has a
// status to report and $result_code carries it and the return is the empty
// string a command printing nothing answers with. The status is a separate
// output because a caller reading the return alone cannot tell a failure from
// a command that printed nothing.
func TestExecReportsTheExitStatus(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$code = -1;
$last = exec('exit 3', $lines, $code);
echo "[", $last, "][", $code, "]";`)
	if want := "[][3]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestExecTrimsEachLine covers the splitting rule: trailing whitespace comes
// off every line, and a command that printed nothing reports no lines at all
// and never one empty one and count($output) is 0 and not 1.
func TestExecTrimsEachLine(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$padded = [];
exec('printf "a  \t\nb\n"', $padded);
$empty = [];
exec('true', $empty);
echo "[", implode("|", $padded), "][", count($empty), "]";`)
	if want := "[a|b][0]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestSystemWritesAsItArrives covers what separates system() from exec(): the
// output reaches the page while the command runs, and the return is still the
// last line. A command that takes a while is one whose output a page wants
// before it finishes.
func TestSystemWritesAsItArrives(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$code = -1;
$last = system('printf "a\nb\n"', $code);
echo "[", $last, "][", $code, "]";`)
	if want := "a\nb\n[b][0]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestPassthruWritesBytesThrough covers the one to reach for when the output
// is binary: the bytes arrive untouched, with no line splitting to trim them,
// and the return is null and the status is the only thing to read. The same
// bytes through exec() lose the trailing space exec() strips per line.
func TestPassthruWritesBytesThrough(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$code = -1;
$value = passthru('printf "a \n"', $code);
echo "[", ($value === null ? "null" : "value"), "][", $code, "]";
$lines = [];
exec('printf "a \n"', $lines);
echo "[", $lines[0], "]";`)
	if want := "a \n[null][0][a]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestShellExecReturnsEverything covers the whole of stdout, newline included,
// and the null a silent command answers with. The null is PHP's and a caller
// checking for failure has to read the string and never the return.
func TestShellExecReturnsEverything(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
$text = shell_exec('printf "x\ny\n"');
$quiet = shell_exec('true');
echo "[", $text, "][", ($quiet === null ? "null" : "value"), "]";`)
	if want := "[x\ny\n][null]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestCommandsRefuseAnEmptyCommandLine covers the guard in front of every one
// of them: an empty command names nothing to run and none of them starts a
// shell. Each reports what it reports for a command it could not start, false
// for the three and null for shell_exec. php 8 raises ValueError here and a
// script written against php sees a return where it would see a throw.
func TestCommandsRefuseAnEmptyCommandLine(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php
echo "[", (exec("") === false ? "false" : "other"), "]";
echo "[", (system("") === false ? "false" : "other"), "]";
echo "[", (passthru("") === false ? "false" : "other"), "]";
echo "[", (shell_exec("") === null ? "null" : "other"), "]";`)
	if want := "[false][false][false][null]"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestCommandsRunThroughTheShell covers the one decision these four share:
// each command line reaches `sh -c` and a pipeline, a redirect and a glob are
// the shell's to read and never parsed here.
func TestCommandsRunThroughTheShell(t *testing.T) {
	t.Parallel()
	out := run(t, "", `<?php echo shell_exec('printf "b\na\n" | sort | tr -d "\n"');`)
	if want := "ab"; strings.TrimSpace(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
