package config

import (
	yaml "github.com/goccy/go-yaml"
)

// Unmarshal reads a YAML document into target.
func Unmarshal(data []byte, target any) error {
	return yaml.Unmarshal(data, target)
}
