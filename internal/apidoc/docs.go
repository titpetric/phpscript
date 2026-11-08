package apidoc

import (
	"encoding/json"
	"strings"
	"sync"

	_ "embed"

	"github.com/titpetric/phpscript/runner"
)

// IndexPath is where the generated doc index sits, relative to the module
// root. `atkins test:introspection` writes it and the embed below reads it.
const IndexPath = "internal/apidoc/godoc.json"

// A shipped binary holds no Go source tree, and the two things a scan of one
// recovers are a doc comment and the PHP spelling of a parameter name. The
// pipeline generates the index at build time and the binary embeds it, the way
// it embeds `config/config.yml` and the helpdocs examples. What a class, a
// property and a method document travels with the binary; what a class holds,
// and what type each member holds, stays a question for reflection against
// the live runtime.
//
//go:embed godoc.json
var indexJSON []byte

var (
	embeddedOnce sync.Once
	embedded     *Docs
)

// Docs is the source text of the registered surface, the half of a listing
// reflection cannot answer.
type Docs struct {
	// Classes is what each registered class documents, by the PHP name a
	// script types. Two class names bound to one Go type document
	// separately, which the SPL exception names and Exception do.
	Classes map[string]*SymbolDocs `json:"classes,omitempty"`

	// Types is the source text of the members of each bound Go type, by
	// import path and type name.
	Types map[string]*TypeDocs `json:"types,omitempty"`
}

// SymbolDocs is what one declaration says: its doc comment and the PHP
// spelling of the parameters it takes.
type SymbolDocs struct {
	Comment []string `json:"comment,omitempty"`
	Params  []string `json:"params,omitempty"`
}

// TypeDocs is the source text of one Go type's members, by Go name.
type TypeDocs struct {
	Fields  map[string][]string    `json:"fields,omitempty"`
	Methods map[string]*SymbolDocs `json:"methods,omitempty"`
}

// Embedded returns the doc index this binary embeds. An unreadable index
// answers an empty one, and every class then reports what reflection sees and
// no prose.
func Embedded() *Docs {
	embeddedOnce.Do(func() {
		embedded = &Docs{}
		decoded := &Docs{}
		if json.Unmarshal(indexJSON, decoded) == nil {
			embedded = decoded
		}
	})
	return embedded
}

// ScanDocs builds the doc index for the classes rt registers, reading the Go
// source tree at root. The index holds the registered surface and no more: a
// comment on a type no constructor returns reaches no listing.
func ScanDocs(rt *runner.Runtime, root string) (*Docs, error) {
	src, err := scan(root)
	if err != nil {
		return nil, err
	}
	_, classes := build(rt, src)

	out := &Docs{Classes: map[string]*SymbolDocs{}, Types: map[string]*TypeDocs{}}
	for _, c := range classes {
		entry := &SymbolDocs{Comment: c.Comment, Params: paramNames(c.Params)}
		for _, name := range append([]string{c.Name}, c.Aliases...) {
			out.Classes[name] = entry
		}
		out.scanType(c, src)
	}
	out.prune()
	return out, nil
}

// scanType records the source text of the bound Go type's members, for the
// members the class publishes and no others.
//
// The scan of the module tree answers for a type this module declares. The Go
// distribution answers for a type the standard library declares, which
// HTTP\Request and Time both bind: the module scan indexes a method by bare
// type name and a namesake here would answer for net/http's.
func (d *Docs) scanType(c Class, src *sources) {
	if c.GoType == "" {
		return
	}
	entry := d.Types[c.GoType]
	if entry == nil {
		entry = &TypeDocs{Fields: map[string][]string{}, Methods: map[string]*SymbolDocs{}}
		d.Types[c.GoType] = entry
	}

	var dist *TypeDocs
	if isStdlib(c.Binds) {
		dist = gorootDocs(c.Binds, strings.TrimPrefix(c.GoType, c.Binds+"."))
	}
	fields := src.paths[c.GoType]
	for _, f := range c.Fields {
		switch {
		case dist != nil:
			if lines := dist.Fields[f.GoName]; len(lines) > 0 {
				entry.Fields[f.GoName] = lines
			}
		case fields != nil:
			if lines := fields.fields[f.GoName]; len(lines) > 0 {
				entry.Fields[f.GoName] = lines
			}
		}
	}
	for _, m := range c.Methods {
		if dist != nil {
			if from := dist.Methods[m.GoName]; from != nil {
				entry.Methods[m.GoName] = from
				continue
			}
		}
		entry.Methods[m.GoName] = &SymbolDocs{Comment: m.Comment, Params: paramNames(m.Params)}
	}
}

// Apply fills in the source text of classes from the index, leaving a member
// the index does not name as reflection reported it.
func (d *Docs) Apply(classes []Class) {
	for i := range classes {
		c := &classes[i]
		if entry := d.Classes[c.Name]; entry != nil {
			c.Comment = entry.Comment
			renameParams(c.Params, entry.Params)
		}
		entry := d.Types[c.GoType]
		if entry == nil {
			continue
		}
		for j := range c.Fields {
			c.Fields[j].Comment = entry.Fields[c.Fields[j].GoName]
		}
		for j := range c.Methods {
			member := entry.Methods[c.Methods[j].GoName]
			if member == nil {
				continue
			}
			c.Methods[j].Comment = member.Comment
			renameParams(c.Methods[j].Params, member.Params)
		}
	}
}

// paramNames lists the PHP spelling of each parameter, the half of a signature
// reflection has to invent.
func paramNames(params []Param) []string {
	if len(params) == 0 {
		return nil
	}
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

// renameParams spells the reflected parameters the way the source declares
// them. A count that disagrees means the index predates the signature and the
// reflected names stand: renaming by position would move a name onto another
// argument.
func renameParams(params []Param, names []string) {
	if len(params) != len(names) {
		return
	}
	for i := range params {
		params[i].Name = names[i]
	}
}

// prune drops the empty entries, which keeps the generated file to the source
// text that exists.
func (d *Docs) prune() {
	for name, entry := range d.Classes {
		if len(entry.Comment) == 0 && len(entry.Params) == 0 {
			delete(d.Classes, name)
		}
	}
	for key, entry := range d.Types {
		for name, member := range entry.Methods {
			if len(member.Comment) == 0 && len(member.Params) == 0 {
				delete(entry.Methods, name)
			}
		}
		if len(entry.Fields) == 0 {
			entry.Fields = nil
		}
		if len(entry.Methods) == 0 {
			entry.Methods = nil
		}
		if entry.Fields == nil && entry.Methods == nil {
			delete(d.Types, key)
		}
	}
}
