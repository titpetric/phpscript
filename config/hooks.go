package config

// Hooks are the PHP files a test session runs around the fixtures of one suite
// root. Both are resolved against that root, and both run once per session
// and not once per fixture.
//
// They are configuration and no annotation because a test session is
// not a server. @startup, @route and @schedule are scanned out of a source tree
// by `phpscript server`, which runs them per virtual host or per application
// root depending on how it is configured; there is no such scope in a fixture
// run, and a comment in a file that a walk happened to reach is not a contract
// a suite can be held to.
type Hooks struct {
	// Setup runs once before any fixture below this suite root: the schema its
	// fixtures query and the rows they read. A failure fails the run before a
	// fixture executes, because a suite whose fixtures all fail on missing
	// state reports the same thing many times and names none of it.
	Setup string `yaml:"setup"`

	// Teardown runs once after those fixtures, whether they passed or failed,
	// so state a suite created outside its own database is still released by a
	// run that ended badly.
	Teardown string `yaml:"teardown"`
}
