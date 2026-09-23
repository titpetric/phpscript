package main

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/config"
)

// loadConfig delegates to config.Load. The command keeps the name it has
// always called, and the reading itself belongs to the package that owns the
// model, because a reload reads the file again from inside the server.
func loadConfig(filename string) (config.Config, error) {
	return config.Load(filename)
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
