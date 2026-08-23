package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner names an execution backend a fixture can be checked against.
type Runner string

const (
	// RunnerFlatstack executes the fixture through the flat bytecode runtime,
	// which falls back to the compatibility interpreter for unsupported syntax.
	RunnerFlatstack Runner = "flatstack"

	// RunnerRuntime executes the fixture through the default interpreter.
	RunnerRuntime Runner = "runtime"

	// RunnerPHP executes the fixture through the php binary found in PATH.
	RunnerPHP Runner = "php"
)

// Runners lists every backend the test matrix covers, in report order.
var Runners = []Runner{RunnerFlatstack, RunnerRuntime, RunnerPHP}

// ErrRunnerUnavailable reports that a runner cannot execute on this machine,
// which is a skip rather than a failure. The php runner returns it when no php
// binary is installed.
var ErrRunnerUnavailable = errors.New("runner unavailable")

// FixtureRunners opts a fixture out of individual runners. An absent field
// means the runner is used; the default runtime can not be opted out of
// because it defines the fixture's expected output.
type FixtureRunners struct {
	Flatstack *bool `yaml:"flatstack"`
	PHP       *bool `yaml:"php"`
}

// Runs reports whether the fixture is checked against r.
func (f *Fixture) Runs(r Runner) bool {
	switch r {
	case RunnerFlatstack:
		return f.Runner.Flatstack == nil || *f.Runner.Flatstack
	case RunnerPHP:
		return f.Runner.PHP == nil || *f.Runner.PHP
	default:
		return true
	}
}

// phpRun is the outcome of one php binary execution.
type phpRun struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// executePHP runs the fixture source through the php binary. The script is
// written into the fixture's include root so relative includes and __DIR__
// resolve the way they do for the two Go runtimes, which are rooted there.
func executePHP(ctx context.Context, f *Fixture) (phpRun, error) {
	binary, err := exec.LookPath("php")
	if err != nil {
		return phpRun{}, fmt.Errorf("php: %w", ErrRunnerUnavailable)
	}

	// php runs in the fixture's include root, which is the directory holding
	// it unless it named a root of its own. That is the same root both Go
	// runtimes resolve against, so a relative require reaches the same file on
	// all three.
	dir := f.RootDir()
	if f.Path == "" {
		dir = "."
	}
	script, cleanup, err := writePHPScript(dir, f)
	if err != nil {
		return phpRun{}, err
	}
	defer cleanup()

	// The fixture output is the contract, so error display is pinned to stderr
	// and log duplication is turned off; a php.ini differing per machine would
	// otherwise decide whether a warning lands in the compared stdout.
	args := []string{
		"-d", "display_errors=stderr",
		"-d", "log_errors=0",
		"-d", "error_reporting=E_ALL",
		"-d", "variables_order=EGPCS",
		script,
	}
	if f.Request.Args != nil {
		// parseArgs names the script itself first, the way $argv does.
		args = append(args, parseArgs(f.Request.Args, f.Path)[1:]...)
	}

	var stdout, stderr strings.Builder
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Stdin = f.stdin()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = phpEnv(f)

	runErr := cmd.Run()
	// php names the file it ran, which is the throwaway copy; the fixture is
	// what a reader of the failure has in front of them.
	absolute, absErr := filepath.Abs(filepath.Join(dir, script))
	if absErr != nil {
		absolute = filepath.Join(dir, script)
	}
	message := strings.NewReplacer(
		absolute, f.Path,
		filepath.Join(dir, script), f.Path,
		script, filepath.Base(f.Path),
	).Replace(stderr.String())
	out := phpRun{Stdout: stdout.String(), Stderr: message}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		out.ExitCode = exitErr.ExitCode()
	default:
		return out, fmt.Errorf("run php: %w", runErr)
	}
	return out, nil
}

// writePHPScript materializes the fixture source as a hidden file in dir. The
// name is derived from the fixture path so parallel fixtures never collide.
func writePHPScript(dir string, f *Fixture) (string, func(), error) {
	base := strings.TrimSuffix(filepath.Base(f.Path), ".phpt")
	if base == "" || base == "." {
		base = "fixture"
	}
	file, err := os.CreateTemp(dir, "."+base+".*.php")
	if err != nil {
		return "", nil, fmt.Errorf("write php script: %w", err)
	}
	name := file.Name()
	cleanup := func() { os.Remove(name) }
	if _, err := file.WriteString(f.PHP); err != nil {
		file.Close()
		cleanup()
		return "", nil, fmt.Errorf("write php script: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write php script: %w", err)
	}
	return filepath.Base(name), cleanup, nil
}

// phpEnv extends the process environment with the fixture request environment.
func phpEnv(f *Fixture) []string {
	if len(f.Request.Env) == 0 {
		return nil
	}
	env := os.Environ()
	for k, v := range f.Request.Env {
		env = append(env, k+"="+v)
	}
	return env
}

// phpFatal reports whether stderr carries an engine failure. An exit status is
// not enough on its own: a script chooses its own status through exit().
func phpFatal(stderr string) error {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "Fatal error") || strings.Contains(line, "Parse error") {
			return errors.New(strings.TrimSpace(line))
		}
	}
	return nil
}

// diagnostics summarizes the php process outcome for a failure report.
func (r phpRun) diagnostics() string {
	parts := make([]string, 0, 2)
	if r.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("php exit status %d", r.ExitCode))
	}
	if stderr := strings.TrimSpace(r.Stderr); stderr != "" {
		parts = append(parts, "php stderr: "+strings.ReplaceAll(stderr, "\n", "; "))
	}
	return strings.Join(parts, ", ")
}
