package config

import (
	yaml "github.com/goccy/go-yaml"
)

// Unmarshal reads a YAML document into target.
//
// Every configuration this tree reads goes through here, including the ones a
// test writes inline. The field tags on the structs of this package are one
// library's dialect, and a second decoder reading them would have to agree
// with it on what a bare 0640 and a 2M mean; one door is what keeps that from
// being two settings.
func Unmarshal(data []byte, target any) error {
	return yaml.Unmarshal(data, target)
}
