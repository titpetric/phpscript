package list

// MethodRow is one class or interface member of the API listing. Member is the
// method name, Signature holds the declarations the reader needs to call it,
// and Doc is the docblock above the declaration with its comment markers
// stripped, on one line.
type MethodRow struct {
	Class     string `json:"class"`
	Member    string `json:"member"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
}
