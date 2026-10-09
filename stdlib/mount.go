package stdlib

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/runner"
)

// Profile selects which contributed binding areas Mount installs. Every area
// that reaches outside the sandbox is its own bit, so a host's call site names
// each insecure area it enables: Mount(rt, stdlib.Secure|stdlib.Exec).
type Profile = runner.Profile

const (
	// Secure is every binding confined to the runtime's sandbox boundaries.
	Secure = runner.ProfileSecure
	// Exec is process execution, the stdlib/pexec package: exec, system,
	// passthru, shell_exec, the shell quoting pair and the pid pair.
	Exec = runner.ProfileExec

	// Default is the profile for a host running untrusted scripts: the secure
	// surface and nothing else.
	Default = Secure
)

// All is every area. AreaNames are the --stdlib spellings.
const All = ^Profile(0)

const AreaNames = "all, secure, exec"

var areas = map[string]Profile{"all": All, "secure": Secure, "exec": Exec}

// profile is what Register mounts, for the whole process: --stdlib limits the
// runtime an operator started, and no single host inside it.
var profile = All

// ParseProfile reads a --stdlib value: area names separated by commas, empty
// meaning All. An unknown name is refused, because a typo that fell back to All
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

// Mount is Register narrowed to a profile: only the binding areas the bitmask
// names are installed, so a name outside them is undefined in the runtime
// and never refused, unreachable through a call, a callable or either
// engine. The extra bindings are the caller's own and are installed verbatim.
func Mount(rt *runner.Runtime, profile Profile, bindings ...func(*runner.Runtime)) {
	rt.SetProfile(profile)
	registerExceptions(rt)

	for _, register := range runner.BindingsFor(profile) {
		register(rt)
	}
	for _, register := range bindings {
		register(rt)
	}

	// How to do this again, for a runtime forked off this one. A binding is a
	// closure over the runtime it was registered on, so a fork has to install
	// its own and inherits none of these, under the same profile.
	rt.SetPreparer(func(child *runner.Runtime) { Mount(child, profile, bindings...) })
}
