package pexec

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// registerExec installs the four functions that run a command. They differ only
// in what they do with the output: exec collects it, system and passthru write
// it out as it arrives, and shell_exec returns all of it.
func registerExec(rt *runner.Runtime, r root) {
	// exec runs $command through the shell and returns the last line of its stdout, appending each output line to $output when an array is passed and writing the exit status to $result_code.
	rt.RegisterFunc("exec", r.phpExec)
	// system runs $command through the shell, writing its output as it arrives, and returns the last line; the exit status is written to $result_code.
	rt.RegisterFunc("system", r.phpSystem)
	// passthru runs $command through the shell and writes its output through untouched, for a command whose output is binary; it returns null, and the exit status is written to $result_code.
	rt.RegisterFunc("passthru", r.phpPassthru)
	// shell_exec runs $command through the shell and returns all of its stdout, or null when the command produced none.
	rt.RegisterFunc("shell_exec", r.phpShellExec)
}

// command builds the process for one of these calls.
//
// Dir is the working directory the script is in, resolved onto the host. This
// is the one place the fs.FS sandbox is left behind on purpose: a command is a
// process, it reads and writes with the permissions of the user running the
// server, and writable_paths cannot reach it. What the runtime can say is where
// it starts, and it says the same thing getcwd() does.
//
// A host that bound no directory leaves Dir empty, which is the process working
// directory — the behaviour every command had before this.
func (r root) command(ctx context.Context, cmdline string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdline)
	if dir := r.hostWorkDir(); dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

// hostWorkDir is the working directory as a path on the host: the directory the
// shims are bound to, plus wherever chdir has moved inside it.
func (r root) hostWorkDir() string {
	work := r.rt.WorkDir()
	if work == "" || work == "." {
		return r.dir
	}
	return filepath.Join(r.dir, filepath.FromSlash(work))
}

// runStatus runs cmd and reports its exit status. A command that ran and failed
// has a status to report; one that could not start at all has none, which is
// what the second return says.
func runStatus(cmd *exec.Cmd) (status int64, started bool) {
	err := cmd.Run()
	if err == nil {
		return 0, true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return int64(exitErr.ExitCode()), true
	}
	return 0, false
}

// writeRef writes an output parameter, tolerating the caller who left it out:
// an omitted by-reference argument arrives as a nil setter.
func writeRef(set func(any), v any) {
	if set != nil {
		set(v)
	}
}

// outputLines splits captured stdout the way PHP's exec does: trailing
// whitespace comes off each line, and a command that printed nothing yields no
// lines rather than one empty one.
func outputLines(stdout []byte) []string {
	trimmed := strings.TrimRight(string(stdout), "\n")
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r\v\f")
	}
	return lines
}

// phpExec captures stdout and hands back its last line.
//
// $output is filled by appending into the array the caller passed, because
// arrays are shared: PHP reaches the same observable result by reference, and
// appending is what PHP does to an array that already holds something.
// $result_code is a by-reference setter, since an integer has no other route
// back.
func (r root) phpExec(ctx context.Context, command string, output any, resultCode func(any)) any {
	if command == "" {
		return false
	}
	cmd := r.command(ctx, command)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.Output()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return false
	}
	status := int64(0)
	if exitErr != nil {
		status = int64(exitErr.ExitCode())
	}
	writeRef(resultCode, status)

	lines := outputLines(stdout)
	if collected, ok := output.(*model.Array); ok {
		for _, line := range lines {
			collected.Append(line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

// phpSystem writes the output as it arrives and returns the last line. The
// stream is teed rather than buffered and echoed afterwards, because a command
// that takes a while is one whose output a page wants while it runs.
func (r root) phpSystem(ctx context.Context, command string, resultCode func(any)) any {
	if command == "" {
		return false
	}
	var tail lastLine
	cmd := r.command(ctx, command)
	cmd.Stdout = io.MultiWriter(r.rt.Output(), &tail)
	cmd.Stderr = os.Stderr

	status, started := runStatus(cmd)
	if !started {
		return false
	}
	writeRef(resultCode, status)
	return tail.String()
}

// phpPassthru writes the output through byte for byte and returns null. It is
// the one to use for a command whose output is binary, where a line-splitting
// pass would corrupt it.
func (r root) phpPassthru(ctx context.Context, command string, resultCode func(any)) any {
	if command == "" {
		return false
	}
	cmd := r.command(ctx, command)
	cmd.Stdout = r.rt.Output()
	cmd.Stderr = os.Stderr

	status, started := runStatus(cmd)
	if !started {
		return false
	}
	writeRef(resultCode, status)
	return nil
}

// phpShellExec returns everything the command wrote to stdout, and null when it
// wrote nothing. The null is PHP's, and it is why a caller checking for failure
// has to check the string rather than the return.
func (r root) phpShellExec(ctx context.Context, command string) any {
	if command == "" {
		return nil
	}
	cmd := r.command(ctx, command)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.Output()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return nil
	}
	if len(stdout) == 0 {
		return nil
	}
	return string(stdout)
}

// lastLine keeps the final line of a stream without holding the rest of it. It
// is what lets system() answer with its last line while the output has already
// gone to the page.
type lastLine struct {
	current  strings.Builder
	previous string
}

func (l *lastLine) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			l.previous = l.current.String()
			l.current.Reset()
			continue
		}
		l.current.WriteByte(b)
	}
	return len(p), nil
}

// String is the last line the stream carried, whether or not it ended with a
// newline.
func (l *lastLine) String() string {
	if l.current.Len() > 0 {
		return strings.TrimRight(l.current.String(), " \t\r\v\f")
	}
	return strings.TrimRight(l.previous, " \t\r\v\f")
}
