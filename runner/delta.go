package runner

// Delta records one install's effect on a runtime's symbol tables.
type Delta struct {
	// Funcs are the functions the install defined.
	Funcs []string
	// Replaced are the functions that already existed and the install
	// registered again: the overlay's names.
	Replaced []string
	// Classes are the classes and constructors the install defined.
	Classes []string
	// Constants are the constants the install defined.
	Constants []string
}
