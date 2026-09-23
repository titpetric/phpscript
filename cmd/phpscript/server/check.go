package server

import (
	"errors"
	"fmt"
	"io"

	"github.com/titpetric/phpscript/config"
)

// ErrReported ends the process non-zero without a second report: -t and -s
// have already printed what failed in their own words.
var ErrReported = errors.New("reported")

// Check is `phpscript -t`: it reports whatever would stop a server starting
// under filename, writes a verdict, and starts nothing.
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

// check stops at the first failure: the second complaint is usually a
// consequence of the first.
func check(appConfig config.Config, name, root string) error {
	// The server never reaches the test block, but -t is a question about
	// the file rather than about one command.
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

// errRootWithVirtualHosts is the refusal Check and Run both give a root named
// beside a virtual host list.
func errRootWithVirtualHosts(root string) error {
	return fmt.Errorf("server: the configuration lists virtual hosts, which name their own roots; %q on the command line has no virtual host to belong to", root)
}

// configName is what a verdict names the configuration as.
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
