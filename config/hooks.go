package config

// Hooks are the PHP files a test session runs around the fixtures of one suite
// root. Both paths resolve against the root and run once per session,
// not once per fixture.
//
// They are configuration, not annotations; a test session is not a server.
// `phpscript server` scans @startup, @route and @schedule out of a source tree
// and runs them per virtual host or per application root, which the operator's
// configuration picks. A fixture run has no such scope. A comment in a file
// some walk reaches binds no suite.
type Hooks struct {
	// Setup runs once before any fixture below this suite root: the schema its
	// fixtures query and the rows they read. A failure fails the run before a
	// fixture executes, because a suite whose fixtures all fail on missing
	// state reports the same thing many times and names none of it.
	Setup string `yaml:"setup"`

	// Teardown runs once after those fixtures, whether they passed or failed,
	// and state a suite created outside its own database is still released by a
	// run that ended badly.
	Teardown string `yaml:"teardown"`
}
