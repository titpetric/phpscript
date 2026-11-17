package shared

import (
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// Limits bounds what a hostile form can build. A zero field means PHP's
// default; negative means no limit.
type Limits struct {
	// MaxNesting is php.ini's max_input_nesting_level. The parse drops a
	// variable past it whole and never truncates one.
	MaxNesting int

	// MaxVars is php.ini's max_input_vars.
	MaxVars int
}

// php.ini's defaults for the two limits, the values a runtime uses when a
// host names neither.
const (
	DefaultMaxNesting = 64
	DefaultMaxVars    = 1000
)

func (l Limits) maxNesting() int {
	if l.MaxNesting == 0 {
		return DefaultMaxNesting
	}
	return l.MaxNesting
}

func (l Limits) maxVars() int {
	if l.MaxVars == 0 {
		return DefaultMaxVars
	}
	return l.MaxVars
}

// ParseStr decodes an application/x-www-form-urlencoded string into the nested
// array PHP builds from it. It backs parse_str(), $_GET and $_POST.
//
// ParseStr decodes before it reads the brackets and `k%5Ba%20b%5D=1` nests
// under the key `a b`.
func ParseStr(raw string, limits Limits) *model.Array {
	if raw == "" {
		return model.NewArray()
	}
	// PHP 8 splits on `&` only, with no arg_separator honouring `;` any more.
	fields := strings.Split(raw, "&")
	pairs := make([]Pair, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		// No `=` is a name with an empty value: `a&b=1` sets both.
		name, value, _ := strings.Cut(field, "=")
		pairs = append(pairs, Pair{Name: URLDecode(name), Value: URLDecode(value)})
	}
	return ParsePairs(limits, pairs)
}

// Pair is one form field, name and value already decoded.
type Pair struct {
	Name  string
	Value string
}

// ParsePairs is ParseStr for input that arrived as pairs: multipart values and
// the cookie header. Order is the order ParsePairs applies them in, which
// decides the result when two name the same variable.
func ParsePairs(limits Limits, pairs []Pair) *model.Array {
	out := model.NewArray()
	maxVars, maxNesting := limits.maxVars(), limits.maxNesting()
	for i, pair := range pairs {
		if maxVars >= 0 && i >= maxVars {
			break
		}
		key, path, ok := parseKey(pair.Name, maxNesting)
		if !ok {
			continue
		}
		assign(out, key, path, pair.Value)
	}
	return out
}

// parseKey splits a name into its mangled top-level key and bracket path, refusing a dropped name.
func parseKey(name string, maxNesting int) (string, []string, bool) {
	open := strings.IndexByte(name, '[')
	if open < 0 || strings.IndexByte(name[open:], ']') < 0 {
		return mangle(name), nil, name != ""
	}
	top := mangle(name[:open])
	if top == "" {
		return "", nil, false
	}
	var path []string
	for open < len(name) && name[open] == '[' {
		end := strings.IndexByte(name[open:], ']')
		if end < 0 {
			break
		}
		end += open
		path = append(path, name[open+1:end])
		if maxNesting >= 0 && len(path) > maxNesting {
			return "", nil, false
		}
		open = end + 1
	}
	return top, path, true
}

// mangle substitutes underscores for space, . and [ in a top-level name.
func mangle(name string) string {
	if !strings.ContainsAny(name, " .[") {
		return name
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '.' || r == '[' {
			return '_'
		}
		return r
	}, name)
}

// assign writes value at key and path; the last assignment decides the shape.
func assign(root *model.Array, key string, path []string, value string) {
	if len(path) == 0 {
		root.Set(phpval.Key(key), value)
		return
	}
	current := child(root, phpval.Key(key))
	for i, segment := range path {
		last := i == len(path)-1
		if segment == "" {
			if last {
				current.Append(value)
				return
			}
			next := model.NewArray()
			current.Append(next)
			current = next
			continue
		}
		k := phpval.Key(segment)
		if last {
			current.Set(k, value)
			return
		}
		current = child(current, k)
	}
}

// child returns the array at key, replacing a scalar and never merging into it.
func child(parent *model.Array, key any) *model.Array {
	if existing, ok := parent.Get(key); ok {
		if arr, ok := existing.(*model.Array); ok {
			return arr
		}
	}
	next := model.NewArray()
	parent.Set(key, next)
	return next
}
