// Package pexec exposes process execution to PHP: exec runs a command through
// the shell the way PHP's does, escapeshellarg quotes an argument for it, and
// posix_getpid names the running process.
//
// exec's $output works because arrays are shared: the binding appends into the
// array the caller passed, which is the same observable result PHP reaches by
// reference. $result_code has no such route — an integer cannot be written
// back — so it is accepted and ignored.
package pexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the process bindings (exec, escapeshellarg, posix_getpid)
// to stdlib.Register.
func init() {
	runner.RegisterBinding(Register)
}

// Register installs symbols into the runtime.
func Register(rt *runner.Runtime) {
	// exec runs $command through the shell and returns the last line of its stdout, appending each output line to $output when an array is passed; $result_code is accepted and ignored, because an integer cannot be written back.
	rt.RegisterFunc("exec", phpExec)
	// escapeshellarg returns $arg single-quoted for the shell, with embedded single quotes escaped.
	rt.RegisterFunc("escapeshellarg", func(arg string) string {
		return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	})
	// posix_getpid returns the process id of the running interpreter.
	rt.RegisterFunc("posix_getpid", func() int64 { return int64(os.Getpid()) })
}

// phpExec is the exec binding. Stdout is captured line by line; stderr passes
// through to the host's, as it does under PHP. A command that starts and
// exits non-zero still yields its output — the status would have gone into
// $result_code — while a command that cannot start at all returns false.
func phpExec(ctx context.Context, command string, args ...any) (any, error) {
	if command == "" {
		return nil, errors.New("exec(): Argument #1 ($command) cannot be empty")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return false, nil
		}
	}

	lines := strings.Split(strings.TrimRight(string(stdout), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	if len(args) > 0 {
		if output, ok := args[0].(*model.Array); ok {
			for _, line := range lines {
				output.Append(strings.TrimRight(line, " \t\r\v\f"))
			}
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.TrimRight(lines[len(lines)-1], " \t\r\v\f"), nil
}
