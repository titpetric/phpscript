package config

import (
	"fmt"
	"os"
)

// Load reads filename over the embedded defaults, through the same overlay a
// virtual host and a test suite get. An empty filename is the defaults alone.
func Load(filename string) (Config, error) {
	base := New()
	if filename == "" {
		return base, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return base, fmt.Errorf("read config %q: %w", filename, err)
	}

	// Nothing is forbidden to the operator: server and virtualhost are theirs
	// to set, which is what makes them forbidden to a site.
	result, _, err := OverlayBytes(base, filename, data, nil, "")
	if err != nil {
		return base, err
	}
	if err := result.Mail.Validate(filename); err != nil {
		return base, err
	}
	return result, nil
}
