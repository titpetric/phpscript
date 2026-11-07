package bindings

import (
	"reflect"

	"github.com/titpetric/phpscript/runner"
)

// Set collects the entries of one package and installs them as its area.
type Set struct {
	path    string
	area    string
	entries []Entry
	seeds   []reflect.Type
}

// Bind files one name of the package.
func (s *Set) Bind(name string, value any) *Set {
	s.entries = append(s.entries, Entry{Package: s.path, Name: name, Value: value})
	return s
}

// Type files a package type no bound signature reaches, by a zero value of
// it. The walk starts from it as it starts from a signature.
func (s *Set) Type(sample any) *Set {
	if kind := reflect.TypeOf(sample); kind != nil {
		s.seeds = append(s.seeds, kind)
	}
	return s
}

// Entries returns what the set binds, in the order a binding wrote them.
func (s *Set) Entries() []Entry { return s.entries }

// Area is the name a script imports the package by.
func (s *Set) Area() string { return s.area }

// Install contributes the area to the runtime that imports the package.
func (s *Set) Install() {
	runner.RegisterNamespace(s.area, s.Register)
}

// Register installs the entries and the types they reach on one runtime.
func (s *Set) Register(rt *runner.Runtime) {
	for _, entry := range s.entries {
		name := s.area + `\` + entry.Name
		if value := reflect.ValueOf(entry.Value); value.Kind() == reflect.Func {
			rt.RegisterFunc(name, entry.Value)
			continue
		}
		rt.SetConst(name, entry.Value)
	}
	for name, kind := range s.Types() {
		// A struct is what `new` makes sense of. An interface, a named map or
		// a func type the package declares reaches a script as the value a
		// binding answered with, and constructing one would answer a nil.
		if kind.Kind() == reflect.Struct {
			rt.RegisterConstructor(s.area+`\`+name, constructor(kind))
		}
	}
}

// constructor answers the host constructor of a discovered type: a zero value
// of it, addressable, as a go function takes and returns one.
func constructor(kind reflect.Type) any {
	return func() any { return reflect.New(kind).Interface() }
}
