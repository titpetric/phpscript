package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/titpetric/phpscript/config"
)

// loadConfig returns the defaults from the embedded config/config.yml, with
// the file passed on the command line read over them, so that file only has to
// name what it changes.
//
// It layers through config.OverlayBytes rather than unmarshalling here, so the
// operator's file gets the same reading a virtual host's phpscript.yml and a
// test suite's get: the same refusals for a block written the old way, and the
// same rule that naming a key with nothing under it means nothing rather than
// the inherited value.
func loadConfig(filename string) (config.Config, error) {
	base := config.New()
	if filename == "" {
		return base, nil
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return base, fmt.Errorf("read config %q: %w", filename, err)
	}

	// Nothing is forbidden to the operator: server and virtualhost are theirs
	// to set, which is what makes them forbidden to a site.
	// The error is returned rather than also logged. It already names the file
	// and the key, and the command reports what it is handed, so logging here
	// printed every configuration error twice.
	result, _, err := config.OverlayBytes(base, filename, data, nil, "")
	if err != nil {
		return base, err
	}
	if err := result.Mail.Validate(filename); err != nil {
		return base, err
	}
	return result, nil
}

func parseConfigFile(args []string) (string, []string, error) {
	var filename string
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-f" || args[i] == "--file":
			if i+1 == len(args) {
				return "", nil, fmt.Errorf("%s requires a configuration file", args[i])
			}
			i++
			filename = args[i]
		case strings.HasPrefix(args[i], "--file="):
			filename = strings.TrimPrefix(args[i], "--file=")
			if filename == "" {
				return "", nil, fmt.Errorf("--file requires a configuration file")
			}
		default:
			remaining = append(remaining, args[i])
		}
	}
	return filename, remaining, nil
}
