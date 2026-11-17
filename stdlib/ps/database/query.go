package database

import (
	"strings"
	"unicode"

	"github.com/titpetric/phpscript/telemetry"
)

// queryInfo holds the keyword a statement starts with and the comment a caller tagged it by.
type queryInfo struct {
	// Type is the leading keyword, lowercased: "select", "insert", "drop".
	// Type stays empty when the statement holds nothing but comments, or
	// starts with something that is not a word.
	Type string

	// Comment is the text of the first /* */ comment preceding the keyword,
	// trimmed: `/* userGet */ select ...` reads as "userGet".
	Comment string
}

// readOnlyStatements lists the statements a read-only client may run, EXPLAIN excluded.
var readOnlyStatements = map[string]bool{
	"select":   true,
	"show":     true,
	"describe": true,
	"desc":     true,
}

// parseQuery classifies a statement by its leading keyword, skipping any preceding comment.
func parseQuery(query string) queryInfo {
	var info queryInfo
	rest := query
	for {
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		switch {
		case strings.HasPrefix(rest, "/*"):
			end := strings.Index(rest[2:], "*/")
			if end < 0 {
				// An unterminated comment leaves no statement behind it.
				return info
			}
			if info.Comment == "" {
				info.Comment = strings.TrimSpace(rest[2 : 2+end])
			}
			rest = rest[2+end+2:]
		case strings.HasPrefix(rest, "--"), strings.HasPrefix(rest, "#"):
			line := strings.IndexByte(rest, '\n')
			if line < 0 {
				return info
			}
			rest = rest[line+1:]
		default:
			info.Type = leadingKeyword(rest)
			return info
		}
	}
}

// isRead reports whether a read-only client may run the statement.
func (i queryInfo) isRead() bool {
	return readOnlyStatements[i.Type]
}

// refusal names the statement in the error a read-only client returns for it.
func (i queryInfo) refusal() string {
	if i.Type == "" {
		return "the statement does not start with a keyword"
	}
	return i.Type + " is not allowed"
}

// record sets the statement's type and tag on the span measuring it.
func (i queryInfo) record(span *telemetry.Span) {
	if i.Type != "" {
		span.SetAttribute("query_type", i.Type)
	}
	if i.Comment != "" {
		span.SetAttribute("query_comment", i.Comment)
	}
}

// leadingKeyword returns the statement's first word lowercased, empty when it starts otherwise.
func leadingKeyword(query string) string {
	end := strings.IndexFunc(query, func(r rune) bool { return !unicode.IsLetter(r) })
	switch {
	case end < 0:
		return strings.ToLower(query)
	case end == 0:
		return ""
	default:
		return strings.ToLower(query[:end])
	}
}
