package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/titpetric/phpscript/config"
)

// SignalReload is the one verb -s takes. nginx also has stop, quit and
// reopen; the first two are a kill with extra steps, and there are no log
// files to reopen.
const SignalReload = "reload"

// Signal sends verb to the server recorded in server.pid_file.
func Signal(appConfig config.Config, verb string, errOut io.Writer) error {
	if verb != SignalReload {
		fmt.Fprintf(errOut, "-s: unknown signal %q, want %s\n", verb, SignalReload)
		return ErrReported
	}

	path := appConfig.Server.PidFile
	if path == "" {
		fmt.Fprintln(errOut, "-s: the configuration sets no server.pid_file, so there is no running server to signal")
		return ErrReported
	}

	pid, err := readPid(path)
	if err != nil {
		fmt.Fprintf(errOut, "-s: %v\n", err)
		return ErrReported
	}

	// FindProcess never fails on unix. The send is what reports a process
	// that is gone, so probing first would add a race without removing one.
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(errOut, "-s: %v\n", err)
		return ErrReported
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			fmt.Fprintf(errOut, "-s: process %d is not running, so the pidfile %q is stale\n", pid, path)
			return ErrReported
		}
		fmt.Fprintf(errOut, "-s: signal process %d: %v\n", pid, err)
		return ErrReported
	}

	// Nothing on success, as nginx prints nothing: a signal carries no way
	// to answer back, and what the reload did is in the server's log.
	return nil
}

// readPid returns the process id recorded in the file at path.
func readPid(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, fmt.Errorf("no pidfile at %q, so no server is running under this configuration", path)
		}
		return 0, fmt.Errorf("read pidfile: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("pidfile %q does not hold a process id", path)
	}
	return pid, nil
}
