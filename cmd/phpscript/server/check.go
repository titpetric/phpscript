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
	if err == nil {
		err = appConfig.Validate(name, root)
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		fmt.Fprintf(errOut, "%s: failed\n", name)
		return ErrReported
	}

	fmt.Fprintf(out, "%s: ok\n", name)
	return nil
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
