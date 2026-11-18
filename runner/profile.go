package runner

// Profile is a bitmask naming the binding areas a host mounts. Each area
// reaching outside the runtime's sandbox gets a separate bit; a call site
// spells out every insecure area it enables. stdlib.Mount reads the mask.
type Profile uint64

// The binding areas, with one bit per area assigned by iota. A host spells out
// the set it mounts.
const (
	// ProfileSecure covers every binding confined to the runtime's sandbox
	// boundaries: the surface for a host running untrusted scripts.
	ProfileSecure Profile = 1 << iota

	// ProfileExec is process execution, contributed by stdlib/ps/pexec. A command
	// runs with the permissions of the user running the host, outside every
	// filesystem boundary.
	ProfileExec
)

// SetProfile records the area mask this runtime's bindings install under.
func (rt *Runtime) SetProfile(profile Profile) { rt.profile = profile }

// Profile reports the area mask this runtime's bindings install under.
func (rt *Runtime) Profile() Profile { return rt.profile }
