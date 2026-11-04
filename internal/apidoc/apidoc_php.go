package apidoc

import (
	"fmt"
	"strings"
)

// PHP renders the class as a PHP class definition.
func (c Class) PHP() string {
	var b strings.Builder
	writeDocblock(&b, c.Describe(), "")
	fmt.Fprintf(&b, "class %s%s\n{\n", c.Name, c.implementsClause())
	// Undocumented properties run together the way a struct declares them;
	// a documented one takes a blank line so its block opens away from the
	// declaration above it.
	for i, f := range c.Fields {
		if i > 0 && len(f.Comment) > 0 {
			b.WriteString("\n")
		}
		writeDocblock(&b, f.Comment, "    ")
		fmt.Fprintf(&b, "    %s;\n", f.Signature())
	}
	if len(c.Fields) > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "    %s;\n", c.Signature())
	for _, m := range c.Methods {
		b.WriteString("\n")
		writeDocblock(&b, m.Comment, "    ")
		fmt.Fprintf(&b, "    %s;\n", m.Signature())
	}
	b.WriteString("}\n")
	return b.String()
}

// Describe is the class description: the godoc of the bound Go type, then the
// citation of that type and of the interfaces it satisfies.
func (c Class) Describe() []string {
	lines := append([]string{}, c.Comment...)
	cites := c.cited()
	if len(cites) == 0 {
		return lines
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, cites...)
}

// cited names the Go package and symbol behind the class, and the interfaces
// the bound type satisfies. A standard library symbol carries a link to its
// reference page; the citation names every other symbol bare.
func (c Class) cited() []string {
	lines := []string{}
	if c.Binds == "" && c.Uses.Name == "" {
		return lines
	}
	lines = append(lines, fmt.Sprintf("Binds %s package, uses %s.", c.Binds, c.usesSymbol()))
	if len(c.Implements) == 0 {
		return lines
	}
	parts := make([]string, 0, len(c.Implements))
	for _, i := range c.Implements {
		parts = append(parts, i.Link())
	}
	return append(lines, "Implements "+strings.Join(parts, ", ")+".")
}

// usesSymbol is the symbol the citation names, written bare when it sits in
// the package the class binds and qualified when it comes from elsewhere.
func (c Class) usesSymbol() string {
	if c.Uses.URL == "" && c.Uses.Package == c.Binds {
		return c.Uses.Name
	}
	return c.Uses.Link()
}

// implementsClause is the interface list the class header carries, empty for a
// bound type that satisfies none of the set.
func (c Class) implementsClause() string {
	if len(c.Implements) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.Implements))
	for _, i := range c.Implements {
		names = append(names, i.Clause())
	}
	return " implements " + strings.Join(names, ", ")
}

// Rename answers a copy of the class under another of its registered names.
func (c Class) Rename(name string) Class {
	c.Name = name
	c.Aliases = nil
	return c
}

// writeDocblock writes doc comment lines as a PHP docblock. A member the index
// documents with no line gets none, in place of an empty block.
func writeDocblock(b *strings.Builder, lines []string, indent string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s/**\n", indent)
	for _, line := range lines {
		fmt.Fprintf(b, "%s *%s\n", indent, strings.TrimRight(" "+line, " "))
	}
	fmt.Fprintf(b, "%s */\n", indent)
}
