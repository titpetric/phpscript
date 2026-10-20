package model

import (
	"sort"
)

// Object is a class instance: a property bag plus a pointer back to its class.
// Because methods live on the Class (resolved by the runtime), an Object passed
// into expr-lang exposes its Props for `$obj->field` style access.
//
// Props is a Go map and has no order of its own, so order records the sequence
// the properties arrive in. Every reader passes through Names or Range and
// than ranging the map, because PHP reads properties back in a defined order
// and a script can see it: json_encode, print_r, var_dump, get_object_vars, the
// `(array)` cast and `foreach` all read them back in it.
type Object struct {
	Class *Class
	Props map[string]any
	ID    string
	order []string
}

// NewObject builds an instance with field defaults applied.
func NewObject(c *Class) *Object {
	size := 0
	if c != nil {
		size = len(c.Fields)
	}
	return &Object{Class: c, Props: make(map[string]any, size), order: make([]string, 0, size)}
}

// SetID records the PHP variable receiving this constructed object.
func (o *Object) SetID(id string) {
	o.ID = id
}

// SetProp writes a property, recording the name on its first write.
// Assigning over a property that is already set leaves its position alone,
// as PHP does.
func (o *Object) SetProp(name string, v any) {
	if o.Props == nil {
		o.Props = map[string]any{}
	}
	if _, exists := o.Props[name]; !exists {
		o.order = append(o.order, name)
	}
	o.Props[name] = v
}

// Prop reads a property.
func (o *Object) Prop(name string) (any, bool) {
	v, ok := o.Props[name]
	return v, ok
}

// DeleteProp removes a property. A later write to a declared field returns it
// to its declared position, because Names reads the declarations first; SetProp
// appends a dynamic one, which lands at the end. Both are PHP's behaviour.
func (o *Object) DeleteProp(name string) {
	if _, ok := o.Props[name]; !ok {
		return
	}
	delete(o.Props, name)
	for i, have := range o.order {
		if have == name {
			o.order = append(o.order[:i], o.order[i+1:]...)
			break
		}
	}
}

// Len counts the properties the object holds.
func (o *Object) Len() int {
	return len(o.Props)
}

// Names returns the property names in the order PHP reads them back: the
// declared fields first, in declaration order, then the properties the script
// added, in the order it added them.
//
// A property written straight into Props, bypassing SetProp, is still reported;
// it sorts after the recorded ones and never disappears, so a missed call
// site degrades to the order this type had before it recorded one.
func (o *Object) Names() []string {
	out := make([]string, 0, len(o.Props))
	if o.Class != nil {
		for _, field := range o.Class.Fields {
			if _, ok := o.Props[field.Name]; ok {
				out = append(out, field.Name)
			}
		}
	}
	for _, name := range o.order {
		if _, ok := o.Props[name]; !ok {
			continue
		}
		if o.Class != nil && o.Class.declares(name) {
			continue
		}
		out = append(out, name)
	}
	if len(out) < len(o.Props) {
		out = o.appendUnrecorded(out)
	}
	return out
}

// appendUnrecorded adds the properties Names did not reach, sorted, so Names
// reports a direct Props write in a stable order and never a random one.
func (o *Object) appendUnrecorded(out []string) []string {
	have := make(map[string]bool, len(out))
	for _, name := range out {
		have[name] = true
	}
	rest := make([]string, 0, len(o.Props)-len(out))
	for name := range o.Props {
		if !have[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// Range calls fn for each property in Names order, stopping early if it
// returns false.
func (o *Object) Range(fn func(name string, v any) bool) {
	for _, name := range o.Names() {
		if !fn(name, o.Props[name]) {
			return
		}
	}
}
