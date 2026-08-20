package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/config"
)

// loadConfig returns the defaults from the embedded config/config.yml, with
// the file passed on the command line read over them, so that file only has to
// name what it changes.
func loadConfig(filename string) (config.Config, error) {
	result := config.New()
	if filename == "" {
		return result, nil
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return result, fmt.Errorf("read config %q: %w", filename, err)
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		log.Printf("Could not parse %s: %v", filename, err)
		return result, err
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
