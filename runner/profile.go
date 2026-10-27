package runner

// Profile is a bitmask naming the binding areas a host mounts. Each area that
// reaches outside the runtime's sandbox is its own bit and a call site spells
// out every insecure area it enables; stdlib.Mount reads one.
type Profile uint64

// The binding areas. One bit per area, assigned by iota and a host spells out
// what it mounts.
const (
	// ProfileSecure is every binding confined to the runtime's own sandbox
	// boundaries: the surface for a host running untrusted scripts.
	ProfileSecure Profile = 1 << iota

	// ProfileExec is process execution, contributed by stdlib/pexec. A command
	// runs with the permissions of the user running the host, outside every
	// filesystem boundary.
	ProfileExec
)

// SetProfile records the area mask this runtime's bindings install under.
func (rt *Runtime) SetProfile(profile Profile) { rt.profile = profile }

// Profile reports the area mask this runtime's bindings install under.
func (rt *Runtime) Profile() Profile { return rt.profile }
