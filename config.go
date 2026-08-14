package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/config"
)

func loadConfig(filename string) (config.Config, error) {
	result := config.New()
	data := config.DefaultRuntimeConfig
	if filename != "" {
		var err error
		data, err = os.ReadFile(filename)
		if err != nil {
			return result, fmt.Errorf("read config %q: %w", filename, err)
		}
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		source := "embedded config"
		if filename != "" {
			source = filename
		}
		log.Printf("Could not parse %s: %v", source, err)
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
