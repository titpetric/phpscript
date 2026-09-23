package config

import (
	"fmt"
	"os"
)

// Load returns the defaults from the embedded config/config.yml, with
// filename read over them, so that file only has to name what it changes. An
// empty filename is the defaults alone.
//
// It layers through OverlayBytes rather than unmarshalling here, so the
// operator's file gets the same reading a virtual host's phpscript.yml and a
// test suite's get: the same refusals for a block written the old way, and the
// same rule that naming a key with nothing under it means nothing rather than
// the inherited value.
//
// It lives here rather than in the command, because a reload reads the file
// again from inside the server and a command cannot be imported.
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
