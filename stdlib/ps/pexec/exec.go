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

// registerExec installs exec, system, passthru and shell_exec, which differ in output handling.
func registerExec(rt *runner.Runtime, r root) {
	// exec runs $command through the shell and returns the last line of its
	// stdout, appending each output line to $output when the caller passes an
	// array, and writing the exit status to $result_code.
	rt.RegisterFunc("exec", r.phpExec)
	// system runs $command through the shell, writing its output as it
	// arrives, and returns the last line; it writes the exit status to
	// $result_code.
	rt.RegisterFunc("system", r.phpSystem)
	// passthru runs $command through the shell and writes its output through
	// untouched, for a command whose output is binary; it returns null and
	// writes the exit status to $result_code.
	rt.RegisterFunc("passthru", r.phpPassthru)
	// shell_exec runs $command through the shell and returns its stdout
	// whole, or null when the command produced none.
	rt.RegisterFunc("shell_exec", r.phpShellExec)
}

// command builds the process, starting it in the script's host working directory.
func (r root) command(ctx context.Context, cmdline string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdline)
	if dir := r.hostWorkDir(); dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

// hostWorkDir returns the bound directory plus wherever chdir has moved.
func (r root) hostWorkDir() string {
	work := r.rt.WorkDir()
	if work == "" || work == "." {
		return r.dir
	}
	return filepath.Join(r.dir, filepath.FromSlash(work))
}

// runStatus runs cmd and reports its status, false when it could not start.
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

// writeRef writes v through set, tolerating an omitted argument's nil setter.
func writeRef(set func(any), v any) {
	if set != nil {
		set(v)
	}
}

// outputLines splits stdout into trimmed lines, none for empty output.
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

// phpExec captures stdout, fills output by appending and returns the last line.
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

// phpSystem writes output as it arrives and returns the last line.
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

// phpPassthru writes output through byte for byte and returns null.
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

// phpShellExec returns the command's whole stdout, null when it wrote nothing.
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

// lastLine keeps the final line of a stream without holding the rest.
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

// String returns the stream's last line, whether it ended with a newline.
func (l *lastLine) String() string {
	if l.current.Len() > 0 {
		return strings.TrimRight(l.current.String(), " \t\r\v\f")
	}
	return strings.TrimRight(l.previous, " \t\r\v\f")
}
