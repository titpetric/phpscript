package server_test

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/titpetric/phpscript/cmd/phpscript/server"
	"github.com/titpetric/phpscript/config"
)

// withPidFile returns a configuration naming path as its pidfile.
func withPidFile(path string) config.Config {
	appConfig := config.NewTestConfig()
	appConfig.Server.PidFile = path
	return appConfig
}

func TestSignal(t *testing.T) {
	t.Run("an unknown verb is refused", func(t *testing.T) {
		var errOut bytes.Buffer
		if err := server.Signal(withPidFile("/tmp/unused.pid"), "restart", &errOut); err == nil {
			t.Fatal("an unknown verb was accepted")
		}
		if !strings.Contains(errOut.String(), `unknown signal "restart", want reload`) {
			t.Errorf("stderr = %q", errOut.String())
		}
	})

	t.Run("no pid_file says so", func(t *testing.T) {
		var errOut bytes.Buffer
		if err := server.Signal(config.NewTestConfig(), server.SignalReload, &errOut); err == nil {
			t.Fatal("a configuration with no pidfile was accepted")
		}
		if !strings.Contains(errOut.String(), "sets no server.pid_file") {
			t.Errorf("stderr = %q", errOut.String())
		}
	})

	t.Run("a missing pidfile names the path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent.pid")

		var errOut bytes.Buffer
		if err := server.Signal(withPidFile(path), server.SignalReload, &errOut); err == nil {
			t.Fatal("a missing pidfile was accepted")
		}
		if !strings.Contains(errOut.String(), path) {
			t.Errorf("stderr = %q, want it to name %q", errOut.String(), path)
		}
	})

	t.Run("a pidfile that holds no process id is refused", func(t *testing.T) {
		for _, contents := range []string{"", "not a pid\n", "0\n", "-1\n"} {
			path := filepath.Join(t.TempDir(), "run.pid")
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}

			var errOut bytes.Buffer
			if err := server.Signal(withPidFile(path), server.SignalReload, &errOut); err == nil {
				t.Fatalf("a pidfile holding %q was accepted", contents)
			}
		}
	})

	// A pid that was real and is not. /bin/true is reaped by Run, so the
	// send reports ESRCH rather than racing a live process.
	t.Run("a stale pid reports the process is gone", func(t *testing.T) {
		cmd := exec.Command("/bin/true")
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(t.TempDir(), "run.pid")
		if err := os.WriteFile(path, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		var errOut bytes.Buffer
		if err := server.Signal(withPidFile(path), server.SignalReload, &errOut); err == nil {
			t.Fatal("a stale pid was accepted")
		}
		if !strings.Contains(errOut.String(), "stale") {
			t.Errorf("stderr = %q, want it to report the pidfile as stale", errOut.String())
		}
	})

	// The delivery itself, to this process. A handler is armed first:
	// SIGHUP with none takes its default disposition and kills the test
	// binary rather than failing a case.
	t.Run("sends SIGHUP to the recorded process", func(t *testing.T) {
		hangup := make(chan os.Signal, 1)
		signal.Notify(hangup, syscall.SIGHUP)
		t.Cleanup(func() { signal.Stop(hangup) })

		path := filepath.Join(t.TempDir(), "run.pid")
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		var errOut bytes.Buffer
		if err := server.Signal(withPidFile(path), server.SignalReload, &errOut); err != nil {
			t.Fatalf("signal failed: %s", errOut.String())
		}
		if errOut.Len() != 0 {
			t.Errorf("stderr = %q, want nothing on success", errOut.String())
		}

		select {
		case got := <-hangup:
			if got != syscall.SIGHUP {
				t.Fatalf("signal = %v, want SIGHUP", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no SIGHUP arrived")
		}
	})
}
