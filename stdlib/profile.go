package stdlib

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// Profile selects which contributed binding areas Mount installs. Every area
// that reaches outside the sandbox gets its bit. A host's call site names each
// insecure area it enables: Mount(rt, stdlib.Secure|stdlib.Exec).
type Profile = runner.Profile

// Secure holds the bindings confined to the runtime's sandbox boundaries.
const (
	Secure = runner.ProfileSecure
	// Exec is process execution, the stdlib/ps/pexec package: exec, system,
	// passthru, shell_exec, the shell quoting pair and the pid pair.
	Exec = runner.ProfileExec

	// Default is the profile for a host running untrusted scripts, secure alone.
	Default = Secure
)

// All holds every area. AreaNames are the --stdlib spellings.
const All = ^Profile(0)

const AreaNames = "all, secure, exec"

var areas = map[string]Profile{"all": All, "secure": Secure, "exec": Exec}

// profile holds the areas Register mounts for every runtime in the process.
var profile = All

// ParseProfile reads a --stdlib value: area names separated by commas, empty
// meaning All. It refuses an unknown name, since a typo falling back to All
// would read as a narrowed runtime and serve an unnarrowed one.
func ParseProfile(value string) (Profile, error) {
	if strings.TrimSpace(value) == "" {
		return All, nil
	}
	var out Profile
	for name := range strings.SplitSeq(value, ",") {
		area, ok := areas[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return 0, fmt.Errorf("unknown area %q, want %s", name, AreaNames)
		}
		out |= area
	}
	return out, nil
}

// SetProfile narrows what Register mounts, for every runtime built after it.
func SetProfile(p Profile) { profile = p }
