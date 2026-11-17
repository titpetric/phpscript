package list

// Symbol is one name the runtime answers to, as a script would type it.
type Symbol struct {
	Kind      string `json:"kind"`      // "function", "class", "method" or "constant"
	Name      string `json:"name"`
	Signature string `json:"signature"` // PHP declaration, or the value for a constant
	Area      string `json:"area"`      // namespace of the area that defines it, Runtime\PS\Core\Strings
}
