package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
)

// SignalReload is the one verb `-s` takes.
//
// nginx also has stop, quit and reopen. stop and quit are a kill with extra
// steps, and there are no log files to reopen, so each would be a second
// lifecycle to document and test for nothing.
const SignalReload = "reload"

// Signal sends verb to the server named by server.pid_file.
//
// It reads the pidfile from the same configuration the server was started
// with, so `-s` needs the `-f` and the `-w` the server got: a relative
// pid_file resolves against the working directory.
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

	pid, err := platform.ReadPidFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(errOut, "-s: no pidfile at %q, so no server is running under this configuration\n", path)
			return ErrReported
		}
		fmt.Fprintf(errOut, "-s: read pidfile: %v\n", err)
		return ErrReported
	}

	// FindProcess never fails on unix; the send is what reports a process
	// that is gone. Probing first with signal 0 would add a race without
	// removing one, since the process can exit between the probe and the
	// send either way.
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

	// Nothing on success, as nginx gives nothing. Whether the reload was
	// applied is in the server's log: a signal carries no way to answer
	// back, which is what -t and Manager.Check are for.
	return nil
}
