package server

import (
	"errors"
	"fmt"
	"io"

	"github.com/titpetric/phpscript/config"
)

// ErrReported ends the process non-zero without a second report. `-t` and
// `-s` print what failed in their own words, and main would otherwise print
// it again as an unexpected error, which a failed configuration test is not.
var ErrReported = errors.New("reported")

// Check reports whatever would stop a server from starting under appConfig,
// and writes a verdict naming the file it read.
//
// It is `phpscript -t`, and it answers the question the server answers on the
// way up without doing any of what the server does: no socket is bound, no
// @startup job runs, nothing is dialled, and no trace storage is created. A
// command that tests a configuration must not leave anything behind.
//
// root is the application root a single-root server would serve. It is an
// error beside a configuration that lists virtual hosts, which name their own.
//
// It reads filename itself rather than being handed a configuration, because
// a file that does not parse is one of the things it reports, and the caller
// failing on that first would report it in the wrong voice.
func Check(filename, root string, out, errOut io.Writer) error {
	name := configName(filename)

	appConfig, err := config.Load(filename)
	if err != nil {
		fmt.Fprintln(errOut, err)
		fmt.Fprintf(errOut, "%s: failed\n", name)
		return ErrReported
	}

	if err := check(appConfig, name, root); err != nil {
		fmt.Fprintln(errOut, err)
		fmt.Fprintf(errOut, "%s: failed\n", name)
		return ErrReported
	}

	fmt.Fprintf(out, "%s: ok\n", name)
	return nil
}

// check stops at the first failure. A configuration is read top to bottom,
// and the second complaint is usually a consequence of the first.
func check(appConfig config.Config, name, root string) error {
	// The server never reaches the test block, but -t is a question about
	// the file rather than about one command, so a test.cache nothing
	// accepts fails it here.
	if err := appConfig.Test.Validate(name); err != nil {
		return err
	}
	if err := appConfig.Telemetry.Validate(); err != nil {
		return err
	}

	if len(appConfig.VirtualHost) > 0 {
		if root != "" {
			return errRootWithVirtualHosts(root)
		}
		return appConfig.ValidateVirtualHosts()
	}

	if root == "" {
		root = "."
	}
	return appConfig.ValidateRoot(root)
}

// errRootWithVirtualHosts is the refusal a root on the command line gets
// beside a virtual host list. Check and Run both give it, so it is spelled
// once.
func errRootWithVirtualHosts(root string) error {
	return fmt.Errorf("server: the configuration lists virtual hosts, which name their own roots; %q on the command line has no virtual host to belong to", root)
}

// configName is what a verdict and a validation error name the configuration
// as. A run given no -f read the defaults compiled into the binary.
func configName(filename string) string {
	if filename == "" {
		return "built-in defaults"
	}
	return filename
}

// rootArg is the application root named on the command line, or "".
func rootArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
