package list

// Symbol is one name the runtime answers to, as a script would type it.
type Symbol struct {
	Kind      string // "function", "class", "method" or "constant"
	Name      string
	Signature string // PHP declaration, or the value for a constant
}
