package runner

// Area is one named group of bindings: the installer a binding package
// contributes, under the namespace that documents it. The package folder maps
// to the area (stdlib/ps/core to Runtime\PS\Core) and a file inside the folder
// to a sub-area (stdlib/ps/core/strings.go to Runtime\PS\Core\Strings).
type Area struct {
	// Name is the namespace path, Runtime\PS\Core\Strings.
	Name string
	// Profile is the mask a host mounts the area under.
	Profile Profile
	// Install registers the area's functions, classes and constants.
	Install func(*Runtime)
}
